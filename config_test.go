package guerrilla

import (
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/tests/testcert"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"
)

// a configuration file with a dummy backend

//
var configJsonA = `
{
    "log_file" : "./tests/testlog",
    "log_level" : "debug",
    "pid_file" : "tests/go-guerrilla.pid",
    "allowed_hosts": ["spam4.me","grr.la"],
    "backend_config" :
        {
            "log_received_mails" : true
        },
    "servers" : [
        {
            "is_enabled" : true,
            "host_name":"mail.guerrillamail.com",
            "max_size": 100017,
            "timeout":160,
            "listen_interface":"127.0.0.1:2526",
            "max_clients": 2,
			"tls" : {
				"start_tls_on":false,
            	"tls_always_on":false,
				"private_key_file":"config_test.go",
            	"public_key_file":"config_test.go"
			}
        },
        {
            "is_enabled" : true,
            "host_name":"mail2.guerrillamail.com",
            "max_size":1000001,
            "timeout":180,
            "listen_interface":"127.0.0.1:2527",
			"max_clients":1,
			"tls" : {
 				"private_key_file":"./tests/mail2.guerrillamail.com.key.pem",
            	"public_key_file":"./tests/mail2.guerrillamail.com.cert.pem",
				"tls_always_on":false,
            	"start_tls_on":true
			}
        },

        {
            "is_enabled" : true,
            "host_name":"mail.stopme.com",
            "max_size": 100017, 
            "timeout":160,
            "listen_interface":"127.0.0.1:9999", 
            "max_clients": 2,
			"tls" : {
				"private_key_file":"config_test.go",
            	"public_key_file":"config_test.go",
				"start_tls_on":false,
            	"tls_always_on":false
			}
        },
        {
            "is_enabled" : true,
            "host_name":"mail.disableme.com",
            "max_size": 100017,
            "timeout":160,
            "listen_interface":"127.0.0.1:3333",
            "max_clients": 2,
			"tls" : { 
				"private_key_file":"config_test.go",
            	"public_key_file":"config_test.go",
				"start_tls_on":false,
				"tls_always_on":false
			}
        }
    ]
}
`

// B is A's configuration with different values from B
// 127.0.0.1:4654 will be added
// A's 127.0.0.1:3333 is disabled
// B's 127.0.0.1:9999 is removed

var configJsonB = `
{
    "log_file" : "./tests/testlog",
    "log_level" : "debug",
    "pid_file" : "tests/different-go-guerrilla.pid",
    "allowed_hosts": ["spam4.me","grr.la","newhost.com"],
    "backend_config" :
        {
            "log_received_mails" : true
        },
    "servers" : [
        {
            "is_enabled" : true,
            "host_name":"mail.guerrillamail.com",
            "max_size": 100017,
            "timeout":161,
            "listen_interface":"127.0.0.1:2526",
            "max_clients": 3,
			"tls" : {
 				"private_key_file":"./tests/mail2.guerrillamail.com.key.pem",
            	"public_key_file": "./tests/mail2.guerrillamail.com.cert.pem",
				"start_tls_on":false,
            	"tls_always_on":true
			}
        },
        {
            "is_enabled" : true,
            "host_name":"mail2.guerrillamail.com",
            "max_size": 100017,
            "timeout":160,
            "listen_interface":"127.0.0.1:2527",
            "log_file" : "./tests/testlog",
            "max_clients": 2,
			"tls" : {
				"private_key_file":"./tests/mail2.guerrillamail.com.key.pem",
            	"public_key_file": "./tests/mail2.guerrillamail.com.cert.pem",
				"start_tls_on":true,
            	"tls_always_on":false
			}
        },

        {
            "is_enabled" : true,
            "host_name":"mail.guerrillamail.com",
            "max_size":1000001,
            "timeout":180,
            "listen_interface":"127.0.0.1:4654",
            "max_clients":1,
			"tls" : {
				"private_key_file":"config_test.go",
				"public_key_file":"config_test.go",
				"start_tls_on":false,
            	"tls_always_on":false
			}
        },

        {
            "is_enabled" : false,
            "host_name":"mail.disbaleme.com",
            "max_size": 100017,
            "timeout":160,
            "listen_interface":"127.0.0.1:3333",
            "max_clients": 2,
			"tls" : {
				"private_key_file":"config_test.go",
            	"public_key_file":"config_test.go",
				"start_tls_on":false,
            	"tls_always_on":false
			}
        }
    ]
}
`

func TestConfigLoad(t *testing.T) {
	if err := testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "./tests/"); err != nil {
		t.Error(err)
	}
	defer func() {
		if err := deleteIfExists("../tests/mail2.guerrillamail.com.cert.pem"); err != nil {
			t.Error(err)
		}
		if err := deleteIfExists("../tests/mail2.guerrillamail.com.key.pem"); err != nil {
			t.Error(err)
		}
	}()

	ac := &AppConfig{}
	if err := ac.Load([]byte(configJsonA)); err != nil {
		t.Error("Cannot load config |", err)
		t.SkipNow()
	}
	expectedLen := 4
	if len(ac.Servers) != expectedLen {
		t.Error("len(ac.Servers), expected", expectedLen, "got", len(ac.Servers))
		t.SkipNow()
	}
	// did we got the timestamps?
	if ac.Servers[0].TLS._privateKeyFileMtime <= 0 {
		t.Error("failed to read timestamp for _privateKeyFileMtime, got", ac.Servers[0].TLS._privateKeyFileMtime)
	}
}

