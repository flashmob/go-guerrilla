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

type Daemon struct {
	Config  *AppConfig
	Logger  log.Logger
	Backend backends.Backend

	g Guerrilla

	configLoadTime time.Time
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
			d.Logger, err = log.GetLogger(d.Config.LogFile)
			if err != nil {
				return err
			}
			d.Logger.SetLevel(d.Config.LogLevel)
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
	}
	err = d.g.Start()
	if err == nil {
		if err := d.resetLogger(); err == nil {
			d.log().Infof("main log configured to %s", d.Config.LogFile)
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
func (d *Daemon) LoadConfig(path string) (AppConfig, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return *d.Config, fmt.Errorf("Could not read config file: %s", err.Error())
	}
	d.Config = &AppConfig{}
	if err := d.Config.Load(data); err != nil {
		return *d.Config, err
	}
	d.configLoadTime = time.Now()
	return *d.Config, nil
}

// SetConfig is same as LoadConfig, except you can pass AppConfig directly
func (d *Daemon) SetConfig(c AppConfig) error {
	// Config.Load takes []byte so we need to serialize
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	// put the data into a fresh d.Config
	d.Config = &AppConfig{}
	if err := d.Config.Load(data); err != nil {
		return err
	}
	d.configLoadTime = time.Now()
	return nil
}

// Reload a config using the passed in AppConfig and emit config change events
func (d *Daemon) ReloadConfig(c AppConfig) error {
	if d.Config == nil {
		return errors.New("d.Config nil")
	}
	oldConfig := *d.Config
	err := d.SetConfig(c)
	if err != nil {
		d.log().WithError(err).Error("Error while reloading config")
		return err
	} else {
		d.log().Infof("Configuration was reloaded at %s", d.configLoadTime)
		d.Config.EmitChangeEvents(&oldConfig, d.g)
	}
	return nil
}

// Reload a config from a file and emit config change events
func (d *Daemon) ReloadConfigFile(path string) error {
	if d.Config == nil {
		return errors.New("d.Config nil")
	}
	var oldConfig AppConfig
	oldConfig = *d.Config
	_, err := d.LoadConfig(path)
	if err != nil {
		d.log().WithError(err).Error("Error while reloading config from file")
		return err
	} else {
		d.log().Infof("Configuration was reloaded at %s", d.configLoadTime)
		d.Config.EmitChangeEvents(&oldConfig, d.g)
	}
	return nil
}

// ReopenLogs re-opens all log files. Typically, one would call this after rotating logs
func (d *Daemon) ReopenLogs() {
	d.Config.EmitLogReopenEvents(d.g)
}

// Subscribe for subscribing to config change events
func (d *Daemon) Subscribe(topic Event, fn interface{}) error {
	return d.g.Subscribe(topic, fn)
}

// for publishing config change events
func (d *Daemon) Publish(topic Event, args ...interface{}) {
	d.g.Publish(topic, args...)
}

// for unsubscribing from config change events
func (d *Daemon) Unsubscribe(topic Event, handler interface{}) error {
	return d.g.Unsubscribe(topic, handler)
}

// log returns a logger that implements our log.Logger interface.
// level is set to "info" by default
func (d *Daemon) log() log.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	out := log.OutputStderr.String()
	if d.Config != nil && len(d.Config.LogFile) > 0 {
		out = d.Config.LogFile
	}
	l, err := log.GetLogger(out)
	if err == nil {
		l.SetLevel("info")
	}
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
	l, err := log.GetLogger(d.Config.LogFile)
	if err != nil {
		return err
	}
	d.Logger = l
	d.g.SetLogger(d.Logger)
	return nil
}
