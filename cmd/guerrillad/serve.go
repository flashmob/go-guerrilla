package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"

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
				log.Infof("Configuration is reloaded at %s", guerrilla.ConfigLoadTime)
				cmdConfig.emitChangeEvents(&oldConfig)
			}
		} else if sig == syscall.SIGTERM || sig == syscall.SIGQUIT || sig == syscall.SIGINT {
			log.Infof("Shutdown signal caught")
			app.Shutdown()
			log.Infof("Shutdown completd, exiting.")
			os.Exit(0)
		} else {
			os.Exit(0)
		}
	}
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

	// Write out our PID
	if len(pidFile) > 0 {
		if f, err := os.Create(pidFile); err == nil {
			defer f.Close()
			if _, err := f.WriteString(fmt.Sprintf("%d", os.Getpid())); err == nil {
				f.Sync()
			} else {
				log.WithError(err).Fatalf("Error while writing pidFile (%s)", pidFile)
			}
		} else {
			log.WithError(err).Fatalf("Error while creating pidFile (%s)", pidFile)
		}
	}
	var backend backends.Backend
	backend, err = backends.New(cmdConfig.BackendName, cmdConfig.BackendConfig)
	if err != nil {
		log.WithError(err).Fatalf("Exiting")
	}

	if app, err := guerrilla.New(&cmdConfig.AppConfig, backend); err == nil {
		go app.Start()
		sigHandler(app)
	} else {
		log.WithError(err).Fatalf("Exiting")
	}

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

func (c *CmdConfig) emitChangeEvents(oldConfig *CmdConfig) {
	// has backend changed?
	if !reflect.DeepEqual((*c).BackendConfig, (*oldConfig).BackendConfig) {
		guerrilla.Bus.Publish("config_change:backend_config", c)
	}
	if c.BackendName != oldConfig.BackendName {
		guerrilla.Bus.Publish("config_change:backend_name", c)
	}
	// call other emitChangeEvents
	c.AppConfig.EmitChangeEvents(&oldConfig.AppConfig)
}

// ReadConfig which should be called at startup, or when a SIG_HUP is caught
func readConfig(path string, pidFile string, config *CmdConfig) error {
	// load in the config.
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("Could not read config file: %s", err.Error())
	}
	config.load(data)
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
