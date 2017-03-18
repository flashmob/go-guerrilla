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

func init() {
	testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "./tests/")
}

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
            "private_key_file":"config_test.go",
            "public_key_file":"config_test.go",
            "timeout":160,
            "listen_interface":"127.0.0.1:2526",
            "start_tls_on":false,
            "tls_always_on":false,
            "max_clients": 2
        },

        {
            "is_enabled" : true,
            "host_name":"mail2.guerrillamail.com",
            "max_size":1000001,
            "private_key_file":"./tests/mail2.guerrillamail.com.key.pem",
            "public_key_file":"./tests/mail2.guerrillamail.com.cert.pem",
            "timeout":180,
            "listen_interface":"127.0.0.1:2527",
            "start_tls_on":true,
            "tls_always_on":false,
            "max_clients":1
        },

        {
            "is_enabled" : true,
            "host_name":"mail.stopme.com",
            "max_size": 100017,
            "private_key_file":"config_test.go",
            "public_key_file":"config_test.go",
            "timeout":160,
            "listen_interface":"127.0.0.1:9999",
            "start_tls_on":false,
            "tls_always_on":false,
            "max_clients": 2
        },

        {
            "is_enabled" : true,
            "host_name":"mail.disableme.com",
            "max_size": 100017,
            "private_key_file":"config_test.go",
            "public_key_file":"config_test.go",
            "timeout":160,
            "listen_interface":"127.0.0.1:3333",
            "start_tls_on":false,
            "tls_always_on":false,
            "max_clients": 2
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
            "private_key_file":"config_test.go",
            "public_key_file":"config_test.go",
            "timeout":161,
            "listen_interface":"127.0.0.1:2526",
            "start_tls_on":false,
            "tls_always_on":true,
            "max_clients": 3
        },
        {
            "is_enabled" : true,
            "host_name":"mail2.guerrillamail.com",
            "max_size": 100017,
            "private_key_file":"./tests/mail2.guerrillamail.com.key.pem",
            "public_key_file": "./tests/mail2.guerrillamail.com.cert.pem",
            "timeout":160,
            "listen_interface":"127.0.0.1:2527",
            "start_tls_on":true,
            "tls_always_on":false,
            "log_file" : "./tests/testlog",
            "max_clients": 2
        },

        {
            "is_enabled" : true,
            "host_name":"mail.guerrillamail.com",
            "max_size":1000001,
            "private_key_file":"config_test.go",
            "public_key_file":"config_test.go",
            "timeout":180,
            "listen_interface":"127.0.0.1:4654",
            "start_tls_on":false,
            "tls_always_on":false,
            "max_clients":1
        },

        {
            "is_enabled" : false,
            "host_name":"mail.disbaleme.com",
            "max_size": 100017,
            "private_key_file":"config_test.go",
            "public_key_file":"config_test.go",
            "timeout":160,
            "listen_interface":"127.0.0.1:3333",
            "start_tls_on":true,
            "tls_always_on":false,
            "max_clients": 2
        }
    ]
}
`

func TestConfigLoad(t *testing.T) {
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
	if ac.Servers[0]._privateKeyFile_mtime <= 0 {
		t.Error("failed to read timestamp for _privateKeyFile_mtime, got", ac.Servers[0]._privateKeyFile_mtime)
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

	oldconf := &AppConfig{}
	oldconf.Load([]byte(configJsonA))
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
	os.Chtimes(oldconf.Servers[1].PrivateKeyFile, time.Now(), time.Now())
	os.Chtimes(oldconf.Servers[1].PublicKeyFile, time.Now(), time.Now())
	newconf := &AppConfig{}
	newconf.Load([]byte(configJsonB))
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
			if strings.Index(e.String(), "config_change") != -1 {
				f := func(c *AppConfig) {
					expectedEvents[e] = true
				}
				app.Subscribe(event, f)
				toUnsubscribe[event] = f
			} else {
				// must be a server config change then
				f := func(c *ServerConfig) {
					expectedEvents[e] = true
				}
				app.Subscribe(event, f)
				toUnsubscribeSrv[event] = f
			}

		}(event)
	}

	// emit events
	newconf.EmitChangeEvents(oldconf, app)
	// unsubscribe
	for unevent, unfun := range toUnsubscribe {
		app.Unsubscribe(unevent, unfun)
	}
	for unevent, unfun := range toUnsubscribeSrv {
		app.Unsubscribe(unevent, unfun)
	}
	for event, val := range expectedEvents {
		if val == false {
			t.Error("Did not fire config change event:", event)
			t.FailNow()
			break
		}
	}

	// don't forget to reset
	os.Truncate(oldconf.LogFile, 0)
}
