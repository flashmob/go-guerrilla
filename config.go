package guerrilla

// AppConfig is the holder of the configuration of the app
type AppConfig struct {
	Backend      Backend
	Servers      []*ServerConfig `json:"servers"`
	AllowedHosts []string        `json:"allowed_hosts"`
}

// ServerConfig specifies config options for a single server
type ServerConfig struct {
	IsEnabled       bool     `json:"is_enabled"`
	Hostname        string   `json:"host_name"`
	AllowedHosts    []string `json:"allowed_hosts"`
	MaxSize         int64    `json:"max_size"`
	PrivateKeyFile  string   `json:"private_key_file"`
	PublicKeyFile   string   `json:"public_key_file"`
	Timeout         int      `json:"timeout"`
	ListenInterface string   `json:"listen_interface"`
	AdvertiseTLS    bool     `json:"advertise_tls,omitempty"`
	RequireTLS      bool     `json:"require_tls,omitempty"`
	MaxClients      int      `json:"max_clients"`
}
