package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/flashmob/go-guerrilla"
	"github.com/flashmob/go-guerrilla/backends"
)

var (
	iface      string
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
	serveCmd.PersistentFlags().StringVarP(&pidFile, "pid-file", "p",
		"/var/run/go-guerrilla.pid", "Path to the pid file")

	rootCmd.AddCommand(serveCmd)
}

func sigHandler() {
	// handle SIGHUP for reloading the configuration while running
	signal.Notify(signalChannel, syscall.SIGHUP)

	for sig := range signalChannel {
		if sig == syscall.SIGHUP {
			err := readConfig(configPath, verbose, &cmdConfig)
			if err != nil {
				log.WithError(err).Error("Error while ReadConfig (reload)")
			} else {
				log.Infof("Configuration is reloaded at %s", guerrilla.ConfigLoadTime)
			}
			// TODO: reinitialize
		} else {
			os.Exit(0)
		}
	}
}

func serve(cmd *cobra.Command, args []string) {
	logVersion()

	err := readConfig(configPath, verbose, &cmdConfig)
	if err != nil {
		log.WithError(err).Fatal("Error while reading config")
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

	switch cmdConfig.BackendName {
	case "dummy":
		b := &backends.DummyBackend{}
		b.Initialize(cmdConfig.BackendConfig)
		cmdConfig.Backend = b
	case "guerrilla-db-redis":
		b := &backends.GuerrillaDBAndRedisBackend{}
		b.Initialize(cmdConfig.BackendConfig)
		cmdConfig.Backend = b
	default:
		log.Fatalf("Unknown backend: %s" , cmdConfig.BackendName)
	}

	app := guerrilla.New(&cmdConfig.AppConfig)
	go app.Start()
	sigHandler()
}

// Superset of `guerrilla.AppConfig` containing options specific
// the the command line interface.
type CmdConfig struct {
	guerrilla.AppConfig
	BackendName   string                 `json:"backend_name"`
	BackendConfig map[string]interface{} `json:"backend_config"`
}

// ReadConfig which should be called at startup, or when a SIG_HUP is caught
func readConfig(path string, verbose bool, config *CmdConfig) error {
	// load in the config.
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("Could not read config file: %s", err.Error())
	}

	err = json.Unmarshal(data, &config)
	if err != nil {
		return fmt.Errorf("Could not parse config file: %s", err.Error())
	}

	if len(config.AllowedHosts) == 0 {
		return errors.New("Empty `allowed_hosts` is not allowed")
	}

	guerrilla.ConfigLoadTime = time.Now()
	return nil
}