// Test the sample config to make sure a valid one is given!
func TestSampleConfig(t *testing.T) {
	fileName := "goguerrilla.conf.sample"
	if jsonBytes, err := ioutil.ReadFile(fileName); err == nil {
		ac := &AppConfig{}
		if err := ac.Load(jsonBytes); err != nil {
			// sample config can have broken tls certs
			if strings.Index(err.Error(), "cannot use TLS config for [127.0.0.1:25") != 0 {
				t.Error("Cannot load config", fileName, "|", err)
				t.FailNow()
			}
		}
	} else {
		t.Error("Error reading", fileName, "|", err)
	}
}

// make sure that we get all the config change events
func TestConfigChangeEvents(t *testing.T) {
	if err := testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "./tests/"); err != nil {
		t.Error(err)
	}
	defer func() {
		if err := deleteIfExists("../tests/mail2.guerrillamail.com.cert.pem"); err != nil {
			t.Error(err)
		}
		if err := deleteIfExists("../tests/mail2.guerrillamail.com.key.pem"); err != nil {
			t.Error(err)
		}
	}()

	oldconf := &AppConfig{}
	if err := oldconf.Load([]byte(configJsonA)); err != nil {
		t.Error(err)
	}
	logger, _ := log.GetLogger(oldconf.LogFile, oldconf.LogLevel)
	bcfg := backends.BackendConfig{"log_received_mails": true}
	backend, err := backends.New(bcfg, logger)
	if err != nil {
		t.Error("cannot create backend", err)
	}
	app, err := New(oldconf, backend, logger)
	if err != nil {
		t.Error("cannot create daemon", err)
	}
	// simulate timestamp change

	time.Sleep(time.Second + time.Millisecond*500)
	if err := os.Chtimes(oldconf.Servers[1].TLS.PrivateKeyFile, time.Now(), time.Now()); err != nil {
		t.Error(err)
	}
	if err := os.Chtimes(oldconf.Servers[1].TLS.PublicKeyFile, time.Now(), time.Now()); err != nil {
		t.Error(err)
	}
	newconf := &AppConfig{}
	if err := newconf.Load([]byte(configJsonB)); err != nil {
		t.Error(err)
	}
	newconf.Servers[0].LogFile = log.OutputOff.String() // test for log file change
	newconf.LogLevel = log.InfoLevel.String()
	newconf.LogFile = "off"
	expectedEvents := map[Event]bool{
		EventConfigPidFile:         false,
		EventConfigLogFile:         false,
		EventConfigLogLevel:        false,
		EventConfigAllowedHosts:    false,
		EventConfigServerNew:       false, // 127.0.0.1:4654 will be added
		EventConfigServerRemove:    false, // 127.0.0.1:9999 server removed
		EventConfigServerStop:      false, // 127.0.0.1:3333: server (disabled)
		EventConfigServerLogFile:   false, // 127.0.0.1:2526
		EventConfigServerLogReopen: false, // 127.0.0.1:2527
		EventConfigServerTimeout:   false, // 127.0.0.1:2526 timeout
		//"server_change:tls_config":    false, // 127.0.0.1:2526
		EventConfigServerMaxClients: false, // 127.0.0.1:2526
		EventConfigServerTLSConfig:  false, // 127.0.0.1:2527 timestamp changed on certificates
	}
	toUnsubscribe := map[Event]func(c *AppConfig){}
	toUnsubscribeSrv := map[Event]func(c *ServerConfig){}

	for event := range expectedEvents {
		// Put in anon func since range is overwriting event
		func(e Event) {
			if strings.Contains(e.String(), "config_change") {
				f := func(c *AppConfig) {
					expectedEvents[e] = true
				}
				_ = app.Subscribe(event, f)
				toUnsubscribe[event] = f
			} else {
				// must be a server config change then
				f := func(c *ServerConfig) {
					expectedEvents[e] = true
				}
				_ = app.Subscribe(event, f)
				toUnsubscribeSrv[event] = f
			}

		}(event)
	}

	// emit events
	newconf.EmitChangeEvents(oldconf, app)
	// unsubscribe
	for unevent, unfun := range toUnsubscribe {
		_ = app.Unsubscribe(unevent, unfun)
	}
	for unevent, unfun := range toUnsubscribeSrv {
		_ = app.Unsubscribe(unevent, unfun)
	}
	for event, val := range expectedEvents {
		if val == false {
			t.Error("Did not fire config change event:", event)
			t.FailNow()
		}
	}

	// don't forget to reset
	if err := os.Truncate(oldconf.LogFile, 0); err != nil {
		t.Error(err)
	}
}
