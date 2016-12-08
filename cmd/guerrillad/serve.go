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
	"reflect"
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
			// save old config & load in new one
			oldConfig := mainConfig
			newConfig := guerrilla.Config{}
			err := config.ReadConfig(configFile, iface, pidFile, &newConfig)
			if err != nil {
				log.WithError(err).Error("Error while ReadConfig (reload)")
			} else {
				// mainConfig becomes the new one
				mainConfig = newConfig
				emitConfigChangeEvents(oldConfig, mainConfig)
				log.Infof("Configuration was reloaded at %s", guerrilla.ConfigLoadTime)
			}
		} else {
			os.Exit(0)
		}
	}
}

func emitConfigChangeEvents(oldConfig guerrilla.Config, newConfig guerrilla.Config) {

	bus.Publish("config_change", newConfig)
	// has 'allowed hosts' changed?
	if strings.Compare(oldConfig.AllowedHosts, newConfig.AllowedHosts) != 0 {
		bus.Publish("config_change:allowed_hosts", newConfig)
	}
	// has pid file changed?
	if strings.Compare(oldConfig.PidFile, newConfig.PidFile) != 0 {
		bus.Publish("config_change:pid_file", newConfig)
	}

	// has backend changed?
	if !reflect.DeepEqual(oldConfig.BackendConfig, newConfig.BackendConfig) {
		bus.Publish("config_change:backend_config", newConfig)
	}

	// server config changes
	oldServers := oldConfig.GetServers()

	for iface, newServer := range newConfig.GetServers() {
		// is server is in both configs?
		if oldServer, ok := oldServers[iface]; ok {
			changes := guerrilla.GetChanges(
				*oldServer,
				*newServer)
			// since old server exists in the new config, we do not track it anymore
			delete(oldServers, iface)
			// enable or disable?
			if _, ok := changes["IsEnabled"]; ok {
				if newServer.IsEnabled {
					bus.Publish("config_change:start_server", newServer)
				} else {
					bus.Publish("config_change:" + iface + ":stop_server")
				}
				// do not emit any more events when IsEnabled changed
				continue
			}
			// log file change?
			if _, ok := changes["LogFile"]; ok {
				bus.Publish("config_change:"+iface+":new_log_file", newServer)
			} else {
				// since config file has not changed, we reload it
				bus.Publish("config_change:"+iface+":reopen_log_file", newServer)
			}
			// timeout changed
			if _, ok := changes["Timeout"]; ok {
				bus.Publish("config_change:"+iface+":timeout", newServer)
			}
			// tls changed
			if ok := func() bool {
				if _, ok := changes["PrivateKeyFile"]; ok {
					return true
				}
				if _, ok := changes["PublicKeyFile"]; ok {
					return true
				}
				if _, ok := changes["StartTLS"]; ok {
					return true
				}
				if _, ok := changes["TLSAlwaysOn"]; ok {
					return true
				}

				return false
			}(); ok {
				bus.Publish("config_change:"+iface+":tls_config", *newServer)
			}

		} else {
			// start new server
			bus.Publish("config_change:start_server", newServer)
		}

	}
	// stop any servers that don't exist anymore
	for iface := range oldServers {
		log.Infof("Server [%s] removed from config, stopping", iface)
		bus.Publish("config_change:" + iface + ":stop_server")
	}

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
	// ...and write out our pid whenever the file name changes in the config
	bus.Subscribe("config_change:pid_file", func(mainConfig guerrilla.Config) {
		writePid(mainConfig.PidFile)
	})
	// launch backed for saving email
	backend, err := backends.New(mainConfig.BackendName, mainConfig.BackendConfig)
	if err != nil {
		log.WithError(err).Fatalf("Error while loading the backend %q",
			mainConfig.BackendName)
	}
	bus.Subscribe("config_change:backend_config", func(mainConfig guerrilla.Config) {
		backend.Finalize()
		backend, err = backends.New(mainConfig.BackendName, mainConfig.BackendConfig)
		if err != nil {
			log.WithError(err).Fatalf("Error while loading the backend %q",
				mainConfig.BackendName)
		}
	})
	// run our servers
	start := func(sConfig guerrilla.ServerConfig) {
		log.Infof("Starting server on %s", sConfig.ListenInterface)
		err := server.RunServer(mainConfig, sConfig, backend, bus)
		if err != nil {
			log.WithError(err).Fatalf("Error while starting server on %s", sConfig.ListenInterface)
		}
	}
	for _, serverConfig := range mainConfig.Servers {
		if serverConfig.IsEnabled {
			go start(serverConfig)
		}
	}
	// start a new server when added to config
	bus.Subscribe("config_change:start_server", func(sConfig *guerrilla.ServerConfig) {
		go start(*sConfig)
	})
	// trap & handle signals
	sigHandler()
}
