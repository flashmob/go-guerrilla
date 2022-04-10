package guerrilla

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
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
	// TLS Configuration
	TLS ServerTLSConfig `json:"tls,omitempty"`
	// LogFile is where the logs go. Use path to file, or "stderr", "stdout" or "off".
	// defaults to AppConfig.Log file setting
	LogFile string `json:"log_file,omitempty"`
	// Hostname will be used in the server's reply to HELO/EHLO. If TLS enabled
	// make sure that the Hostname matches the cert. Defaults to os.Hostname()
	// Hostname will also be used to fill the 'Host' property when the "RCPT TO" address is
	// addressed to just <postmaster>
	Hostname string `json:"host_name"`
	// Listen interface specified in <ip>:<port> - defaults to 127.0.0.1:2525
	ListenInterface string `json:"listen_interface"`
	// MaxSize is the maximum size of an email that will be accepted for delivery.
	// Defaults to 10 Mebibytes
	MaxSize int64 `json:"max_size"`
	// Timeout specifies the connection timeout in seconds. Defaults to 30
	Timeout int `json:"timeout"`
	// MaxClients controls how many maximum clients we can handle at once.
	// Defaults to defaultMaxClients
	MaxClients int `json:"max_clients"`
	// IsEnabled set to true to start the server, false will ignore it
	IsEnabled bool `json:"is_enabled"`
	// XClientOn when using a proxy such as Nginx, XCLIENT command is used to pass the
	// original client's IP address & client's HELO
	XClientOn bool `json:"xclient_on,omitempty"`
	// Greeting is the custom greeting.
	Greeting string `json:"greeting"`
}

type ServerTLSConfig struct {
	// TLS Protocols to use. [0] = min, [1]max
	// Use Go's default if empty
	Protocols []string `json:"protocols,omitempty"`
	// TLS Ciphers to use.
	// Use Go's default if empty
	Ciphers []string `json:"ciphers,omitempty"`
	// TLS Curves to use.
	// Use Go's default if empty
	Curves []string `json:"curves,omitempty"`
	// PrivateKeyFile path to cert private key in PEM format.
	PrivateKeyFile string `json:"private_key_file"`
	// PublicKeyFile path to cert (public key) chain in PEM format.
	PublicKeyFile string `json:"public_key_file"`
	// TLS Root cert authorities to use. "A PEM encoded CA's certificate file.
	// Defaults to system's root CA file if empty
	RootCAs string `json:"root_cas_file,omitempty"`
	// declares the policy the server will follow for TLS Client Authentication.
	// Use Go's default if empty
	ClientAuthType string `json:"client_auth_type,omitempty"`
	// The following used to watch certificate changes so that the TLS can be reloaded
	_privateKeyFileMtime int64
	_publicKeyFileMtime  int64
	// controls whether the server selects the
	// client's most preferred cipher suite
	PreferServerCipherSuites bool `json:"prefer_server_cipher_suites,omitempty"`
	// StartTLSOn should we offer STARTTLS command. Cert must be valid.
	// False by default
	StartTLSOn bool `json:"start_tls_on,omitempty"`
	// AlwaysOn run this server as a pure TLS server, i.e. SMTPS
	AlwaysOn bool `json:"tls_always_on,omitempty"`
}

