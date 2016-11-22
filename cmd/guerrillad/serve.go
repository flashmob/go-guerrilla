package main

import (
	"os"
	"os/signal"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"

	evbus "github.com/asaskevich/EventBus"

	"fmt"

	guerrilla "github.com/flashmob/go-guerrilla"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/config"
	"github.com/flashmob/go-guerrilla/server"
	"strings"
)

var (
	iface      string
	configFile string
	pidFile    string

	serveCmd = &cobra.Command{
		Use:   "serve",
		Short: "start the small SMTP server",
		Run:   serve,
	}

	bus *evbus.EventBus

	mainConfig    = guerrilla.Config{}
	signalChannel = make(chan os.Signal, 1) // for trapping SIG_HUP

)

func init() {
	serveCmd.PersistentFlags().StringVarP(&iface, "if", "", "",
		"Interface and port to listen on, eg. 127.0.0.1:2525 ")
	serveCmd.PersistentFlags().StringVarP(&configFile, "config", "c",
		"goguerrilla.conf", "Path to the configuration file")
	serveCmd.PersistentFlags().StringVarP(&pidFile, "pidFile", "p",
		"", "Path to the pid file")

	rootCmd.AddCommand(serveCmd)
	bus = evbus.New()
}

func sigHandler() {
	// handle SIGHUP for reloading the configuration while running
	signal.Notify(signalChannel, syscall.SIGHUP)

	for sig := range signalChannel {
		if sig == syscall.SIGHUP {
			oldConfig := mainConfig;
			err := config.ReadConfig(configFile, iface, pidFile, &mainConfig)
			if err != nil {
				log.WithError(err).Error("Error while ReadConfig (reload)")
			} else {
				emitConfigChangeEvents(oldConfig, mainConfig)
				log.Infof("Configuration is reloaded at %s", guerrilla.ConfigLoadTime)
			}
		} else {
			os.Exit(0)
		}
	}
}

func emitConfigChangeEvents(oldConfig guerrilla.Config, newConfig guerrilla.Config) {
	// has 'allowed hosts' changed?
	if strings.Compare(oldConfig.AllowedHosts, newConfig.AllowedHosts) != 0 {
		bus.Publish("config_change:allowed_hosts", newConfig)
	}
	if strings.Compare(oldConfig.PidFile, newConfig.PidFile) != 0 {
		bus.Publish("config_change:pid_file", newConfig)
	}

	// has backend changed?
	// TODO

	// has ssl config changed for any of the servers?
	// TODO

	// have log files changed?
	// TODO



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

func serve(cmd *cobra.Command, args []string) {
	logVersion()

	err := config.ReadConfig(configFile, iface, pidFile, &mainConfig)
	if err != nil {
		log.WithError(err).Fatal("Error while ReadConfig")
	}

	// write out our PID
	writePid(mainConfig.PidFile)
	// ...and write out our pid whenever the file name changes
	bus.Subscribe("config_change:pid_file", func(mainConfig guerrilla.Config) {
		writePid(mainConfig.PidFile)
	})

	backend, err := backends.New(mainConfig.BackendName, mainConfig.BackendConfig)
	if err != nil {
		log.WithError(err).Fatalf("Error while loading the backend %q",
			mainConfig.BackendName)
	}

	// run our servers
	for _, serverConfig := range mainConfig.Servers {
		if serverConfig.IsEnabled {
			log.Infof("Starting server on %s", serverConfig.ListenInterface)
			go func(sConfig guerrilla.ServerConfig) {
				err := server.RunServer(mainConfig, sConfig, backend, bus)
				if err != nil {
					log.WithError(err).Fatalf("Error while starting server on %s", serverConfig.ListenInterface)
				}
			}(serverConfig)
		}
	}

	sigHandler()
}
