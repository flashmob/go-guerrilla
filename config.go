package guerrilla

import (
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"os"
	"reflect"
	"strings"
)

// AppConfig is the holder of the configuration of the app
type AppConfig struct {
	Servers      []ServerConfig `json:"servers"`
	AllowedHosts []string       `json:"allowed_hosts"`
	PidFile      string         `json:"pid_file"`
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
	StartTLSOn      bool     `json:"start_tls_on,omitempty"`
	TLSAlwaysOn     bool     `json:"tls_always_on,omitempty"`
	MaxClients      int      `json:"max_clients"`
	LogFile         string   `json:"log_file,omitempty"`

	_privateKeyFile_mtime int
	_publicKeyFile_mtime  int
}

// Unmarshalls json data into AppConfig struct and any other initialization of the struct
func (c *AppConfig) Load(jsonBytes []byte) error {
	err := json.Unmarshal(jsonBytes, c)
	if err != nil {
		return fmt.Errorf("could not parse config file: %s", err)
	}
	if len(c.AllowedHosts) == 0 {
		return errors.New("empty AllowedHosts is not allowed")
	}

	// read the timestamps for the ssl keys, to determine if they need to be reloaded
	for i := 0; i < len(c.Servers); i++ {
		if err := c.Servers[i].loadTlsKeyTimestamps(); err != nil {
			return err
		}
	}
	return nil
}

// Emits any configuration change events onto the event bus.
func (c *AppConfig) EmitChangeEvents(oldConfig *AppConfig) {
	// has 'allowed hosts' changed?
	if !reflect.DeepEqual(oldConfig.AllowedHosts, c.AllowedHosts) {
		Bus.Publish("config_change:allowed_hosts", c)
	}
	// has pid file changed?
	if strings.Compare(oldConfig.PidFile, c.PidFile) != 0 {
		Bus.Publish("config_change:pid_file", c)
	}
	// server config changes
	oldServers := oldConfig.getServers()
	for iface, newServer := range c.getServers() {
		// is server is in both configs?
		if oldServer, ok := oldServers[iface]; ok {
			// since old server exists in the new config, we do not track it anymore
			delete(oldServers, iface)
			newServer.emitChangeEvents(oldServer)
		} else {
			// start new server
			Bus.Publish("server_change:start_server", newServer)
		}

	}
	// stop any servers that don't exist anymore
	for _, oldserver := range oldServers {
		log.Infof("Server [%s] removed from config, stopping", oldserver.ListenInterface)
		Bus.Publish("server_change:"+oldserver.ListenInterface+":stop_server", oldserver)
	}
}

// gets the servers in a map (key by interface) for easy lookup
func (c *AppConfig) getServers() map[string]*ServerConfig {
	servers := make(map[string]*ServerConfig, len(c.Servers))
	for i := 0; i < len(c.Servers); i++ {
		servers[c.Servers[i].ListenInterface] = &c.Servers[i]
	}
	return servers
}

// Emits any configuration change events on the server
func (sc *ServerConfig) emitChangeEvents(oldServer *ServerConfig) {
	// get a list of changes
	changes := getDiff(
		*oldServer,
		*sc,
	)

	// enable or disable?
	if _, ok := changes["IsEnabled"]; ok {
		if sc.IsEnabled {
			Bus.Publish("server_change:start_server", sc)
		} else {
			Bus.Publish("server_change:"+sc.ListenInterface+":stop_server", sc)
		}
		// do not emit any more events when IsEnabled changed
		return
	}
	// log file change?
	if _, ok := changes["LogFile"]; ok {
		Bus.Publish("server_change:"+sc.ListenInterface+":new_log_file", sc)
	} else {
		// since config file has not changed, we reload it
		Bus.Publish("server_change:"+sc.ListenInterface+":reopen_log_file", sc)
	}
	// timeout changed
	if _, ok := changes["Timeout"]; ok {
		Bus.Publish("server_change:"+sc.ListenInterface+":timeout", sc)
	}
	// max_clients changed
	if _, ok := changes["Timeout"]; ok {
		Bus.Publish("server_change:"+sc.ListenInterface+":max_clients", sc)
	}

	// tls changed
	if ok := func() bool {
		if _, ok := changes["PrivateKeyFile"]; ok {
			return true
		}
		if _, ok := changes["PublicKeyFile"]; ok {
			return true
		}
		if _, ok := changes["StartTLSOn"]; ok {
			return true
		}
		if _, ok := changes["TLSAlwaysOn"]; ok {
			return true
		}
		return false
	}(); ok {
		Bus.Publish("server_change:"+sc.ListenInterface+":tls_config", sc)
	}
}

// Loads in timestamps for the ssl keys
func (sc *ServerConfig) loadTlsKeyTimestamps() error {
	var statErr = func(iface string, err error) error {
		return errors.New(
			fmt.Sprintf(
				"could not stat key for server [%s], %s",
				iface,
				err.Error()))
	}
	if info, err := os.Stat(sc.PrivateKeyFile); err == nil {
		sc._privateKeyFile_mtime = info.ModTime().Second()
	} else {
		return statErr(sc.ListenInterface, err)
	}
	if info, err := os.Stat(sc.PublicKeyFile); err == nil {
		sc._publicKeyFile_mtime = info.ModTime().Second()
	} else {
		return statErr(sc.ListenInterface, err)
	}
	return nil
}

// Gets the timestamp of the TLS certificates. Returns a unix time of when they were last modified
// when the config was read. We use this info to determine if TLS needs to be re-loaded.
func (sc *ServerConfig) getTlsKeyTimestamps() (int, int) {
	return sc._privateKeyFile_mtime, sc._publicKeyFile_mtime
}

// Returns a diff between struct a & struct b.
// Results are returned in a map, where each key is the name of the field that was different.
// a and b are struct values, must not be pointer
// and of the same struct type
func getDiff(a interface{}, b interface{}) map[string]interface{} {
	ret := make(map[string]interface{}, 5)
	compareWith := structtomap(b)
	for key, val := range structtomap(a) {
		if val != compareWith[key] {
			ret[key] = compareWith[key]
		}
	}
	// detect tls changes (have the key files been modified?)
	if oldServer, ok := a.(ServerConfig); ok {
		t1, t2 := oldServer.getTlsKeyTimestamps()
		if newServer, ok := b.(ServerConfig); ok {
			t3, t4 := newServer.getTlsKeyTimestamps()
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

// Convert fields of a struct to a map
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
