package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/flashmob/go-guerrilla"
	test "github.com/flashmob/go-guerrilla/tests"
	"github.com/flashmob/go-guerrilla/tests/testcert"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
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
        },
        {
            "is_enabled" : false,
            "host_name":"enable.test.com",
            "max_size": 1000000,
            "private_key_file":"../..//tests/mail2.guerrillamail.com.key.pem",
            "public_key_file":"../../tests/mail2.guerrillamail.com.cert.pem",
            "timeout":180,
            "listen_interface":"127.0.0.1:2228",
            "start_tls_on":true,
            "tls_always_on":false,
            "max_clients": 1000
        }
    ]
}
`

// backend config changed, log_received_mails is false
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

// backend_name changed, is guerrilla-redis-db + added a server
var configJsonC = `
{
"pid_file" : "./pidfile.pid",
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
            "private_key_file":"../..//tests/mail2.guerrillamail.com.key.pem",
            "public_key_file":"../../tests/mail2.guerrillamail.com.cert.pem",
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
            "private_key_file":"../..//tests/mail2.guerrillamail.com.key.pem",
            "public_key_file":"../../tests/mail2.guerrillamail.com.cert.pem",
            "timeout":180,
            "listen_interface":"127.0.0.1:465",
            "start_tls_on":false,
            "tls_always_on":true,
            "max_clients":500
        }
    ]
}
`

// adds 127.0.0.1:4655, a secure server
var configJsonD = `
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
        "log_received_mails": false
    },
    "servers" : [
        {
            "is_enabled" : true,
            "host_name":"mail.test.com",
            "max_size": 1000000,
            "private_key_file":"../..//tests/mail2.guerrillamail.com.key.pem",
            "public_key_file":"../../tests/mail2.guerrillamail.com.cert.pem",
            "timeout":180,
            "listen_interface":"127.0.0.1:2552",
            "start_tls_on":true,
            "tls_always_on":false,
            "max_clients": 1000
        },
        {
            "is_enabled" : true,
            "host_name":"secure.test.com",
            "max_size":1000000,
            "private_key_file":"../..//tests/mail2.guerrillamail.com.key.pem",
            "public_key_file":"../../tests/mail2.guerrillamail.com.cert.pem",
            "timeout":180,
            "listen_interface":"127.0.0.1:4655",
            "start_tls_on":false,
            "tls_always_on":true,
            "max_clients":500
        }
    ]
}
`

// reload config
func sigHup() {
	if data, err := ioutil.ReadFile("pidfile.pid"); err == nil {
		log.Infof("pid read is %s", data)
		ecmd := exec.Command("kill", "-HUP", string(data))
		_, err = ecmd.Output()
		if err != nil {
			log.Infof("could not SIGHUP", err)
		}
	} else {
		log.WithError(err).Info("sighup - Could not read pidfle")
	}

}

// shutdown after calling serve()
func sigKill() {
	if data, err := ioutil.ReadFile("pidfile.pid"); err == nil {
		log.Infof("pid read is %s", data)
		ecmd := exec.Command("kill", string(data))
		_, err = ecmd.Output()
		if err != nil {
			log.Infof("could not sigkill", err)
		}
	} else {
		log.WithError(err).Info("sigKill - Could not read pidfle")
	}

}

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
		"server_change:new_server":     false,
	}
	toUnsubscribe := map[string]func(c *CmdConfig){}
	toUnsubscribeS := map[string]func(c *guerrilla.ServerConfig){}

	for event := range expectedEvents {
		// Put in anon func since range is overwriting event
		func(e string) {

			if strings.Index(e, "server_change") == 0 {
				f := func(c *guerrilla.ServerConfig) {
					expectedEvents[e] = true
				}
				guerrilla.Bus.Subscribe(event, f)
				toUnsubscribeS[event] = f
			} else {
				f := func(c *CmdConfig) {
					expectedEvents[e] = true
				}
				guerrilla.Bus.Subscribe(event, f)
				toUnsubscribe[event] = f
			}

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

// start server, chnage config, send SIG HUP, confirm that the pidfile changed & backend reloaded
func TestServe(t *testing.T) {
	// hold the output of logs
	var logBuffer bytes.Buffer
	// logs redirected to this writer
	var logOut *bufio.Writer
	// read the logs
	var logIn *bufio.Reader
	testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	logOut = bufio.NewWriter(&logBuffer)
	logIn = bufio.NewReader(&logBuffer)
	log.SetLevel(log.DebugLevel)
	//log.SetOutput(os.Stdout)
	log.SetOutput(logOut)

	ioutil.WriteFile("configJsonA.json", []byte(configJsonA), 0644)
	cmd := &cobra.Command{}
	configPath = "configJsonA.json"
	var serveWG sync.WaitGroup
	serveWG.Add(1)
	go func() {
		serve(cmd, []string{})
		serveWG.Done()
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
	time.Sleep(time.Second) // allow sighup to do its job
	// did the pidfile change as expected?
	if _, err := os.Stat("./pidfile2.pid"); os.IsNotExist(err) {
		t.Error("pidfile not changed after sighup SIGHUP", err)
	}
	// send kill signal and wait for exit
	sigKill()
	serveWG.Wait()

	logOut.Flush()
	// did backend started as expected?
	if read, err := ioutil.ReadAll(logIn); err == nil {
		logOutput := string(read)
		//fmt.Println(logOutput)
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

}

// Start with configJsonA.json,
// then add a new server to it (127.0.0.1:2526),
// then SIGHUP (to reload config & trigger config update events),
// then connect to it & HELO.
func TestServerAddEvent(t *testing.T) {
	// hold the output of logs
	var logBuffer bytes.Buffer
	// logs redirected to this writer
	var logOut *bufio.Writer
	// read the logs
	var logIn *bufio.Reader
	testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	logOut = bufio.NewWriter(&logBuffer)
	logIn = bufio.NewReader(&logBuffer)
	log.SetLevel(log.DebugLevel)
	log.SetOutput(logOut)
	// start the server by emulating the serve command
	ioutil.WriteFile("configJsonA.json", []byte(configJsonA), 0644)
	cmd := &cobra.Command{}
	configPath = "configJsonA.json"
	var serveWG sync.WaitGroup
	serveWG.Add(1)
	go func() {
		serve(cmd, []string{})
		serveWG.Done()
	}()
	time.Sleep(time.Second)
	// now change the config by adding a server
	conf := &CmdConfig{}                                 // blank one
	conf.load([]byte(configJsonA))                       // load configJsonA
	newServer := conf.Servers[0]                         // copy the first server config
	newServer.ListenInterface = "127.0.0.1:2526"         // change it
	newConf := conf                                      // copy the cmdConfg
	newConf.Servers = append(newConf.Servers, newServer) // add the new server
	if jsonbytes, err := json.Marshal(newConf); err == nil {
		//fmt.Println(string(jsonbytes))
		ioutil.WriteFile("configJsonA.json", []byte(jsonbytes), 0644)
	}
	// send a sighup signal to the server
	sigHup()
	time.Sleep(time.Second * 1) // pause for config to reload

	if conn, buffin, err := test.Connect(newServer, 20); err != nil {
		t.Error("Could not connect to new server", newServer.ListenInterface)
	} else {
		if result, err := test.Command(conn, buffin, "HELO"); err == nil {
			expect := "250 mail.test.com Hello"
			if strings.Index(result, expect) != 0 {
				t.Error("Expected", expect, "but got", result)
			}
		} else {
			t.Error(err)
		}
	}

	// send kill signal and wait for exit
	sigKill()
	serveWG.Wait()
	logOut.Flush()
	// did backend started as expected?
	if read, err := ioutil.ReadAll(logIn); err == nil {
		logOutput := string(read)
		//fmt.Println(logOutput)
		if i := strings.Index(logOutput, "New server added [127.0.0.1:2526]"); i < 0 {
			t.Error("Did not add [127.0.0.1:2526], most likely because Bus.Subscribe(\"server_change:new_server\" didnt fire")
		}
	}
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)

	// cleanup
	os.Remove("configJsonA.json")
	os.Remove("./pidfile.pid")

}

// Start with configJsonA.json,
// then change the config to enable 127.0.0.1:2228,
// then write the new config,
// then SIGHUP (to reload config & trigger config update events),
// then connect to 127.0.0.1:2228 & HELO.
func TestServerStartEvent(t *testing.T) {
	// hold the output of logs

	var logBuffer bytes.Buffer
	// logs redirected to this writer
	var logOut *bufio.Writer
	// read the logs
	var logIn *bufio.Reader
	testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	logOut = bufio.NewWriter(&logBuffer)
	logIn = bufio.NewReader(&logBuffer)
	log.SetLevel(log.DebugLevel)
	log.SetOutput(logOut)
	// start the server by emulating the serve command
	ioutil.WriteFile("configJsonA.json", []byte(configJsonA), 0644)
	cmd := &cobra.Command{}
	configPath = "configJsonA.json"
	var serveWG sync.WaitGroup
	serveWG.Add(1)
	go func() {
		serve(cmd, []string{})
		serveWG.Done()
	}()
	time.Sleep(time.Second)
	// now change the config by adding a server
	conf := &CmdConfig{}           // blank one
	conf.load([]byte(configJsonA)) // load configJsonA

	newConf := conf // copy the cmdConfg
	newConf.Servers[1].IsEnabled = true
	if jsonbytes, err := json.Marshal(newConf); err == nil {
		//fmt.Println(string(jsonbytes))
		ioutil.WriteFile("configJsonA.json", []byte(jsonbytes), 0644)
	} else {
		t.Error(err)
	}
	// send a sighup signal to the server
	sigHup()
	time.Sleep(time.Second * 1) // pause for config to reload

	if conn, buffin, err := test.Connect(newConf.Servers[1], 20); err != nil {
		t.Error("Could not connect to new server", newConf.Servers[1].ListenInterface)
	} else {
		if result, err := test.Command(conn, buffin, "HELO"); err == nil {
			expect := "250 enable.test.com Hello"
			if strings.Index(result, expect) != 0 {
				t.Error("Expected", expect, "but got", result)
			}
		} else {
			t.Error(err)
		}
	}
	// send kill signal and wait for exit
	sigKill()
	serveWG.Wait()
	logOut.Flush()
	// did backend started as expected?
	if read, err := ioutil.ReadAll(logIn); err == nil {
		logOutput := string(read)
		//fmt.Println(logOutput)
		if i := strings.Index(logOutput, "Starting server [127.0.0.1:2228]"); i < 0 {
			t.Error("did not add [127.0.0.1:2228], most likely because Bus.Subscribe(\"server_change:start_server\" didnt fire")
		}
	}
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)

	// cleanup
	os.Remove("configJsonA.json")
	os.Remove("./pidfile.pid")

}

// Start with configJsonA.json,
// then change the config to enable 127.0.0.1:2228,
// then write the new config,
// then SIGHUP (to reload config & trigger config update events),
// then connect to 127.0.0.1:2228 & HELO.
// then change the config to dsiable 127.0.0.1:2228,
// then SIGHUP (to reload config & trigger config update events),
// then connect to 127.0.0.1:2228 - it should not connect

func TestServerStopEvent(t *testing.T) {
	// hold the output of logs
	return
	var logBuffer bytes.Buffer
	// logs redirected to this writer
	var logOut *bufio.Writer
	// read the logs
	var logIn *bufio.Reader
	testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	logOut = bufio.NewWriter(&logBuffer)
	logIn = bufio.NewReader(&logBuffer)
	log.SetLevel(log.DebugLevel)
	log.SetOutput(logOut)
	// start the server by emulating the serve command
	ioutil.WriteFile("configJsonA.json", []byte(configJsonA), 0644)
	cmd := &cobra.Command{}
	configPath = "configJsonA.json"
	var serveWG sync.WaitGroup
	serveWG.Add(1)
	go func() {
		serve(cmd, []string{})
		serveWG.Done()
	}()
	time.Sleep(time.Second)
	// now change the config by enabling a server
	conf := &CmdConfig{}           // blank one
	conf.load([]byte(configJsonA)) // load configJsonA

	newConf := conf // copy the cmdConfg
	newConf.Servers[1].IsEnabled = true
	if jsonbytes, err := json.Marshal(newConf); err == nil {
		//fmt.Println(string(jsonbytes))
		ioutil.WriteFile("configJsonA.json", []byte(jsonbytes), 0644)
	} else {
		t.Error(err)
	}
	// send a sighup signal to the server
	sigHup()
	time.Sleep(time.Second * 1) // pause for config to reload

	if conn, buffin, err := test.Connect(newConf.Servers[1], 20); err != nil {
		t.Error("Could not connect to new server", newConf.Servers[1].ListenInterface)
	} else {
		if result, err := test.Command(conn, buffin, "HELO"); err == nil {
			expect := "250 enable.test.com Hello"
			if strings.Index(result, expect) != 0 {
				t.Error("Expected", expect, "but got", result)
			}
		} else {
			t.Error(err)
		}
		conn.Close()
	}
	// now disable the server
	newerConf := newConf // copy the cmdConfg
	newerConf.Servers[1].IsEnabled = false
	if jsonbytes, err := json.Marshal(newerConf); err == nil {
		//fmt.Println(string(jsonbytes))
		ioutil.WriteFile("configJsonA.json", []byte(jsonbytes), 0644)
	} else {
		t.Error(err)
	}
	// send a sighup signal to the server
	sigHup()
	time.Sleep(time.Second * 1) // pause for config to reload

	// it should not connect to the server
	if _, _, err := test.Connect(newConf.Servers[1], 20); err == nil {
		t.Error("127.0.0.1:2228 was disabled, but still accepting connections", newConf.Servers[1].ListenInterface)
	}
	// send kill signal and wait for exit
	sigKill()
	serveWG.Wait()

	logOut.Flush()
	// did backend started as expected?
	if read, err := ioutil.ReadAll(logIn); err == nil {
		logOutput := string(read)
		//fmt.Println(logOutput)
		if i := strings.Index(logOutput, "Server [127.0.0.1:2228] has stopped"); i < 0 {
			t.Error("did not stop [127.0.0.1:2228], most likely because Bus.Subscribe(\"server_change:stop_server\" didnt fire")
		}
	}
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)

	// cleanup
	os.Remove("configJsonA.json")
	os.Remove("./pidfile.pid")

}

// Start with configJsonD.json,
// then connect to 127.0.0.1:4655 & HELO & try RCPT TO with an invalid host [grr.la]
// then change the config to enable add new host [grr.la] to allowed_hosts
// then write the new config,
// then SIGHUP (to reload config & trigger config update events),
// connect to 127.0.0.1:4655 & HELO & try RCPT TO, grr.la should work

func TestAllowedHostsEvent(t *testing.T) {
	// hold the output of logs

	var logBuffer bytes.Buffer
	// logs redirected to this writer
	var logOut *bufio.Writer
	// read the logs
	var logIn *bufio.Reader
	testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	logOut = bufio.NewWriter(&logBuffer)
	logIn = bufio.NewReader(&logBuffer)
	log.SetLevel(log.DebugLevel)
	log.SetOutput(logOut)
	// start the server by emulating the serve command
	ioutil.WriteFile("configJsonD.json", []byte(configJsonD), 0644)
	conf := &CmdConfig{}           // blank one
	conf.load([]byte(configJsonD)) // load configJsonD
	cmd := &cobra.Command{}
	configPath = "configJsonD.json"
	var serveWG sync.WaitGroup
	time.Sleep(time.Second)
	serveWG.Add(1)
	go func() {
		serve(cmd, []string{})
		serveWG.Done()
	}()
	time.Sleep(time.Second)

	// now connect and try RCPT TO with an invalid host
	if conn, buffin, err := test.Connect(conf.AppConfig.Servers[1], 20); err != nil {
		t.Error("Could not connect to new server", conf.AppConfig.Servers[1].ListenInterface, err)
	} else {
		if result, err := test.Command(conn, buffin, "HELO"); err == nil {
			expect := "250 secure.test.com Hello"
			if strings.Index(result, expect) != 0 {
				t.Error("Expected", expect, "but got", result)
			} else {
				if result, err = test.Command(conn, buffin, "RCPT TO:test@grr.la"); err == nil {
					expect := "454 Error: Relay access denied: grr.la"
					if strings.Index(result, expect) != 0 {
						t.Error("Expected:", expect, "but got:", result)
					}
				}
			}
		}
		conn.Close()
	}

	// now change the config by adding a host to allowed hosts

	newConf := conf // copy the cmdConfg
	newConf.AllowedHosts = append(newConf.AllowedHosts, "grr.la")
	if jsonbytes, err := json.Marshal(newConf); err == nil {
		ioutil.WriteFile("configJsonD.json", []byte(jsonbytes), 0644)
	} else {
		t.Error(err)
	}
	// send a sighup signal to the server to reload config
	sigHup()
	time.Sleep(time.Second) // pause for config to reload

	// now repeat the same conversion, RCPT TO should be accepted
	if conn, buffin, err := test.Connect(conf.AppConfig.Servers[1], 20); err != nil {
		t.Error("Could not connect to new server", conf.AppConfig.Servers[1].ListenInterface, err)
	} else {
		if result, err := test.Command(conn, buffin, "HELO"); err == nil {
			expect := "250 secure.test.com Hello"
			if strings.Index(result, expect) != 0 {
				t.Error("Expected", expect, "but got", result)
			} else {
				if result, err = test.Command(conn, buffin, "RCPT TO:test@grr.la"); err == nil {
					expect := "250 OK"
					if strings.Index(result, expect) != 0 {
						t.Error("Expected:", expect, "but got:", result)
					}
				}
			}
		}
		conn.Close()
	}

	// send kill signal and wait for exit
	sigKill()
	serveWG.Wait()
	logOut.Flush()
	// did backend started as expected?
	if read, err := ioutil.ReadAll(logIn); err == nil {
		logOutput := string(read)
		//fmt.Println(logOutput)
		if i := strings.Index(logOutput, "allowed_hosts config changed, a new list was set"); i < 0 {
			t.Error("did not change allowed_hosts, most likely because Bus.Subscribe(\"config_change:allowed_hosts\" didnt fire")
		}
	}
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)

	// cleanup
	os.Remove("configJsonD.json")
	os.Remove("./pidfile.pid")

}

// Test TLS config change event
// start with configJsonD
// should be able to STARTTLS to 127.0.0.1:2525 with no problems
// generate new certs & reload config
// should get a new tls event & able to STARTTLS with no problem

func TestTLSConfigEvent(t *testing.T) {
	// hold the output of logs

	var logBuffer bytes.Buffer
	// logs redirected to this writer
	var logOut *bufio.Writer
	// read the logs
	var logIn *bufio.Reader
	testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	logOut = bufio.NewWriter(&logBuffer)
	logIn = bufio.NewReader(&logBuffer)
	log.SetLevel(log.DebugLevel)
	log.SetOutput(logOut)
	// start the server by emulating the serve command
	ioutil.WriteFile("configJsonD.json", []byte(configJsonD), 0644)
	conf := &CmdConfig{}           // blank one
	conf.load([]byte(configJsonD)) // load configJsonD
	cmd := &cobra.Command{}
	configPath = "configJsonD.json"
	var serveWG sync.WaitGroup
	time.Sleep(time.Second)
	serveWG.Add(1)
	go func() {
		serve(cmd, []string{})
		serveWG.Done()
	}()
	time.Sleep(time.Second)

	// Test STARTTLS handshake
	testTlsHandshake := func() {
		if conn, buffin, err := test.Connect(conf.AppConfig.Servers[0], 20); err != nil {
			t.Error("Could not connect to server", conf.AppConfig.Servers[0].ListenInterface, err)
		} else {
			if result, err := test.Command(conn, buffin, "HELO"); err == nil {
				expect := "250 mail.test.com Hello"
				if strings.Index(result, expect) != 0 {
					t.Error("Expected", expect, "but got", result)
				} else {
					if result, err = test.Command(conn, buffin, "STARTTLS"); err == nil {
						expect := "220 Ready to start TLS"
						if strings.Index(result, expect) != 0 {
							t.Error("Expected:", expect, "but got:", result)
						} else {
							tlsConn := tls.Client(conn, &tls.Config{
								InsecureSkipVerify: true,
								ServerName:         "127.0.0.1",
							})
							if err := tlsConn.Handshake(); err != nil {
								t.Error("Failed to handshake", conf.AppConfig.Servers[0].ListenInterface)
							} else {
								conn = tlsConn
								log.Info("TLS Handshake succeeded")
							}

						}
					}
				}
			}
			conn.Close()
		}
	}
	testTlsHandshake()

	// generate a new cert
	testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	sigHup()

	time.Sleep(time.Second) // pause for config to reload
	testTlsHandshake()

	time.Sleep(time.Second)
	// send kill signal and wait for exit
	sigKill()
	serveWG.Wait()
	logOut.Flush()
	// did backend started as expected?
	if read, err := ioutil.ReadAll(logIn); err == nil {
		logOutput := string(read)
		//fmt.Println(logOutput)
		if i := strings.Index(logOutput, "Server [127.0.0.1:2552] new TLS configuration loaded"); i < 0 {
			t.Error("did not change tls, most likely because Bus.Subscribe(\"server_change:tls_config\" didnt fire")
		}
	}
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)

	// cleanup
	os.Remove("configJsonD.json")
	os.Remove("./pidfile.pid")

}

// Test for missing TLS certificate, when starting or config reload

func TestBadTLS(t *testing.T) {
	// hold the output of logs

	var logBuffer bytes.Buffer
	// logs redirected to this writer
	var logOut *bufio.Writer
	// read the logs
	var logIn *bufio.Reader

	//testcert.GenerateCert("mail2.guerrillamail.com", "", 365 * 24 * time.Hour, false, 2048, "P256", "../../tests/")
	logOut = bufio.NewWriter(&logBuffer)
	logIn = bufio.NewReader(&logBuffer)
	//log.SetLevel(log.DebugLevel) // it will trash std out of debug
	log.SetLevel(log.InfoLevel)
	log.SetOutput(logOut)
	if err := os.Remove("./../../tests/mail2.guerrillamail.com.cert.pem"); err != nil {
		log.WithError(err).Error("could not remove ./../../tests/mail2.guerrillamail.com.cert.pem")
	} else {
		log.Info("removed ./../../tests/mail2.guerrillamail.com.cert.pem")
	}
	// start the server by emulating the serve command
	ioutil.WriteFile("configJsonD.json", []byte(configJsonD), 0644)
	conf := &CmdConfig{}           // blank one
	conf.load([]byte(configJsonD)) // load configJsonD
	cmd := &cobra.Command{}
	configPath = "configJsonD.json"
	var serveWG sync.WaitGroup
	time.Sleep(time.Second)
	serveWG.Add(1)
	go func() {
		serve(cmd, []string{})
		serveWG.Done()
	}()
	time.Sleep(time.Second)

	// Test STARTTLS handshake
	testTlsHandshake := func() {
		if conn, buffin, err := test.Connect(conf.AppConfig.Servers[0], 20); err != nil {
			t.Error("Could not connect to server", conf.AppConfig.Servers[0].ListenInterface, err)
		} else {
			if result, err := test.Command(conn, buffin, "HELO"); err == nil {
				expect := "250 mail.test.com Hello"
				if strings.Index(result, expect) != 0 {
					t.Error("Expected", expect, "but got", result)
				} else {
					if result, err = test.Command(conn, buffin, "STARTTLS"); err == nil {
						expect := "220 Ready to start TLS"
						if strings.Index(result, expect) != 0 {
							t.Error("Expected:", expect, "but got:", result)
						} else {
							tlsConn := tls.Client(conn, &tls.Config{
								InsecureSkipVerify: true,
								ServerName:         "127.0.0.1",
							})
							if err := tlsConn.Handshake(); err != nil {
								log.Info("TLS Handshake failed")
							} else {
								t.Error("Handshake succeeded, expected it to fail", conf.AppConfig.Servers[0].ListenInterface)
								conn = tlsConn

							}

						}
					}
				}
			}
			conn.Close()
		}
	}
	testTlsHandshake()

	// generate a new cert
	//testcert.GenerateCert("mail2.guerrillamail.com", "", 365 * 24 * time.Hour, false, 2048, "P256", "../../tests/")
	sigHup()

	time.Sleep(time.Second) // pause for config to reload
	testTlsHandshake()

	time.Sleep(time.Second)
	// send kill signal and wait for exit
	sigKill()
	serveWG.Wait()
	logOut.Flush()
	// did backend started as expected?
	if read, err := ioutil.ReadAll(logIn); err == nil {
		logOutput := string(read)
		//fmt.Println(logOutput)
		if i := strings.Index(logOutput, "failed to load the new TLS configuration"); i < 0 {
			t.Error("did not detect TLS load failure")
		}
	}
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)

	// cleanup
	os.Remove("configJsonD.json")
	os.Remove("./pidfile.pid")

}

// Test for when the server config Timeout value changes
// Start with configJsonD.json

func TestSetTimeoutEvent(t *testing.T) {
	// hold the output of logs

	var logBuffer bytes.Buffer
	// logs redirected to this writer
	var logOut *bufio.Writer
	// read the logs
	var logIn *bufio.Reader

	testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	logOut = bufio.NewWriter(&logBuffer)
	logIn = bufio.NewReader(&logBuffer)
	log.SetLevel(log.DebugLevel)
	log.SetOutput(logOut)

	// start the server by emulating the serve command
	ioutil.WriteFile("configJsonD.json", []byte(configJsonD), 0644)
	conf := &CmdConfig{}           // blank one
	conf.load([]byte(configJsonD)) // load configJsonD
	cmd := &cobra.Command{}
	configPath = "configJsonD.json"
	var serveWG sync.WaitGroup
	time.Sleep(time.Second)
	serveWG.Add(1)
	go func() {
		serve(cmd, []string{})
		serveWG.Done()
	}()
	time.Sleep(time.Second)

	if conn, buffin, err := test.Connect(conf.AppConfig.Servers[0], 20); err != nil {
		t.Error("Could not connect to server", conf.AppConfig.Servers[0].ListenInterface, err)
	} else {
		if result, err := test.Command(conn, buffin, "HELO"); err == nil {
			expect := "250 mail.test.com Hello"
			if strings.Index(result, expect) != 0 {
				t.Error("Expected", expect, "but got", result)
			}
		}
	}
	// set the timeout to 1 second

	newConf := conf // copy the cmdConfg
	newConf.Servers[0].Timeout = 1
	if jsonbytes, err := json.Marshal(newConf); err == nil {
		ioutil.WriteFile("configJsonD.json", []byte(jsonbytes), 0644)
	} else {
		t.Error(err)
	}
	// send a sighup signal to the server to reload config
	sigHup()
	time.Sleep(time.Millisecond * 1200) // pause for connection to timeout

	// so the connection we have opened should timeout by now

	// send kill signal and wait for exit
	sigKill()
	serveWG.Wait()
	logOut.Flush()
	// did backend started as expected?
	if read, err := ioutil.ReadAll(logIn); err == nil {
		logOutput := string(read)
		fmt.Println(logOutput)
		if i := strings.Index(logOutput, "i/o timeout"); i < 0 {
			t.Error("Connection to 127.0.0.1:2552 didn't timeout as expected")
		}
	}
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)

	// cleanup
	os.Remove("configJsonD.json")
	os.Remove("./pidfile.pid")

}
