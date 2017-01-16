package main

import (
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/flashmob/go-guerrilla"
	"github.com/flashmob/go-guerrilla/backends"
	"reflect"
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
)

func init() {
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
				log.WithError(err).Error("Error while ReadConfig (reload)")
			} else {
				cmdConfig = newConfig
				log.Infof("Configuration was reloaded at %s", guerrilla.ConfigLoadTime)
				cmdConfig.emitChangeEvents(&oldConfig, app)
			}
		} else if sig == syscall.SIGTERM || sig == syscall.SIGQUIT || sig == syscall.SIGINT {
			log.Infof("Shutdown signal caught")
			app.Shutdown()
			log.Infof("Shutdown completd, exiting.")
			return
		} else {
			log.Infof("Shutdown, unknown signal caught")
			return
		}
	}
}

func subscribeBackendEvent(event string, backend backends.Backend, app guerrilla.Guerrilla) {

	app.Subscribe(event, func(cmdConfig *CmdConfig) {
		var err error
		if err = backend.Shutdown(); err != nil {
			log.WithError(err).Warn("Backend failed to shutdown")
			return
		}
		backend, err = backends.New(cmdConfig.BackendName, cmdConfig.BackendConfig)
		if err != nil {
			log.WithError(err).Fatalf("Error while loading the backend %q",
				cmdConfig.BackendName)
		} else {
			log.Info("Backend started:", cmdConfig.BackendName)
		}
	})
}

func serve(cmd *cobra.Command, args []string) {
	logVersion()

	err := readConfig(configPath, pidFile, &cmdConfig)
	if err != nil {
		log.WithError(err).Fatal("Error while reading config")
	}

	// Check that max clients is not greater than system open file limit.
	fileLimit := getFileLimit()

	if fileLimit > 0 {
		maxClients := 0
		for _, s := range cmdConfig.Servers {
			maxClients += s.MaxClients
		}
		if maxClients > fileLimit {
			log.Fatalf("Combined max clients for all servers (%d) is greater than open file limit (%d). "+
				"Please increase your open file limit or decrease max clients.", maxClients, fileLimit)
		}
	}

	// Backend setup
	var backend backends.Backend
	backend, err = backends.New(cmdConfig.BackendName, cmdConfig.BackendConfig)
	if err != nil {
		log.WithError(err).Fatalf("Error while loading the backend %q",
			cmdConfig.BackendName)
	}

	app, err := guerrilla.New(&cmdConfig.AppConfig, backend)
	if err != nil {
		log.WithError(err).Error("Error(s) when creating new server(s)")
	}
	err = app.Start()
	if err != nil {
		log.WithError(err).Error("Error(s) when starting server(s)")
	}
	subscribeBackendEvent("config_change:backend_config", backend, app)
	subscribeBackendEvent("config_change:backend_name", backend, app)
	// Write out our PID
	writePid(cmdConfig.PidFile)
	// ...and write out our pid whenever the file name changes in the config
	app.Subscribe("config_change:pid_file", func(ac *guerrilla.AppConfig) {
		writePid(ac.PidFile)
	})
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
	c.AppConfig.Load(jsonBytes)
	err := json.Unmarshal(jsonBytes, &c)
	if err != nil {
		return fmt.Errorf("Could not parse config file: %s", err.Error())
	}
	return nil
}

func (c *CmdConfig) emitChangeEvents(oldConfig *CmdConfig, app guerrilla.Guerrilla) {
	// has backend changed?
	if !reflect.DeepEqual((*c).BackendConfig, (*oldConfig).BackendConfig) {
		app.Publish("config_change:backend_config", c)
	}
	if c.BackendName != oldConfig.BackendName {
		app.Publish("config_change:backend_name", c)
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
				log.Infof("pid_file (%s) written with pid:%v", pidFile, pid)
			} else {
				log.WithError(err).Fatalf("Error while writing pidFile (%s)", pidFile)
			}
		} else {
			log.WithError(err).Fatalf("Error while creating pidFile (%s)", pidFile)
		}
	}
}
