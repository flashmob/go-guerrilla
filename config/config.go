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
func ReadConfig(configFile, iface string, verbose bool, mainConfig *guerrilla.Config) error {
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

	// TODO: deprecate
	if len(iface) > 0 && len(mainConfig.Servers) > 0 {
		mainConfig.Servers[0].ListenInterface = iface
	}

	guerrilla.ConfigLoadTime = time.Now()
	return nil
}
