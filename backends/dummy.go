package backends

func init() {
	// decorator pattern
	backends["dummy"] = &AbstractBackend{
		extend: &DummyBackend{},
	}
}

// custom configuration we will parse from the json
// see guerrillaDBAndRedisConfig struct for a more complete example
type dummyConfig struct {
	LogReceivedMails bool `json:"log_received_mails"`
}

// putting all the paces we need together
type DummyBackend struct {
	config dummyConfig
	// embed functions form AbstractBackend so that DummyBackend satisfies the Backend interface
	AbstractBackend
}

// Backends should implement this method and set b.config field with a custom config struct
// Therefore, your implementation would have a custom config type instead of dummyConfig
func (b *DummyBackend) loadConfig(backendConfig BackendConfig) (err error) {
	// Load the backend config for the backend. It has already been unmarshalled
	// from the main config file 'backend' config "backend_config"
	// Now we need to convert each type and copy into the dummyConfig struct
	configType := baseConfig(&dummyConfig{})
	bcfg, err := b.extractConfig(backendConfig, configType)
	if err != nil {
		return err
	}
	m := bcfg.(*dummyConfig)
	b.config = *m
	return nil
}
