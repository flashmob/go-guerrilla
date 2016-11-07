package main

import (
	"os"
	"os/signal"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"

	"fmt"

	guerrilla "github.com/flashmob/go-guerrilla"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/config"
	"github.com/flashmob/go-guerrilla/server"
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

	mainConfig    = guerrilla.Config{}
	signalChannel = make(chan os.Signal, 1) // for trapping SIG_HUB
)

func init() {
	serveCmd.PersistentFlags().StringVarP(&iface, "if", "", "",
		"Interface and port to listen on, eg. 127.0.0.1:2525 ")
	serveCmd.PersistentFlags().StringVarP(&configFile, "config", "c",
		"goguerrilla.conf", "Path to the configuration file")
	serveCmd.PersistentFlags().StringVarP(&pidFile, "pidFile", "p",
		"/var/run/go-guerrilla.pid", "Path to the pid file")

	rootCmd.AddCommand(serveCmd)
}

func sigHandler() {
	// handle SIGHUP for reloading the configuration while running
	signal.Notify(signalChannel, syscall.SIGHUP)

	for sig := range signalChannel {
		if sig == syscall.SIGHUP {
			err := config.ReadConfig(configFile, iface, verbose, &mainConfig)
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

	err := config.ReadConfig(configFile, iface, verbose, &mainConfig)
	if err != nil {
		log.WithError(err).Fatal("Error while ReadConfig")
	}

	// write out our PID
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
				err := server.RunServer(sConfig, backend, mainConfig.AllowedHosts)
				if err != nil {
					log.WithError(err).Fatalf("Error while starting server on %s", serverConfig.ListenInterface)
				}
			}(serverConfig)
		}
	}

	sigHandler()
}
