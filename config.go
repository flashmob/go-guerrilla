package guerrilla

type BackendConfig map[string]interface{}

// Config is the holder of the configuration of the app
type Config struct {
	BackendName   string         `json:"backend_name"`
	BackendConfig BackendConfig  `json:"backend_config,omitempty"`
	Servers       []ServerConfig `json:"servers"`
	AllowedHosts  string         `json:"allowed_hosts"`
}

// ServerConfig is the holder of the configuration of a server
type ServerConfig struct {
	IsEnabled       bool   `json:"is_enabled"`
	Hostname        string `json:"host_name"`
	MaxSize         int    `json:"max_size"`
	PrivateKeyFile  string `json:"private_key_file"`
	PublicKeyFile   string `json:"public_key_file"`
	Timeout         int    `json:"timeout"`
	ListenInterface string `json:"listen_interface"`
	StartTLS        bool   `json:"start_tls_on,omitempty"`
	TLSAlwaysOn     bool   `json:"tls_always_on,omitempty"`
	MaxClients      int    `json:"max_clients"`
}
