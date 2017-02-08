package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/flashmob/go-guerrilla"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultPidFile = "/var/run/go-guerrilla.pid"
)

var (
	configPath string
	pidFile    string

	serveCmd = &cobra.Command{
		Use:   "serve",
		Short: "start the small SMTP server",
		Run:   serve,
	}

	cmdConfig     = CmdConfig{}
	signalChannel = make(chan os.Signal, 1) // for trapping SIG_HUP
	mainlog       log.Logger
)

func init() {
	// log to stderr on startup
	var logOpenError error
	if mainlog, logOpenError = log.GetLogger(log.OutputStderr.String()); logOpenError != nil {
		mainlog.WithError(logOpenError).Errorf("Failed creating a logger to %s", log.OutputStderr)
	}
	serveCmd.PersistentFlags().StringVarP(&configPath, "config", "c",
		"goguerrilla.conf", "Path to the configuration file")
	// intentionally didn't specify default pidFile; value from config is used if flag is empty
	serveCmd.PersistentFlags().StringVarP(&pidFile, "pidFile", "p",
		"", "Path to the pid file")

	rootCmd.AddCommand(serveCmd)
}

func sigHandler(app guerrilla.Guerrilla) {
	// handle SIGHUP for reloading the configuration while running
	signal.Notify(signalChannel, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGKILL)

	for sig := range signalChannel {
		if sig == syscall.SIGHUP {
			// save old config & load in new one
			oldConfig := cmdConfig
			newConfig := CmdConfig{}
			err := readConfig(configPath, pidFile, &newConfig)
			if err != nil {
				// new config will not be applied
				mainlog.WithError(err).Error("Error while ReadConfig (reload)")
				// re-open logs
				cmdConfig.EmitLogReopenEvents(app)
			} else {
				cmdConfig = newConfig
				mainlog.Infof("Configuration was reloaded at %s", guerrilla.ConfigLoadTime)
				cmdConfig.emitChangeEvents(&oldConfig, app)
			}
		} else if sig == syscall.SIGTERM || sig == syscall.SIGQUIT || sig == syscall.SIGINT {
			mainlog.Infof("Shutdown signal caught")
			app.Shutdown()
			mainlog.Infof("Shutdown completed, exiting.")
			return
		} else {
			mainlog.Infof("Shutdown, unknown signal caught")
			return
		}
	}
}

func subscribeBackendEvent(event guerrilla.Event, backend backends.Backend, app guerrilla.Guerrilla) {

	app.Subscribe(event, func(cmdConfig *CmdConfig) {
		logger, _ := log.GetLogger(cmdConfig.LogFile)
		var err error
		if err = backend.Shutdown(); err != nil {
			logger.WithError(err).Warn("Backend failed to shutdown")
			return
		}
		backend, err = backends.New(cmdConfig.BackendName, cmdConfig.BackendConfig, logger)
		if err != nil {
			logger.WithError(err).Fatalf("Error while loading the backend %q",
				cmdConfig.BackendName)
		} else {
			logger.Info("Backend started:", cmdConfig.BackendName)
		}
	})
}

