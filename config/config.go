package config

import (
	"time"

	guerrilla "github.com/flashmob/go-guerrilla"
)

// ReadConfig which should be called at startup, or when a SIG_HUP is caught
func ReadConfig(configFile, iface string, pidFile string, mainConfig *guerrilla.Config) error {
	if err := mainConfig.Load(configFile); err != nil {
		return err
	}
	// Use the iface passed form command-line rather rather than config file
	if len(iface) > 0 && len(mainConfig.Servers) > 0 {
		mainConfig.Servers[0].ListenInterface = iface
	}
	// same for the file that stores our process id
	if len(pidFile) > 0 {
		mainConfig.PidFile = pidFile
	}
	guerrilla.ConfigLoadTime = time.Now()
	return nil
}