// https://golang.org/pkg/crypto/tls/#pkg-constants
// Ciphers introduced before Go 1.7 are listed here,
// ciphers since Go 1.8, see tls_go1.8.go
// ....... since Go 1.13, see tls_go1.13.go
var TLSCiphers = map[string]uint16{

	// Note: Generally avoid using CBC unless for compatibility
	// The following ciphersuites are not configurable for TLS 1.3
	// see tls_go1.13.go for a list of ciphersuites always used in TLS 1.3

	"TLS_RSA_WITH_3DES_EDE_CBC_SHA":        tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	"TLS_RSA_WITH_AES_128_CBC_SHA":         tls.TLS_RSA_WITH_AES_128_CBC_SHA,
	"TLS_RSA_WITH_AES_256_CBC_SHA":         tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA": tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA": tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	"TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA":  tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA":   tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA":   tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,

	"TLS_RSA_WITH_RC4_128_SHA":        tls.TLS_RSA_WITH_RC4_128_SHA,
	"TLS_RSA_WITH_AES_128_GCM_SHA256": tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	"TLS_RSA_WITH_AES_256_GCM_SHA384": tls.TLS_RSA_WITH_AES_256_GCM_SHA384,

	"TLS_ECDHE_ECDSA_WITH_RC4_128_SHA":        tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
	"TLS_ECDHE_RSA_WITH_RC4_128_SHA":          tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
	"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,

	// see tls_go1.13 for new TLS 1.3 ciphersuites
	// Note that TLS 1.3 ciphersuites are not configurable
}

// https://golang.org/pkg/crypto/tls/#pkg-constants
var TLSProtocols = map[string]uint16{
	"tls1.0": tls.VersionTLS10,
	"tls1.1": tls.VersionTLS11,
	"tls1.2": tls.VersionTLS12,
}

// https://golang.org/pkg/crypto/tls/#CurveID
var TLSCurves = map[string]tls.CurveID{
	"P256": tls.CurveP256,
	"P384": tls.CurveP384,
	"P521": tls.CurveP521,
}

// https://golang.org/pkg/crypto/tls/#ClientAuthType
var TLSClientAuthTypes = map[string]tls.ClientAuthType{
	"NoClientCert":               tls.NoClientCert,
	"RequestClientCert":          tls.RequestClientCert,
	"RequireAnyClientCert":       tls.RequireAnyClientCert,
	"VerifyClientCertIfGiven":    tls.VerifyClientCertIfGiven,
	"RequireAndVerifyClientCert": tls.RequireAndVerifyClientCert,
}

