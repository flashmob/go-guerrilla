package guerrilla

import "strings"

type BackendConfig map[string]interface{}

// Config is the holder of the configuration of the app
type Config struct {
	BackendName   string         `json:"backend_name"`
	BackendConfig BackendConfig  `json:"backend_config,omitempty"`
	Servers       []ServerConfig `json:"servers"`
	AllowedHosts  string         `json:"allowed_hosts"`

	_allowedHosts map[string]bool
}

func (c *Config) IsAllowed(host string) bool {
	if c._allowedHosts == nil {
		arr := strings.Split(c.AllowedHosts, ",")
		c._allowedHosts = make(map[string]bool, len(arr))
		for _, h := range arr {
			c._allowedHosts[strings.ToLower(h)] = true
		}
	}
	return c._allowedHosts[strings.ToLower(host)]
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
