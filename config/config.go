package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"time"

	guerrilla "github.com/flashmob/go-guerrilla"
)

// ReadConfig which should be called at startup, or when a SIG_HUP is caught
func ReadConfig(configFile, iface string, pidFile string, mainConfig *guerrilla.Config) error {
	// load in the config.
	b, err := ioutil.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("could not read config file: %s", err)
	}

	err = json.Unmarshal(b, &mainConfig)
	if err != nil {
		return fmt.Errorf("could not parse config file: %s", err)
	}

	if len(mainConfig.AllowedHosts) == 0 {
		return errors.New("empty AllowedHosts is not allowed")
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
