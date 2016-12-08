package guerrilla

import (
	"reflect"
	"strings"

	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
)

type BackendConfig map[string]interface{}

// Config is the holder of the configuration of the app
type Config struct {
	BackendName   string         `json:"backend_name"`
	BackendConfig BackendConfig  `json:"backend_config,omitempty"`
	Servers       []ServerConfig `json:"servers"`
	AllowedHosts  string         `json:"allowed_hosts"`
	PidFile       string         `json:"pid_file,omitempty"`

	_allowedHosts map[string]bool
	_servers      map[string]*ServerConfig
}

// load in the config.
func (c *Config) Load(filename string) error {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("could not read config file: %s", err)
	}
	err = json.Unmarshal(b, c)
	if err != nil {
		return fmt.Errorf("could not parse config file: %s", err)
	}
	if len(c.AllowedHosts) == 0 {
		return errors.New("empty AllowedHosts is not allowed")
	}
	// read the timestamps for the ssl keys, to determine if they need to be reloaded
	for i := 0; i < len(c.Servers); i++ {
		if info, err := os.Stat(c.Servers[i].PrivateKeyFile); err == nil {
			c.Servers[i]._privateKeyFile_mtime = info.ModTime().Second()
		}
		if info, err := os.Stat(c.Servers[i].PublicKeyFile); err == nil {
			c.Servers[i]._publicKeyFile_mtime = info.ModTime().Second()
		}
	}
	return nil
}

// do we accept this 'rcpt to' host?
func (c *Config) IsHostAllowed(host string) bool {
	if c._allowedHosts == nil {
		// unpack from the config
		arr := strings.Split(c.AllowedHosts, ",")
		c._allowedHosts = make(map[string]bool, len(arr))
		for _, h := range arr {
			c._allowedHosts[strings.ToLower(h)] = true
		}
	}
	return c._allowedHosts[strings.ToLower(host)]
}

func (c *Config) ResetAllowedHosts() {
	c._allowedHosts = nil
}

// gets the servers in a map for easy lookup
// key by interface
func (c *Config) GetServers() map[string]*ServerConfig {
	if c._servers == nil {
		c._servers = make(map[string]*ServerConfig, len(c.Servers))
		for i := 0; i < len(c.Servers); i++ {
			c._servers[c.Servers[i].ListenInterface] = &c.Servers[i]
		}
	}
	return c._servers
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
	LogFile         string `json:"log_file,omitempty"`

	_privateKeyFile_mtime int
	_publicKeyFile_mtime  int
}

func (sc *ServerConfig) GetTlsKeyTimestamps() (int, int) {
	return sc._privateKeyFile_mtime, sc._publicKeyFile_mtime
}

// Return a map with with items whose values changed
// obj1 and obj2 are struct values, must not be pointer
// assuming obj1 and obj2 are of the same struct type
func GetChanges(obj1 interface{}, obj2 interface{}) map[string]interface{} {
	ret := make(map[string]interface{}, 5)
	compareWith := structtomap(obj2)
	for key, val := range structtomap(obj1) {
		if val != compareWith[key] {
			ret[key] = compareWith[key]
		}
	}
	// detect tls changes (have the key files been modified?)
	if oldServer, ok := obj1.(ServerConfig); ok {
		t1, t2 := oldServer.GetTlsKeyTimestamps()
		if newServer, ok := obj2.(ServerConfig); ok {
			t3, t4 := newServer.GetTlsKeyTimestamps()
			if t1 != t3 {
				ret["PrivateKeyFile"] = newServer.PrivateKeyFile
			}
			if t2 != t4 {
				ret["PublicKeyFile"] = newServer.PublicKeyFile
			}
		}
	}
	return ret
}

// only able to convert int, bool and string; not recursive
func structtomap(obj interface{}) map[string]interface{} {
	ret := make(map[string]interface{}, 0)
	v := reflect.ValueOf(obj)
	t := v.Type()
	for index := 0; index < v.NumField(); index++ {
		vField := v.Field(index)
		fName := t.Field(index).Name

		switch vField.Kind() {
		case reflect.Int:
			value := vField.Int()
			ret[fName] = value
		case reflect.String:
			value := vField.String()
			ret[fName] = value
		case reflect.Bool:
			value := vField.Bool()
			ret[fName] = value
		}
	}
	return ret

}