const defaultMaxClients = 100
const defaultTimeout = 30
const defaultInterface = "127.0.0.1:2525"
const defaultMaxSize = int64(10 << 20) // 10 Mebibytes

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

	// read the timestamps for the TLS keys, to determine if they need to be reloaded
	for i := 0; i < len(c.Servers); i++ {
		if err := c.Servers[i].loadTlsKeyTimestamps(); err != nil {
			return err
		}
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
	for _, oldServer := range oldServers {
		app.Publish(EventConfigServerRemove, oldServer)
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
		sc.MaxClients = defaultMaxClients
		sc.Timeout = defaultTimeout
		sc.MaxSize = defaultMaxSize
		c.Servers = append(c.Servers, sc)
	} else {
		// make sure each server has defaults correctly configured
		for i := range c.Servers {
			if c.Servers[i].Hostname == "" {
				c.Servers[i].Hostname = h
			}
			if c.Servers[i].MaxClients == 0 {
				c.Servers[i].MaxClients = defaultMaxClients
			}
			if c.Servers[i].Timeout == 0 {
				c.Servers[i].Timeout = defaultTimeout
			}
			if c.Servers[i].MaxSize == 0 {
				c.Servers[i].MaxSize = defaultMaxSize // 10 Mebibytes
			}
			if c.Servers[i].ListenInterface == "" {
				return fmt.Errorf("listen interface not specified for server at index %d", i)
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
	changes := getChanges(
		*oldServer,
		*sc,
	)
	tlsChanges := getChanges(
		(*oldServer).TLS,
		(*sc).TLS,
	)

	if len(changes) > 0 || len(tlsChanges) > 0 {
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

	if len(tlsChanges) > 0 {
		app.Publish(EventConfigServerTLSConfig, sc)
	}
}

// Loads in timestamps for the TLS keys
func (sc *ServerConfig) loadTlsKeyTimestamps() error {
	var statErr = func(iface string, err error) error {
		return fmt.Errorf(
			"could not stat key for server [%s], %s",
			iface,
			err.Error())
	}
	if sc.TLS.PrivateKeyFile == "" {
		sc.TLS._privateKeyFileMtime = time.Now().Unix()
		return nil
	}
	if sc.TLS.PublicKeyFile == "" {
		sc.TLS._publicKeyFileMtime = time.Now().Unix()
		return nil
	}
	if info, err := os.Stat(sc.TLS.PrivateKeyFile); err == nil {
		sc.TLS._privateKeyFileMtime = info.ModTime().Unix()
	} else {
		return statErr(sc.ListenInterface, err)
	}
	if info, err := os.Stat(sc.TLS.PublicKeyFile); err == nil {
		sc.TLS._publicKeyFileMtime = info.ModTime().Unix()
	} else {
		return statErr(sc.ListenInterface, err)
	}
	return nil
}

// Validate validates the server's configuration.
func (sc *ServerConfig) Validate() error {
	var errs Errors

	if sc.TLS.StartTLSOn || sc.TLS.AlwaysOn {
		if sc.TLS.PublicKeyFile == "" {
			errs = append(errs, errors.New("PublicKeyFile is empty"))
		}
		if sc.TLS.PrivateKeyFile == "" {
			errs = append(errs, errors.New("PrivateKeyFile is empty"))
		}
		if _, err := tls.LoadX509KeyPair(sc.TLS.PublicKeyFile, sc.TLS.PrivateKeyFile); err != nil {
			errs = append(errs, fmt.Errorf("cannot use TLS config for [%s], %v", sc.ListenInterface, err))
		}
	}
	if len(errs) > 0 {
		return errs
	}

	return nil
}

// Gets the timestamp of the TLS certificates. Returns a unix time of when they were last modified
// when the config was read. We use this info to determine if TLS needs to be re-loaded.
func (stc *ServerTLSConfig) getTlsKeyTimestamps() (int64, int64) {
	return stc._privateKeyFileMtime, stc._publicKeyFileMtime
}

// Returns value changes between struct a & struct b.
// Results are returned in a map, where each key is the name of the field that was different.
// a and b are struct values, must not be pointer
// and of the same struct type
func getChanges(a interface{}, b interface{}) map[string]interface{} {
	ret := make(map[string]interface{}, 5)
	compareWith := structtomap(b)
	for key, val := range structtomap(a) {
		if sliceOfStr, ok := val.([]string); ok {
			val, _ = json.Marshal(sliceOfStr)
			val = string(val.([]uint8))
		}
		if sliceOfStr, ok := compareWith[key].([]string); ok {
			compareWith[key], _ = json.Marshal(sliceOfStr)
			compareWith[key] = string(compareWith[key].([]uint8))
		}
		if val != compareWith[key] {
			ret[key] = compareWith[key]
		}
	}
	// detect changes to TLS keys (have the key files been modified?)
	if oldTLS, ok := a.(ServerTLSConfig); ok {
		t1, t2 := oldTLS.getTlsKeyTimestamps()
		if newTLS, ok := b.(ServerTLSConfig); ok {
			t3, t4 := newTLS.getTlsKeyTimestamps()
			if t1 != t3 {
				ret["PrivateKeyFile"] = newTLS.PrivateKeyFile
			}
			if t2 != t4 {
				ret["PublicKeyFile"] = newTLS.PublicKeyFile
			}
		}
	}
	return ret
}

// Convert fields of a struct to a map
// only able to convert int, bool, slice-of-strings and string; not recursive
// slices are marshal'd to json for convenient comparison later
func structtomap(obj interface{}) map[string]interface{} {
	ret := make(map[string]interface{})
	v := reflect.ValueOf(obj)
	t := v.Type()
	for index := 0; index < v.NumField(); index++ {
		vField := v.Field(index)
		fName := t.Field(index).Name
		k := vField.Kind()
		switch k {
		case reflect.Int:
			fallthrough
		case reflect.Int64:
			value := vField.Int()
			ret[fName] = value
		case reflect.String:
			value := vField.String()
			ret[fName] = value
		case reflect.Bool:
			value := vField.Bool()
			ret[fName] = value
		case reflect.Slice:
			ret[fName] = vField.Interface().([]string)
		}
	}
	return ret
}
