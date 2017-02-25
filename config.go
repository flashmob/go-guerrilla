package guerrilla

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
)

// AppConfig is the holder of the configuration of the app
type AppConfig struct {
	Servers      []ServerConfig `json:"servers"`
	AllowedHosts []string       `json:"allowed_hosts"`
	PidFile      string         `json:"pid_file"`
	LogFile      string         `json:"log_file,omitempty"`
	LogLevel     string         `json:"log_level,omitempty"`
}

// ServerConfig specifies config options for a single server
type ServerConfig struct {
	IsEnabled       bool   `json:"is_enabled"`
	Hostname        string `json:"host_name"`
	MaxSize         int64  `json:"max_size"`
	PrivateKeyFile  string `json:"private_key_file"`
	PublicKeyFile   string `json:"public_key_file"`
	Timeout         int    `json:"timeout"`
	ListenInterface string `json:"listen_interface"`
	StartTLSOn      bool   `json:"start_tls_on,omitempty"`
	TLSAlwaysOn     bool   `json:"tls_always_on,omitempty"`
	MaxClients      int    `json:"max_clients"`
	LogFile         string `json:"log_file,omitempty"`

	_privateKeyFile_mtime int
	_publicKeyFile_mtime  int
}

// Unmarshalls json data into AppConfig struct and any other initialization of the struct
// also does validation, returns error if validation failed or something went wrong
func (c *AppConfig) Load(jsonBytes []byte) error {
	err := json.Unmarshal(jsonBytes, c)
	if err != nil {
		return fmt.Errorf("could not parse config file: %s", err)
	}
	if len(c.AllowedHosts) == 0 {
		return errors.New("empty AllowedHosts is not allowed")
	}

	// all servers must be valid in order to continue
	for _, server := range c.Servers {
		if errs := server.Validate(); errs != nil {
			return errs
		}
	}

	// read the timestamps for the ssl keys, to determine if they need to be reloaded
	for i := 0; i < len(c.Servers); i++ {
		c.Servers[i].loadTlsKeyTimestamps()
	}
	return nil
}

// Emits any configuration change events onto the event bus.
func (c *AppConfig) EmitChangeEvents(oldConfig *AppConfig, app Guerrilla) {
	// has config changed, general check
	if !reflect.DeepEqual(oldConfig, c) {
		app.Publish(EventConfigNewConfig, c)
	}
	// has 'allowed hosts' changed?
	if !reflect.DeepEqual(oldConfig.AllowedHosts, c.AllowedHosts) {
		app.Publish(EventConfigAllowedHosts, c)
	}
	// has pid file changed?
	if strings.Compare(oldConfig.PidFile, c.PidFile) != 0 {
		app.Publish(EventConfigPidFile, c)
	}
	// has mainlog log changed?
	if strings.Compare(oldConfig.LogFile, c.LogFile) != 0 {
		app.Publish(EventConfigLogFile, c)
	} else {
		// since config file has not changed, we reload it
		app.Publish(EventConfigLogReopen, c)
	}
	// has log level changed?
	if strings.Compare(oldConfig.LogLevel, c.LogLevel) != 0 {
		app.Publish(EventConfigLogLevel, c)
	}
	// server config changes
	oldServers := oldConfig.getServers()
	for iface, newServer := range c.getServers() {
		// is server is in both configs?
		if oldServer, ok := oldServers[iface]; ok {
			// since old server exists in the new config, we do not track it anymore
			delete(oldServers, iface)
			// so we know the server exists in both old & new configs
			newServer.emitChangeEvents(oldServer, app)
		} else {
			// start new server
			app.Publish(EventConfigServerNew, newServer)
		}

	}
	// remove any servers that don't exist anymore
	for _, oldserver := range oldServers {
		app.Publish(EventConfigServerRemove, oldserver)
	}
}

// EmitLogReopen emits log reopen events using existing config
func (c *AppConfig) EmitLogReopenEvents(app Guerrilla) {
	app.Publish(EventConfigLogReopen, c)
	for _, sc := range c.getServers() {
		app.Publish(EventConfigServerLogReopen, sc)
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

// Emits any configuration change events on the server.
// All events are fired and run synchronously
func (sc *ServerConfig) emitChangeEvents(oldServer *ServerConfig, app Guerrilla) {
	// get a list of changes
	changes := getDiff(
		*oldServer,
		*sc,
	)
	if len(changes) > 0 {
		// something changed in the server config
		app.Publish(EventConfigServerConfig, sc)
	}

	// enable or disable?
	if _, ok := changes["IsEnabled"]; ok {
		if sc.IsEnabled {
			app.Publish(EventConfigServerStart, sc)
		} else {
			app.Publish(EventConfigServerStop, sc)
		}
		// do not emit any more events when IsEnabled changed
		return
	}
	// log file change?
	if _, ok := changes["LogFile"]; ok {
		app.Publish(EventConfigServerLogFile, sc)
	} else {
		// since config file has not changed, we reload it
		app.Publish(EventConfigServerLogReopen, sc)
	}
	// timeout changed
	if _, ok := changes["Timeout"]; ok {
		app.Publish(EventConfigServerTimeout, sc)
	}
	// max_clients changed
	if _, ok := changes["MaxClients"]; ok {
		app.Publish(EventConfigServerMaxClients, sc)
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
		app.Publish(EventConfigServerTLSConfig, sc)
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

// Validate validates the server's configuration.
func (sc *ServerConfig) Validate() Errors {
	var errs Errors
	if _, err := tls.LoadX509KeyPair(sc.PublicKeyFile, sc.PrivateKeyFile); err != nil {
		if sc.StartTLSOn || sc.TLSAlwaysOn {
			errs = append(errs,
				errors.New(fmt.Sprintf("cannot use TLS config for [%s], %v", sc.ListenInterface, err)))
		}

	}
	return errs
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
