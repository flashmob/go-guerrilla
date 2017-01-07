package main

import (
	"bufio"
	"bytes"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/flashmob/go-guerrilla"
	test "github.com/flashmob/go-guerrilla/tests"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

var configJsonA = `
{
    "pid_file" : "./pidfile.pid",
    "allowed_hosts": [
      "guerrillamail.com",
      "guerrillamailblock.com",
      "sharklasers.com",
      "guerrillamail.net",
      "guerrillamail.org"
    ],
    "backend_name": "dummy",
    "backend_config": {
        "log_received_mails": true
    },
    "servers" : [
        {
            "is_enabled" : true,
            "host_name":"mail.test.com",
            "max_size": 1000000,
            "private_key_file":"../..//tests/mail2.guerrillamail.com.key.pem",
            "public_key_file":"../../tests/mail2.guerrillamail.com.cert.pem",
            "timeout":180,
            "listen_interface":"127.0.0.1:25",
            "start_tls_on":true,
            "tls_always_on":false,
            "max_clients": 1000
        }
    ]
}
`

var configJsonB = `
{
"pid_file" : "./pidfile2.pid",
    "allowed_hosts": [
      "guerrillamail.com",
      "guerrillamailblock.com",
      "sharklasers.com",
      "guerrillamail.net",
      "guerrillamail.org"
    ],
    "backend_name": "dummy",
    "backend_config": {
        "log_received_mails": false
    },
    "servers" : [
        {
            "is_enabled" : true,
            "host_name":"mail.test.com",
            "max_size": 1000000,
            "private_key_file":"/path/to/pem/file/test.com.key",
            "public_key_file":"/path/to/pem/file/test.com.crt",
            "timeout":180,
            "listen_interface":"127.0.0.1:25",
            "start_tls_on":true,
            "tls_always_on":false,
            "max_clients": 1000
        }
    ]
}
`
var configJsonC = `
{
"pid_file" : "pidfile.pid",
    "allowed_hosts": [
      "guerrillamail.com",
      "guerrillamailblock.com",
      "sharklasers.com",
      "guerrillamail.net",
      "guerrillamail.org"
    ],
    "backend_name": "guerrilla-redis-db",
    "backend_config" :
        {
            "mysql_db":"gmail_mail",
            "mysql_host":"127.0.0.1:3306",
            "mysql_pass":"ok",
            "mysql_user":"root",
            "mail_table":"new_mail",
            "redis_interface" : "127.0.0.1:6379",
            "redis_expire_seconds" : 7200,
            "save_workers_size" : 3,
            "primary_mail_host":"sharklasers.com"
        },
    "servers" : [
        {
            "is_enabled" : true,
            "host_name":"mail.test.com",
            "max_size": 1000000,
            "private_key_file":"/path/to/pem/file/test.com.key",
            "public_key_file":"/path/to/pem/file/test.com.crt",
            "timeout":180,
            "listen_interface":"127.0.0.1:25",
            "start_tls_on":true,
            "tls_always_on":false,
            "max_clients": 1000
        },
        {
            "is_enabled" : true,
            "host_name":"mail.test.com",
            "max_size":1000000,
            "private_key_file":"/path/to/pem/file/test.com.key",
            "public_key_file":"/path/to/pem/file/test.com.crt",
            "timeout":180,
            "listen_interface":"127.0.0.1:465",
            "start_tls_on":false,
            "tls_always_on":true,
            "max_clients":500
        }
    ]
}
`

// make sure that we get all the config change events
func TestCmdConfigChangeEvents(t *testing.T) {
	oldconf := &CmdConfig{}
	oldconf.load([]byte(configJsonA))

	newconf := &CmdConfig{}
	newconf.load([]byte(configJsonB))

	newerconf := &CmdConfig{}
	newerconf.load([]byte(configJsonC))

	expectedEvents := map[string]bool{
		"config_change:backend_config": false,
		"config_change:backend_name":   false,
	}
	toUnsubscribe := map[string]func(c *CmdConfig){}

	for event := range expectedEvents {
		// Put in anon func since range is overwriting event
		func(e string) {
			f := func(c *CmdConfig) {
				expectedEvents[e] = true
			}
			guerrilla.Bus.Subscribe(event, f)
			toUnsubscribe[event] = f

		}(event)
	}

	// emit events
	newconf.emitChangeEvents(oldconf)
	newerconf.emitChangeEvents(newconf)
	// unsubscribe
	for unevent, unfun := range toUnsubscribe {
		guerrilla.Bus.Unsubscribe(unevent, unfun)
	}

	for event, val := range expectedEvents {
		if val == false {
			t.Error("Did not fire config change event:", event)
			t.FailNow()
			break
		}
	}
}

func TestServe(t *testing.T) {
	// hold the output of logs
	var logBuffer bytes.Buffer
	// logs redirected to this writer
	var logOut *bufio.Writer
	// read the logs
	var logIn *bufio.Reader
	test.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	logOut = bufio.NewWriter(&logBuffer)
	logIn = bufio.NewReader(&logBuffer)
	log.SetLevel(log.DebugLevel)
	log.SetOutput(logOut)

	ioutil.WriteFile("configJsonA.json", []byte(configJsonA), 0644)
	cmd := &cobra.Command{}
	configPath = "configJsonA.json"
	go func() {
		serve(cmd, []string{})
	}()
	time.Sleep(time.Second)

	data, err := ioutil.ReadFile("pidfile.pid")
	if err != nil {
		t.Error("error reading pidfile.pid", err)
		t.FailNow()
	}
	_, err = strconv.Atoi(string(data))
	if err != nil {
		t.Error("could not parse pidfile.pid", err)
		t.FailNow()
	}

	// change the config file
	ioutil.WriteFile("configJsonA.json", []byte(configJsonB), 0644)

	// test SIGHUP via the kill command
	ecmd := exec.Command("kill", "-HUP", string(data))
	_, err = ecmd.Output()
	if err != nil {
		t.Error("could not SIGHUP", err)
		t.FailNow()
	}
	time.Sleep(time.Second) // allow sihgup to do its job

	if _, err := os.Stat("./pidfile2.pid"); os.IsNotExist(err) {
		t.Error("pidfile not changed after sighup SIGHUP", err)
	}

	logOut.Flush()
	if read, err := ioutil.ReadAll(logIn); err == nil {
		logOutput := string(read)
		fmt.Println(logOutput)
		if i := strings.Index(logOutput, "Backend started:dummy"); i < 0 {
			t.Error("Dummy backend not restared")
		}
	}
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)

	// cleanup
	os.Remove("configJsonA.json")
	os.Remove("./pidfile.pid")
	os.Remove("./pidfile2.pid")

	// test the SIGTERM kill command
	/*
		ecmd = exec.Command("kill",string(data))
		_, err = ecmd.Output()
		if err != nil {
			t.Error("could not kill", err)
			t.FailNow()
		}
	*/

}
