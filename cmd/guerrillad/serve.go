package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/flashmob/go-guerrilla"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/spf13/cobra"
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
	signalChannel = make(chan os.Signal, 1) // for trapping SIGHUP and friends
	mainlog       log.Logger

	d guerrilla.Daemon
)

func init() {
	// log to stderr on startup
	var err error
	mainlog, err = log.GetLogger(log.OutputStderr.String())
	if err != nil {
		mainlog.WithError(err).Errorf("Failed creating a logger to %s", log.OutputStderr)
	}
	serveCmd.PersistentFlags().StringVarP(&configPath, "config", "c",
		"goguerrilla.conf", "Path to the configuration file")
	// intentionally didn't specify default pidFile; value from config is used if flag is empty
	serveCmd.PersistentFlags().StringVarP(&pidFile, "pidFile", "p",
		"", "Path to the pid file")
	rootCmd.AddCommand(serveCmd)
}

func sigHandler() {
	signal.Notify(signalChannel,
		syscall.SIGHUP,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		syscall.SIGINT,
		syscall.SIGKILL,
		syscall.SIGUSR1,
	)
	for sig := range signalChannel {
		if sig == syscall.SIGHUP {
			d.ReloadConfigFile(configPath)
		} else if sig == syscall.SIGUSR1 {
			d.ReopenLogs()
		} else if sig == syscall.SIGTERM || sig == syscall.SIGQUIT || sig == syscall.SIGINT {
			mainlog.Infof("Shutdown signal caught")
			go func() {
				select {
				// exit if graceful shutdown not finished in 60 sec.
				case <-time.After(time.Second * 60):
					mainlog.Error("graceful shutdown timed out")
					os.Exit(1)
				}
			}()
			d.Shutdown()
			mainlog.Infof("Shutdown completed, exiting.")
			return
		} else {
			mainlog.Infof("Shutdown, unknown signal caught")
			return
		}
	}
}

func serve(cmd *cobra.Command, args []string) {
	logVersion()
	d = guerrilla.Daemon{Logger: mainlog}
	err := readConfig(configPath, pidFile)
	if err != nil {
		mainlog.WithError(err).Fatal("Error while reading config")
	}
	mainlog.SetLevel(cmdConfig.LogLevel)

	// Check that max clients is not greater than system open file limit.
	fileLimit := getFileLimit()

	if fileLimit > 0 {
		maxClients := 0
		for _, s := range cmdConfig.Servers {
			maxClients += s.MaxClients
		}
		if maxClients > fileLimit {
			mainlog.Warnf("Combined max clients for all servers (%d) is greater than open file limit (%d). "+
				"Please increase your open file limit or decrease max clients.", maxClients, fileLimit)
		}
	}

	err = d.Start()
	if err != nil {
		mainlog.WithError(err).Error("Error(s) when creating new server(s)")
		os.Exit(1)
	}
	sigHandler()

}

// Superset of `guerrilla.AppConfig` containing options specific
// the the command line interface.
type CmdConfig struct {
	guerrilla.AppConfig
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
	// if your CmdConfig has any extra fields, you can emit events here
	// ...
	// call other emitChangeEvents
	c.AppConfig.EmitChangeEvents(&oldConfig.AppConfig, app)
}

// ReadConfig which should be called at startup, or when a SIG_HUP is caught
func readConfig(path string, pidFile string) error {
	// Load in the config.
	// Note here is the only place we can make an exception to the
	// "treat config values as immutable". For example, here the
	// command line flags can override config values
	appConfig, err := d.LoadConfig(path)
	if err != nil {
		return fmt.Errorf("Could not read config file: %s", err.Error())
	}
	// override config pidFile with with flag from the command line
	if len(pidFile) > 0 {
		appConfig.PidFile = pidFile
	} else if len(appConfig.PidFile) == 0 {
		appConfig.PidFile = defaultPidFile
	}
	d.SetConfig(&appConfig)
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
