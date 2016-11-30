package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/jordanschalm/guerrilla"
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

	appConfig     = guerrilla.AppConfig{}
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
			err := guerrilla.ReadConfig(configPath, verbose, &appConfig)
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

	err := guerrilla.ReadConfig(configPath, verbose, &appConfig)
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

	go guerrilla.Run(&appConfig)
	sigHandler()
}