func serve(cmd *cobra.Command, args []string) {
	logVersion()

	err := readConfig(configPath, pidFile, &cmdConfig)
	if err != nil {
		mainlog.WithError(err).Fatal("Error while reading config")
	}

	// Check that max clients is not greater than system open file limit.
	fileLimit := getFileLimit()

	if fileLimit > 0 {
		maxClients := 0
		for _, s := range cmdConfig.Servers {
			maxClients += s.MaxClients
		}
		if maxClients > fileLimit {
			mainlog.Fatalf("Combined max clients for all servers (%d) is greater than open file limit (%d). "+
				"Please increase your open file limit or decrease max clients.", maxClients, fileLimit)
		}
	}

	// Backend setup
	var backend backends.Backend
	backend, err = backends.New(cmdConfig.BackendName, cmdConfig.BackendConfig, mainlog)
	if err != nil {
		mainlog.WithError(err).Fatalf("Error while loading the backend %q",
			cmdConfig.BackendName)
	}

	app, err := guerrilla.New(&cmdConfig.AppConfig, backend, mainlog)
	if err != nil {
		mainlog.WithError(err).Error("Error(s) when creating new server(s)")
	}

	// start the app
	err = app.Start()
	if err != nil {
		mainlog.WithError(err).Error("Error(s) when starting server(s)")
	}
	subscribeBackendEvent(guerrilla.EvConfigBackendConfig, backend, app)
	subscribeBackendEvent(guerrilla.EvConfigBackendName, backend, app)
	// Write out our PID
	writePid(cmdConfig.PidFile)
	// ...and write out our pid whenever the file name changes in the config
	app.Subscribe(guerrilla.EvConfigPidFile, func(ac *guerrilla.AppConfig) {
		writePid(ac.PidFile)
	})
	// change the logger from stdrerr to one from config
	mainlog.Infof("main log configured to %s", cmdConfig.LogFile)
	var logOpenError error
	if mainlog, logOpenError = log.GetLogger(cmdConfig.LogFile); logOpenError != nil {
		mainlog.WithError(logOpenError).Errorf("Failed changing to a custom logger [%s]", cmdConfig.LogFile)
	}
	app.SetLogger(mainlog)
	sigHandler(app)

}

// Superset of `guerrilla.AppConfig` containing options specific
// the the command line interface.
type CmdConfig struct {
	guerrilla.AppConfig
	BackendName   string                 `json:"backend_name"`
	BackendConfig backends.BackendConfig `json:"backend_config"`
}

func (c *CmdConfig) load(jsonBytes []byte) error {
	err := json.Unmarshal(jsonBytes, &c)
	if err != nil {
		return fmt.Errorf("Could not parse config file: %s", err.Error())
	} else {
		// load in guerrilla.AppConfig
		return c.AppConfig.Load(jsonBytes)
	}
}

func (c *CmdConfig) emitChangeEvents(oldConfig *CmdConfig, app guerrilla.Guerrilla) {
	// has backend changed?
	if !reflect.DeepEqual((*c).BackendConfig, (*oldConfig).BackendConfig) {
		app.Publish(guerrilla.EvConfigBackendConfig, c)
	}
	if c.BackendName != oldConfig.BackendName {
		app.Publish(guerrilla.EvConfigBackendName, c)
	}
	// call other emitChangeEvents
	c.AppConfig.EmitChangeEvents(&oldConfig.AppConfig, app)
}

// ReadConfig which should be called at startup, or when a SIG_HUP is caught
func readConfig(path string, pidFile string, config *CmdConfig) error {
	// load in the config.
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("Could not read config file: %s", err.Error())
	}
	if err := config.load(data); err != nil {
		return err
	}
	// override config pidFile with with flag from the command line
	if len(pidFile) > 0 {
		config.AppConfig.PidFile = pidFile
	} else if len(config.AppConfig.PidFile) == 0 {
		config.AppConfig.PidFile = defaultPidFile
	}

	if len(config.AllowedHosts) == 0 {
		return errors.New("Empty `allowed_hosts` is not allowed")
	}
	guerrilla.ConfigLoadTime = time.Now()
	return nil
}

func getFileLimit() int {
	cmd := exec.Command("ulimit", "-n")
	out, err := cmd.Output()
	if err != nil {
		return -1
	}
	limit, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return -1
	}
	return limit
}

func writePid(pidFile string) {
	if len(pidFile) > 0 {
		if f, err := os.Create(pidFile); err == nil {
			defer f.Close()
			pid := os.Getpid()
			if _, err := f.WriteString(fmt.Sprintf("%d", pid)); err == nil {
				f.Sync()
				mainlog.Infof("pid_file (%s) written with pid:%v", pidFile, pid)
			} else {
				mainlog.WithError(err).Fatalf("Error while writing pidFile (%s)", pidFile)
			}
		} else {
			mainlog.WithError(err).Fatalf("Error while creating pidFile (%s)", pidFile)
		}
	}
}
