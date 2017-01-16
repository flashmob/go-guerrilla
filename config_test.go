package guerrilla

import (
	"bufio"
	"bytes"
	log "github.com/Sirupsen/logrus"
	"github.com/flashmob/go-guerrilla/backends"
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
    "pid_file" : "/var/run/go-guerrilla.pid",
    "allowed_hosts": ["spam4.me","grr.la"],
    "backend_name" : "dummy",
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
            "start_tls_on":true,
            "tls_always_on":false,
            "max_clients": 2,
            "log_file":"/dev/stdout"
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
            "max_clients":1,
            "log_file":"/dev/stdout"
        },

        {
            "is_enabled" : true,
            "host_name":"mail.stopme.com",
            "max_size": 100017,
            "private_key_file":"config_test.go",
            "public_key_file":"config_test.go",
            "timeout":160,
            "listen_interface":"127.0.0.1:9999",
            "start_tls_on":true,
            "tls_always_on":false,
            "max_clients": 2,
            "log_file":"/dev/stdout"
        },

        {
            "is_enabled" : true,
            "host_name":"mail.disableme.com",
            "max_size": 100017,
            "private_key_file":"config_test.go",
            "public_key_file":"config_test.go",
            "timeout":160,
            "listen_interface":"127.0.0.1:3333",
            "start_tls_on":true,
            "tls_always_on":false,
            "max_clients": 2,
            "log_file":"/dev/stdout"
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
    "pid_file" : "/var/run/different-go-guerrilla.pid",
    "allowed_hosts": ["spam4.me","grr.la","newhost.com"],
    "backend_name" : "dummy",
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
            "max_clients": 3,
            "log_file":"/var/log/smtpd.log"
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
            "max_clients": 2,
            "log_file":"/dev/stdout"
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
            "tls_always_on":true,
            "max_clients":1,
            "log_file":"/dev/stdout"
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
            "max_clients": 2,
            "log_file":"/dev/stdout"
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
			if strings.Index(err.Error(), "could not stat key") != 0 {
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

	// hold the output of logs
	var logBuffer bytes.Buffer
	// logs redirected to this writer
	var logOut *bufio.Writer
	// read the logs
	var logIn *bufio.Reader
	logOut = bufio.NewWriter(&logBuffer)
	logIn = bufio.NewReader(&logBuffer)
	log.SetLevel(log.DebugLevel)
	//log.SetOutput(os.Stdout)
	log.SetOutput(logOut)

	oldconf := &AppConfig{}
	oldconf.Load([]byte(configJsonA))
	bcfg := backends.BackendConfig{"log_received_mails": true}
	backend, _ := backends.New("dummy", bcfg)
	app, _ := New(oldconf, backend)
	// simulate timestamp change
	time.Sleep(time.Second + time.Millisecond*500)
	os.Chtimes(oldconf.Servers[1].PrivateKeyFile, time.Now(), time.Now())
	os.Chtimes(oldconf.Servers[1].PublicKeyFile, time.Now(), time.Now())
	newconf := &AppConfig{}
	newconf.Load([]byte(configJsonB))
	expectedEvents := map[string]bool{
		"config_change:pid_file":                       false,
		"config_change:allowed_hosts":                  false,
		"server_change:new_server":                     false, // 127.0.0.1:4654 will be added
		"server_change:remove_server":                  false, // 127.0.0.1:9999 server removed
		"server_change:stop_server":                    false, // 127.0.0.1:3333: server (disabled)
		"server_change:127.0.0.1:2526:new_log_file":    false,
		"server_change:127.0.0.1:2527:reopen_log_file": false,
		"server_change:timeout":                        false, // 127.0.0.1:2526 timeout
		//"server_change:tls_config":      false, // 127.0.0.1:2526
		"server_change:max_clients": false, // 127.0.0.1:2526
		"server_change:tls_config":  false, // 127.0.0.1:2527 timestamp changed on certificates
	}
	toUnsubscribe := map[string]func(c *AppConfig){}
	toUnsubscribeS := map[string]func(c *ServerConfig){}

	for event := range expectedEvents {
		// Put in anon func since range is overwriting event
		func(e string) {
			if strings.Index(e, "config_change") != -1 {
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
				toUnsubscribeS[event] = f
			}

		}(event)
	}

	// emit events
	newconf.EmitChangeEvents(oldconf, app)
	// unsubscribe
	for unevent, unfun := range toUnsubscribe {
		app.Unsubscribe(unevent, unfun)
	}
	for unevent, unfun := range toUnsubscribeS {
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
	logBuffer.Reset()
	logIn.Reset(&logBuffer)
}
