package guerrilla

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"io/ioutil"
	"time"
)

// Daemon provides a convenient API when using go-guerrilla as a package in your Go project.
// Is's facade for Guerrilla, AppConfig, backends.Backend and log.Logger
type Daemon struct {
	Config  *AppConfig
	Logger  log.Logger
	Backend backends.Backend

	// Guerrilla will be managed through the API
	g Guerrilla

	configLoadTime time.Time
	subs           []deferredSub
}

type deferredSub struct {
	topic Event
	fn    interface{}
}

const defaultInterface = "127.0.0.1:2525"

// AddProcessor adds a processor constructor to the backend.
// name is the identifier to be used in the config. See backends docs for more info.
func (d *Daemon) AddProcessor(name string, pc backends.ProcessorConstructor) {
	backends.Svc.AddProcessor(name, pc)
}

// Starts the daemon, initializing d.Config, d.Logger and d.Backend with defaults
// can only be called once through the lifetime of the program
func (d *Daemon) Start() (err error) {
	if d.g == nil {
		if d.Config == nil {
			d.Config = &AppConfig{}
		}
		if err = d.configureDefaults(); err != nil {
			return err
		}
		if d.Logger == nil {
			d.Logger, err = log.GetLogger(d.Config.LogFile, d.Config.LogLevel)
			if err != nil {
				return err
			}
		}
		if d.Backend == nil {
			d.Backend, err = backends.New(d.Config.BackendConfig, d.Logger)
			if err != nil {
				return err
			}
		}
		d.g, err = New(d.Config, d.Backend, d.Logger)
		if err != nil {
			return err
		}
		for i := range d.subs {
			d.Subscribe(d.subs[i].topic, d.subs[i].fn)

		}
		d.subs = make([]deferredSub, 0)
	}
	err = d.g.Start()
	if err == nil {
		if err := d.resetLogger(); err == nil {
			d.Log().Infof("main log configured to %s", d.Config.LogFile)
		}

	}
	return err
}

// Shuts down the daemon, including servers and backend.
// Do not call Start on it again, use a new server.
func (d *Daemon) Shutdown() {
	if d.g != nil {
		d.g.Shutdown()
	}
}

// LoadConfig reads in the config from a JSON file.
// Note: if d.Config is nil, the sets d.Config with the unmarshalled AppConfig which will be returned
func (d *Daemon) LoadConfig(path string) (AppConfig, error) {
	var ac AppConfig
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return ac, fmt.Errorf("Could not read config file: %s", err.Error())
	}
	err = ac.Load(data)
	if err != nil {
		return ac, err
	}
	if d.Config == nil {
		d.Config = &ac
	}
	return ac, nil
}

// SetConfig is same as LoadConfig, except you can pass AppConfig directly
// does not emit any change events, instead use ReloadConfig after daemon has started
func (d *Daemon) SetConfig(c AppConfig) error {
	// need to call c.Load, thus need to convert the config
	// d.load takes json bytes, marshal it
	data, err := json.Marshal(&c)
	if err != nil {
		return err
	}
	err = c.Load(data)
	if err != nil {
		return err
	}
	d.Config = &c
	return nil
}

// Reload a config using the passed in AppConfig and emit config change events
func (d *Daemon) ReloadConfig(c AppConfig) error {
	oldConfig := *d.Config
	err := d.SetConfig(c)
	if err != nil {
		d.Log().WithError(err).Error("Error while reloading config")
		return err
	} else {
		d.Log().Infof("Configuration was reloaded at %s", d.configLoadTime)
		d.Config.EmitChangeEvents(&oldConfig, d.g)
	}
	return nil
}

// Reload a config from a file and emit config change events
func (d *Daemon) ReloadConfigFile(path string) error {
	ac, err := d.LoadConfig(path)
	if err != nil {
		d.Log().WithError(err).Error("Error while reloading config from file")
		return err
	} else if d.Config != nil {
		oldConfig := *d.Config
		d.Config = &ac
		d.Log().Infof("Configuration was reloaded at %s", d.configLoadTime)
		d.Config.EmitChangeEvents(&oldConfig, d.g)
	}
	return nil
}

// ReopenLogs send events to re-opens all log files.
// Typically, one would call this after rotating logs
func (d *Daemon) ReopenLogs() error {
	if d.Config == nil {
		return errors.New("d.Config nil")
	}
	d.Config.EmitLogReopenEvents(d.g)
	return nil
}

// Subscribe for subscribing to config change events
func (d *Daemon) Subscribe(topic Event, fn interface{}) error {
	if d.g == nil {
		d.subs = append(d.subs, deferredSub{topic, fn})
		return nil
	}

	return d.g.Subscribe(topic, fn)
}

// for publishing config change events
func (d *Daemon) Publish(topic Event, args ...interface{}) {
	if d.g == nil {
		return
	}
	d.g.Publish(topic, args...)
}

// for unsubscribing from config change events
func (d *Daemon) Unsubscribe(topic Event, handler interface{}) error {
	if d.g == nil {
		for i := range d.subs {
			if d.subs[i].topic == topic && d.subs[i].fn == handler {
				d.subs = append(d.subs[:i], d.subs[i+1:]...)
			}
		}
		return nil
	}
	return d.g.Unsubscribe(topic, handler)
}

// log returns a logger that implements our log.Logger interface.
// level is set to "info" by default
func (d *Daemon) Log() log.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	out := log.OutputStderr.String()
	level := log.InfoLevel.String()
	if d.Config != nil {
		if len(d.Config.LogFile) > 0 {
			out = d.Config.LogFile
		}
		if len(d.Config.LogLevel) > 0 {
			level = d.Config.LogLevel
		}
	}
	l, _ := log.GetLogger(out, level)
	return l

}

// set the default values for the servers and backend config options
func (d *Daemon) configureDefaults() error {
	err := d.Config.setDefaults()
	if err != nil {
		return err
	}
	if d.Backend == nil {
		err = d.Config.setBackendDefaults()
		if err != nil {
			return err
		}
	}
	return err
}

// resetLogger sets the logger to the one specified in the config.
// This is because at the start, the daemon may be logging to stderr,
// then attaches to the logs once the config is loaded.
// This will propagate down to the servers / backend too.
func (d *Daemon) resetLogger() error {
	l, err := log.GetLogger(d.Config.LogFile, d.Config.LogLevel)
	if err != nil {
		return err
	}
	d.Logger = l
	d.g.SetLogger(d.Logger)
	return nil
}
