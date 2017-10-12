package guerrilla

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"os"
	"reflect"
	"strings"
)

// AppConfig is the holder of the configuration of the app
type AppConfig struct {
	// Servers can have one or more items.
	/// Defaults to 1 server listening on 127.0.0.1:2525
	Servers []ServerConfig `json:"servers"`
	// AllowedHosts lists which hosts to accept email for. Defaults to os.Hostname
	AllowedHosts []string `json:"allowed_hosts"`
	// PidFile is the path for writing out the process id. No output if empty
	PidFile string `json:"pid_file"`
	// LogFile is where the logs go. Use path to file, or "stderr", "stdout"
	// or "off". Default "stderr"
	LogFile string `json:"log_file,omitempty"`
	// LogLevel controls the lowest level we log.
	// "info", "debug", "error", "panic". Default "info"
	LogLevel string `json:"log_level,omitempty"`
	// BackendConfig configures the email envelope processing backend
	BackendConfig backends.BackendConfig `json:"backend_config"`
}

// ServerConfig specifies config options for a single server
type ServerConfig struct {
	// IsEnabled set to true to start the server, false will ignore it
	IsEnabled bool `json:"is_enabled"`
	// Hostname will be used in the server's reply to HELO/EHLO. If TLS enabled
	// make sure that the Hostname matches the cert. Defaults to os.Hostname()
	Hostname string `json:"host_name"`
	// MaxSize is the maximum size of an email that will be accepted for delivery.
	// Defaults to 10 Mebibytes
	MaxSize int64 `json:"max_size"`
	// PrivateKeyFile path to cert private key in PEM format. Will be ignored if blank
	PrivateKeyFile string `json:"private_key_file"`
	// PublicKeyFile path to cert (public key) chain in PEM format.
	// Will be ignored if blank
	PublicKeyFile string `json:"public_key_file"`
	// Timeout specifies the connection timeout in seconds. Defaults to 30
	Timeout int `json:"timeout"`
	// Listen interface specified in <ip>:<port> - defaults to 127.0.0.1:2525
	ListenInterface string `json:"listen_interface"`
	// StartTLSOn should we offer STARTTLS command. Cert must be valid.
	// False by default
	StartTLSOn bool `json:"start_tls_on,omitempty"`
	// TLSAlwaysOn run this server as a pure TLS server, i.e. SMTPS
	TLSAlwaysOn bool `json:"tls_always_on,omitempty"`
	// MaxClients controls how many maxiumum clients we can handle at once.
	// Defaults to 100
	MaxClients int `json:"max_clients"`
	// LogFile is where the logs go. Use path to file, or "stderr", "stdout" or "off".
	// defaults to AppConfig.Log file setting
	LogFile string `json:"log_file,omitempty"`
	// XClientOn when using a proxy such as Nginx, XCLIENT command is used to pass the
	// original client's IP address & client's HELO
	XClientOn bool `json:"xclient_on,omitempty"`

	// The following used to watch certificate changes so that the TLS can be reloaded
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
	if err = c.setDefaults(); err != nil {
		return err
	}
	if err = c.setBackendDefaults(); err != nil {
		return err
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
	// has backend changed?
	if !reflect.DeepEqual((*c).BackendConfig, (*oldConfig).BackendConfig) {
		app.Publish(EventConfigBackendConfig, c)
	}
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

// setDefaults fills in default server settings for values that were not configured
// The defaults are:
// * Server listening to 127.0.0.1:2525
// * use your hostname to determine your which hosts to accept email for
// * 100 maximum clients
// * 10MB max message size
// * log to Stderr,
// * log level set to "`debug`"
// * timeout to 30 sec
// * Backend configured with the following processors: `HeadersParser|Header|Debugger`
// where it will log the received emails.
func (c *AppConfig) setDefaults() error {
	if c.LogFile == "" {
		c.LogFile = log.OutputStderr.String()
	}
	if c.LogLevel == "" {
		c.LogLevel = "debug"
	}
	if len(c.AllowedHosts) == 0 {
		if h, err := os.Hostname(); err != nil {
			return err
		} else {
			c.AllowedHosts = append(c.AllowedHosts, h)
		}
	}
	h, err := os.Hostname()
	if err != nil {
		return err
	}
	if len(c.Servers) == 0 {
		sc := ServerConfig{}
		sc.LogFile = c.LogFile
		sc.ListenInterface = defaultInterface
		sc.IsEnabled = true
		sc.Hostname = h
		sc.MaxClients = 100
		sc.Timeout = 30
		sc.MaxSize = 10 << 20 // 10 Mebibytes
		c.Servers = append(c.Servers, sc)
	} else {
		// make sure each server has defaults correctly configured
		for i := range c.Servers {
			if c.Servers[i].Hostname == "" {
				c.Servers[i].Hostname = h
			}
			if c.Servers[i].MaxClients == 0 {
				c.Servers[i].MaxClients = 100
			}
			if c.Servers[i].Timeout == 0 {
				c.Servers[i].Timeout = 20
			}
			if c.Servers[i].MaxSize == 0 {
				c.Servers[i].MaxSize = 10 << 20 // 10 Mebibytes
			}
			if c.Servers[i].ListenInterface == "" {
				return errors.New(fmt.Sprintf("Listen interface not specified for server at index %d", i))
			}
			if c.Servers[i].LogFile == "" {
				c.Servers[i].LogFile = c.LogFile
			}
			// validate the server config
			err = c.Servers[i].Validate()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// setBackendDefaults sets default values for the backend config,
// if no backend config was added before starting, then use a default config
// otherwise, see what required values were missed in the config and add any missing with defaults
func (c *AppConfig) setBackendDefaults() error {

	if len(c.BackendConfig) == 0 {
		h, err := os.Hostname()
		if err != nil {
			return err
		}
		c.BackendConfig = backends.BackendConfig{
			"log_received_mails": true,
			"save_workers_size":  1,
			"save_process":       "HeadersParser|Header|Debugger",
			"primary_mail_host":  h,
		}
	} else {
		if _, ok := c.BackendConfig["save_process"]; !ok {
			c.BackendConfig["save_process"] = "HeadersParser|Header|Debugger"
		}
		if _, ok := c.BackendConfig["primary_mail_host"]; !ok {
			h, err := os.Hostname()
			if err != nil {
				return err
			}
			c.BackendConfig["primary_mail_host"] = h
		}
		if _, ok := c.BackendConfig["save_workers_size"]; !ok {
			c.BackendConfig["save_workers_size"] = 1
		}

		if _, ok := c.BackendConfig["log_received_mails"]; !ok {
			c.BackendConfig["log_received_mails"] = false
		}
	}
	return nil
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
func (sc *ServerConfig) Validate() error {
	var errs Errors

	if sc.StartTLSOn || sc.TLSAlwaysOn {
		if sc.PublicKeyFile == "" {
			errs = append(errs, errors.New("PublicKeyFile is empty"))
		}
		if sc.PrivateKeyFile == "" {
			errs = append(errs, errors.New("PrivateKeyFile is empty"))
		}
		if _, err := tls.LoadX509KeyPair(sc.PublicKeyFile, sc.PrivateKeyFile); err != nil {
			errs = append(errs,
				errors.New(fmt.Sprintf("cannot use TLS config for [%s], %v", sc.ListenInterface, err)))
		}
	}
	if len(errs) > 0 {
		return errs
	}

	return nil
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
