package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flashmob/go-guerrilla"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	test "github.com/flashmob/go-guerrilla/tests"
	"github.com/flashmob/go-guerrilla/tests/testcert"
	"github.com/spf13/cobra"
)

var configJsonA = `
{
    "log_file" : "../../tests/testlog",
    "log_level" : "debug",
    "pid_file" : "./pidfile.pid",
    "allowed_hosts": [
      "guerrillamail.com",
      "guerrillamailblock.com",
      "sharklasers.com",
      "guerrillamail.net",
      "guerrillamail.org"
    ],
    "backend_config": {
    	"save_workers_size" : 1,
    	"save_process": "HeadersParser|Debugger",
        "log_received_mails": true
    },
    "servers" : [
        {
            "is_enabled" : true,
            "host_name":"mail.test.com",
            "max_size": 1000000,
            "timeout":180,
            "listen_interface":"127.0.0.1:3536",
            "max_clients": 200,
            "log_file" : "../../tests/testlog",
			"tls" : {
				"private_key_file":"../../tests/mail2.guerrillamail.com.key.pem",
            	"public_key_file":"../../tests/mail2.guerrillamail.com.cert.pem",
				"start_tls_on":true,
            	"tls_always_on":false
			}
        },
        {
            "is_enabled" : false,
            "host_name":"enable.test.com",
            "max_size": 1000000,
            "timeout":180,
            "listen_interface":"127.0.0.1:2228",
            "max_clients": 200,
            "log_file" : "../../tests/testlog",
			"tls" : {
				"private_key_file":"../../tests/mail2.guerrillamail.com.key.pem",
				"public_key_file":"../../tests/mail2.guerrillamail.com.cert.pem",
				"start_tls_on":true,
            	"tls_always_on":false
			}
        }
    ]
}
`

// backend config changed, log_received_mails is false
var configJsonB = `
{
    "log_file" : "../../tests/testlog",
    "log_level" : "debug",
    "pid_file" : "./pidfile2.pid",
    "allowed_hosts": [
      "guerrillamail.com",
      "guerrillamailblock.com",
      "sharklasers.com",
      "guerrillamail.net",
      "guerrillamail.org"
    ],
    "backend_config": {
    	"save_workers_size" : 1,
    	"save_process": "HeadersParser|Debugger",
        "log_received_mails": false
    },
    "servers" : [
        {
            "is_enabled" : true,
            "host_name":"mail.test.com",
            "max_size": 1000000,
            "timeout":180,
            "listen_interface":"127.0.0.1:3536",
            "max_clients": 200,
            "log_file" : "../../tests/testlog",
			"tls" : {
				"private_key_file":"../../tests/mail2.guerrillamail.com.key.pem",
            	"public_key_file":"../../tests/mail2.guerrillamail.com.cert.pem",
            	"start_tls_on":true,
            	"tls_always_on":false
			}
        }
    ]
}
`

// added a server
var configJsonC = `
{
    "log_file" : "../../tests/testlog",
    "log_level" : "debug",
    "pid_file" : "./pidfile.pid",
    "allowed_hosts": [
      "guerrillamail.com",
      "guerrillamailblock.com",
      "sharklasers.com",
      "guerrillamail.net",
      "guerrillamail.org"
    ],
    "backend_config" :
        {
            "sql_driver": "mysql",
            "sql_dsn": "root:ok@tcp(127.0.0.1:3306)/gmail_mail?readTimeout=10s&writeTimeout=10s",
            "mail_table":"new_mail",
            "redis_interface" : "127.0.0.1:6379",
            "redis_expire_seconds" : 7200,
            "save_workers_size" : 3,
            "primary_mail_host":"sharklasers.com",
            "save_workers_size" : 1,
	    	"save_process": "HeadersParser|Debugger",
	    	"log_received_mails": true
        },
    "servers" : [
        {
            "is_enabled" : true,
            "host_name":"mail.test.com",
            "max_size": 1000000,
            "timeout":180,
            "listen_interface":"127.0.0.1:25",
            "max_clients": 200,
            "log_file" : "../../tests/testlog",
			"tls" : {
				"private_key_file":"../../tests/mail2.guerrillamail.com.key.pem",
            	"public_key_file":"../../tests/mail2.guerrillamail.com.cert.pem",
				"start_tls_on":true,
            	"tls_always_on":false
			}
        },
        {
            "is_enabled" : true,
            "host_name":"mail.test.com",
            "max_size":1000000,
            "timeout":180,
            "listen_interface":"127.0.0.1:465",
            "max_clients":200,
            "log_file" : "../../tests/testlog",
			"tls" : {
				"private_key_file":"../../tests/mail2.guerrillamail.com.key.pem",
            	"public_key_file":"../../tests/mail2.guerrillamail.com.cert.pem",
				"start_tls_on":false,
            	"tls_always_on":true
			}
        }
    ]
}
`

// adds 127.0.0.1:4655, a secure server
var configJsonD = `
{
    "log_file" : "../../tests/testlog",
    "log_level" : "debug",
    "pid_file" : "./pidfile.pid",
    "allowed_hosts": [
      "guerrillamail.com",
      "guerrillamailblock.com",
      "sharklasers.com",
      "guerrillamail.net",
      "guerrillamail.org"
    ],
    "backend_config": {
        "save_workers_size" : 1,
    	"save_process": "HeadersParser|Debugger",
        "log_received_mails": false
    },
    "servers" : [
        {
            "is_enabled" : true,
            "host_name":"mail.test.com",
            "max_size": 1000000,
            "timeout":180,
            "listen_interface":"127.0.0.1:2552",
            "max_clients": 200,
            "log_file" : "../../tests/testlog",
			"tls" : {
				"private_key_file":"../../tests/mail2.guerrillamail.com.key.pem",
            	"public_key_file":"../../tests/mail2.guerrillamail.com.cert.pem",
				"start_tls_on":true,
            	"tls_always_on":false
			}
        },
        {
            "is_enabled" : true,
            "host_name":"secure.test.com",
            "max_size":1000000,
            "timeout":180,
            "listen_interface":"127.0.0.1:4655",
            "max_clients":200,
            "log_file" : "../../tests/testlog",
			"tls" : {
				"private_key_file":"../../tests/mail2.guerrillamail.com.key.pem",
				"public_key_file":"../../tests/mail2.guerrillamail.com.cert.pem",
				"start_tls_on":false,
            	"tls_always_on":true
			}
        }
    ]
}
`

// adds 127.0.0.1:4655, a secure server
var configJsonE = `
{
    "log_file" : "../../tests/testlog",
    "log_level" : "debug",
    "pid_file" : "./pidfile2.pid",
    "allowed_hosts": [
      "guerrillamail.com",
      "guerrillamailblock.com",
      "sharklasers.com",
      "guerrillamail.net",
      "guerrillamail.org"
    ],
    "backend_config" :
        {
            "save_process_old": "HeadersParser|Debugger|Hasher|Header|Compressor|Redis|MySql",
            "save_process": "GuerrillaRedisDB",
            "log_received_mails" : true,
            "sql_driver": "mysql",
            "sql_dsn": "root:secret@tcp(127.0.0.1:3306)/gmail_mail?readTimeout=10s&writeTimeout=10s",
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
            "timeout":180,
            "listen_interface":"127.0.0.1:2552",
            "max_clients": 200,
            "log_file" : "../../tests/testlog",
			"tls" : {
				"private_key_file":"../../tests/mail2.guerrillamail.com.key.pem",
            	"public_key_file":"../../tests/mail2.guerrillamail.com.cert.pem",
				"start_tls_on":true,
            	"tls_always_on":false
			}
        },
        {
            "is_enabled" : true,
            "host_name":"secure.test.com",
            "max_size":1000000,
            "timeout":180,
            "listen_interface":"127.0.0.1:4655",
            "max_clients":200,
            "log_file" : "../../tests/testlog",
			"tls" : {
				"private_key_file":"../../tests/mail2.guerrillamail.com.key.pem",
            	"public_key_file":"../../tests/mail2.guerrillamail.com.cert.pem",
				"start_tls_on":false,
            	"tls_always_on":true
			}
        }
    ]
}
`

const testPauseDuration = time.Millisecond * 1010

// reload config
func sigHup() {
	if data, err := ioutil.ReadFile("pidfile.pid"); err == nil {
		mainlog.Infof("pid read is %s", data)
		ecmd := exec.Command("kill", "-HUP", string(data))
		_, err = ecmd.Output()
		if err != nil {
			mainlog.Infof("could not SIGHUP", err)
		}
	} else {
		mainlog.WithError(err).Info("sighup - Could not read pidfle")
	}

}

// shutdown after calling serve()
func sigKill() {
	if data, err := ioutil.ReadFile("pidfile.pid"); err == nil {
		mainlog.Infof("pid read is %s", data)
		ecmd := exec.Command("kill", string(data))
		_, err = ecmd.Output()
		if err != nil {
			mainlog.Infof("could not sigkill", err)
		}
	} else {
		mainlog.WithError(err).Info("sigKill - Could not read pidfle")
	}
}

func Round(x float64) float64 {
	t := math.Trunc(x)
	if math.Abs(x-t) >= 0.5 {
		return t + math.Copysign(1, x)
	}
	return t
}

// exponentialBackoff sleeps in nanoseconds, according to this formula 2^(i-1) * 25 / 2
func exponentialBackoff(i int) {
	time.Sleep(time.Duration(Round(math.Pow(3.0, float64(i))-1.0)*100.0/2.0) * time.Millisecond)
}

var grepNotFound error

// grepTestlog looks for the `match` string in the testlog
// the lineNumber indicates what line to start the search from
// returns line number it was found on
// error otherwise
//
// It will attempt to search the log multiple times, pausing loner for each re-try
//
func grepTestlog(match string, lineNumber int) (found int, err error) {
	found = 0
	fd, err := os.Open("../../tests/testlog")
	if err != nil {
		return found, err
	}
	defer func() {
		_ = fd.Close()
	}()
	buff := bufio.NewReader(fd)
	var ln int
	var line string
	for tries := 0; tries < 6; tries++ {
		//fmt.Println("try..", tries)
		for {
			ln++
			line, err = buff.ReadString('\n')
			if err != nil {
				break
			}
			if ln > lineNumber {
				//fmt.Print(ln, line)
				if i := strings.Index(line, match); i != -1 {
					return ln, nil
				}
			}
		}
		if err != io.EOF {
			return found, err
		}

		err = fd.Close()
		if err != nil {
			return 0, err
		}
		fd = nil

		// sleep
		exponentialBackoff(tries)
		_ = mainlog.Reopen()

		// re-open
		fd, err = os.OpenFile("../../tests/testlog", os.O_RDONLY, 0644)
		if err != nil {
			return found, err
		}
		buff.Reset(fd)

		ln = 0
	}

	grepNotFound = errors.New("could not find " + match + " in tests/testlog after line" + strconv.Itoa(lineNumber))
	return found, grepNotFound
}

// In all the tests, there will be a minimum of about 2000 available
func TestFileLimit(t *testing.T) {
	cfg := &guerrilla.AppConfig{LogFile: log.OutputOff.String()}
	sc := guerrilla.ServerConfig{
		ListenInterface: "127.0.0.1:2526",
		IsEnabled:       true,
		MaxClients:      1000,
	}
	cfg.Servers = append(cfg.Servers, sc)
	d := guerrilla.Daemon{Config: cfg}
	if ok, maxClients, fileLimit := guerrilla.CheckFileLimit(d.Config); !ok {
		t.Errorf("Combined max clients for all servers (%d) is greater than open file limit (%d). "+
			"Please increase your open file limit. Please check your OS docs for how to increase the limit.", maxClients, fileLimit)
	}
}

func getTestLog() (mainlog log.Logger, err error) {
	return log.GetLogger("../../tests/testlog", "debug")
}

func truncateIfExists(filename string) error {
	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		return os.Truncate(filename, 0)
	}
	return nil
}
func deleteIfExists(filename string) error {
	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		return os.Remove(filename)
	}
	return nil
}

func cleanTestArtifacts(t *testing.T) {

	if err := truncateIfExists("../../tests/testlog"); err != nil {
		t.Error("could not clean tests/testlog:", err)
	}
	if err := truncateIfExists("../../tests/testlog2"); err != nil {
		t.Error("could not clean tests/testlog2:", err)
	}

	letters := []byte{'A', 'B', 'C', 'D', 'E'}
	for _, l := range letters {
		if err := deleteIfExists("configJson" + string(l) + ".json"); err != nil {
			t.Error("could not delete configJson"+string(l)+".json:", err)
		}
	}

	if err := deleteIfExists("./pidfile.pid"); err != nil {
		t.Error("could not delete ./pidfile.pid", err)
	}
	if err := deleteIfExists("./pidfile2.pid"); err != nil {
		t.Error("could not delete ./pidfile2.pid", err)
	}

	if err := deleteIfExists("../../tests/mail.guerrillamail.com.cert.pem"); err != nil {
		t.Error("could not delete ../../tests/mail.guerrillamail.com.cert.pem", err)
	}
	if err := deleteIfExists("../../tests/mail.guerrillamail.com.key.pem"); err != nil {
		t.Error("could not delete ../../tests/mail.guerrillamail.com.key.pem", err)
	}

	if err := deleteIfExists("../../tests/mail2.guerrillamail.com.cert.pem"); err != nil {
		t.Error("could not delete ../../tests/mail2.guerrillamail.com.cert.pem", err)
	}
	if err := deleteIfExists("../../tests/mail2.guerrillamail.com.key.pem"); err != nil {
		t.Error("could not delete ../../tests/mail2.guerrillamail.com.key.pem", err)
	}

}

// make sure that we get all the config change events
func TestCmdConfigChangeEvents(t *testing.T) {
	defer cleanTestArtifacts(t)
	var err error
	err = testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	if err != nil {
		t.Error("failed to generate a test certificate", err)
		t.FailNow()
	}

	oldconf := &guerrilla.AppConfig{}
	if err := oldconf.Load([]byte(configJsonA)); err != nil {
		t.Error("configJsonA is invalid", err)
	}

	newconf := &guerrilla.AppConfig{}
	if err := newconf.Load([]byte(configJsonB)); err != nil {
		t.Error("configJsonB is invalid", err)
	}

	newerconf := &guerrilla.AppConfig{}
	if err := newerconf.Load([]byte(configJsonC)); err != nil {
		t.Error("configJsonC is invalid", err)
	}

	expectedEvents := map[guerrilla.Event]bool{
		guerrilla.EventConfigBackendConfig: false,
		guerrilla.EventConfigServerNew:     false,
	}
	mainlog, err = getTestLog()
	if err != nil {
		t.Error("could not get logger,", err)
		t.FailNow()
	}

	bcfg := backends.BackendConfig{"log_received_mails": true}
	backend, err := backends.New(bcfg, mainlog)
	app, err := guerrilla.New(oldconf, backend, mainlog)
	if err != nil {
		t.Error("Failed to create new app", err)
	}
	toUnsubscribe := map[guerrilla.Event]func(c *guerrilla.AppConfig){}
	toUnsubscribeS := map[guerrilla.Event]func(c *guerrilla.ServerConfig){}

	for event := range expectedEvents {
		// Put in anon func since range is overwriting event
		func(e guerrilla.Event) {
			if strings.Index(e.String(), "server_change") == 0 {
				f := func(c *guerrilla.ServerConfig) {
					expectedEvents[e] = true
				}
				_ = app.Subscribe(e, f)
				toUnsubscribeS[e] = f
			} else {
				f := func(c *guerrilla.AppConfig) {
					expectedEvents[e] = true
				}
				_ = app.Subscribe(e, f)
				toUnsubscribe[e] = f
			}

		}(event)
	}

	// emit events
	newconf.EmitChangeEvents(oldconf, app)
	newerconf.EmitChangeEvents(newconf, app)
	// unsubscribe
	for unevent, unfun := range toUnsubscribe {
		_ = app.Unsubscribe(unevent, unfun)
	}
	for unevent, unfun := range toUnsubscribeS {
		_ = app.Unsubscribe(unevent, unfun)
	}

	for event, val := range expectedEvents {
		if val == false {
			t.Error("Did not fire config change event:", event)
			t.FailNow()
		}
	}

}

// start server, change config, send SIG HUP, confirm that the pidfile changed & backend reloaded
func TestServe(t *testing.T) {
	defer cleanTestArtifacts(t)
	var err error
	err = testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	if err != nil {
		t.Error("failed to generate a test certificate", err)
		t.FailNow()
	}

	mainlog, err = getTestLog()
	if err != nil {
		t.Error("could not get logger,", err)
		t.FailNow()
	}
	if err := ioutil.WriteFile("configJsonA.json", []byte(configJsonA), 0644); err != nil {
		t.Error(err)
		t.FailNow()
	}

	cmd := &cobra.Command{}
	configPath = "configJsonA.json"

	go func() {
		serve(cmd, []string{})
	}()
	if _, err := grepTestlog("istening on TCP 127.0.0.1:3536", 0); err != nil {
		t.Error("server not started")
	}
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
	err = ioutil.WriteFile("configJsonA.json", []byte(configJsonB), 0644)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	// test SIGHUP via the kill command
	// Would not work on windows as kill is not available.
	// TODO: Implement an alternative test for windows.
	if runtime.GOOS != "windows" {
		sigHup()
		// did the pidfile change as expected?
		if _, err := grepTestlog("Configuration was reloaded", 0); err != nil {
			t.Error("server did not catch sighp")
		}
	}
	// send kill signal and wait for exit
	d.Shutdown()

	// did backend started as expected?

	if _, err := grepTestlog("new backend started", 0); err != nil {
		t.Error("Dummy backend not restarted")
	}

	// wait for shutdown
	if _, err := grepTestlog("Backend shutdown completed", 0); err != nil {
		t.Error("server didn't stop")
	}

}

// Start with configJsonA.json,
// then add a new server to it (127.0.0.1:2526),
// then SIGHUP (to reload config & trigger config update events),
// then connect to it & HELO.
func TestServerAddEvent(t *testing.T) {
	var err error
	err = testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	if err != nil {
		t.Error("failed to generate a test certificate", err)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
	mainlog, err = getTestLog()
	if err != nil {
		t.Error("could not get logger,", err)
		t.FailNow()
	}
	// start the server by emulating the serve command
	if err := ioutil.WriteFile("configJsonA.json", []byte(configJsonA), 0644); err != nil {
		t.Error(err)
		t.FailNow()
	}
	cmd := &cobra.Command{}
	configPath = "configJsonA.json"
	go func() {
		serve(cmd, []string{})
	}()

	// allow the server to start
	if _, err := grepTestlog("Listening on TCP 127.0.0.1:3536", 0); err != nil {
		t.Error("server didn't start")
	}

	// now change the config by adding a server
	conf := &guerrilla.AppConfig{}       // blank one
	err = conf.Load([]byte(configJsonA)) // load configJsonA
	if err != nil {
		t.Error(err)
	}
	newServer := conf.Servers[0]                         // copy the first server config
	newServer.ListenInterface = "127.0.0.1:2526"         // change it
	newConf := conf                                      // copy the cmdConfg
	newConf.Servers = append(newConf.Servers, newServer) // add the new server
	if jsonbytes, err := json.Marshal(newConf); err == nil {
		if err := ioutil.WriteFile("configJsonA.json", jsonbytes, 0644); err != nil {
			t.Error(err)
		}
	}
	// send a sighup signal to the server
	sigHup()
	if _, err := grepTestlog("[127.0.0.1:2526] Waiting for a new client", 0); err != nil {
		t.Error("new server didn't start")
	}

	if conn, buffin, err := test.Connect(newServer, 20); err != nil {
		t.Error("Could not connect to new server", newServer.ListenInterface, err)
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

	// shutdown the server
	d.Shutdown()

	// did backend started as expected?
	if _, err := grepTestlog("New server added [127.0.0.1:2526]", 0); err != nil {
		t.Error("Did not add server [127.0.0.1:2526] after sighup")
	}

	if _, err := grepTestlog("Backend shutdown completed", 0); err != nil {
		t.Error("Server failed to stop")
	}

}

// Start with configJsonA.json,
// then change the config to enable 127.0.0.1:2228,
// then write the new config,
// then SIGHUP (to reload config & trigger config update events),
// then connect to 127.0.0.1:2228 & HELO.
func TestServerStartEvent(t *testing.T) {
	var err error
	err = testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	if err != nil {
		t.Error("failed to generate a test certificate", err)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
	mainlog, err = getTestLog()
	if err != nil {
		t.Error("could not get logger,", err)
		t.FailNow()
	}

	if err := ioutil.WriteFile("configJsonA.json", []byte(configJsonA), 0644); err != nil {
		t.Error(err)
		t.FailNow()
	}
	cmd := &cobra.Command{}
	configPath = "configJsonA.json"
	go func() {
		serve(cmd, []string{})
	}()
	if _, err := grepTestlog("Listening on TCP 127.0.0.1:3536", 0); err != nil {
		t.Error("server didn't start")
	}
	// now change the config by adding a server
	conf := &guerrilla.AppConfig{}                        // blank one
	if err = conf.Load([]byte(configJsonA)); err != nil { // load configJsonA
		t.Error(err)
	}
	newConf := conf // copy the cmdConfg
	newConf.Servers[1].IsEnabled = true
	if jsonbytes, err := json.Marshal(newConf); err == nil {
		//fmt.Println(string(jsonbytes))
		if err = ioutil.WriteFile("configJsonA.json", jsonbytes, 0644); err != nil {
			t.Error(err)
		}
	} else {
		t.Error(err)
	}
	// send a sighup signal to the server
	sigHup()

	// see if the new server started?
	if _, err := grepTestlog("Listening on TCP 127.0.0.1:2228", 0); err != nil {
		t.Error("second server didn't start")
	}

	// can we talk to it?
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
	// shutdown and wait for exit
	d.Shutdown()

	if _, err := grepTestlog("Backend shutdown completed", 0); err != nil {
		t.Error("server didn't stop")
	}

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
	var err error
	err = testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	if err != nil {
		t.Error("failed to generate a test certificate", err)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
	mainlog, err = getTestLog()
	if err != nil {
		t.Error("could not get logger,", err)
		t.FailNow()
	}
	if err := ioutil.WriteFile("configJsonA.json", []byte(configJsonA), 0644); err != nil {
		t.Error(err)
		t.FailNow()
	}
	cmd := &cobra.Command{}
	configPath = "configJsonA.json"

	go func() {
		serve(cmd, []string{})
	}()
	// allow the server to start
	if _, err := grepTestlog("Listening on TCP 127.0.0.1:3536", 0); err != nil {
		t.Error("server didn't start")
	}
	// now change the config by enabling a server
	conf := &guerrilla.AppConfig{}                        // blank one
	if err = conf.Load([]byte(configJsonA)); err != nil { // load configJsonA
		t.Error(err)
	}
	newConf := conf                     // copy the cmdConfg
	newConf.Servers[1].IsEnabled = true // enable 2nd server
	if jsonbytes, err := json.Marshal(newConf); err == nil {
		//fmt.Println(string(jsonbytes))
		if err = ioutil.WriteFile("configJsonA.json", jsonbytes, 0644); err != nil {
			t.Error(err)
		}
	} else {
		t.Error(err)
	}
	// send a sighup signal to the server
	sigHup()
	// detect config change
	if _, err := grepTestlog("Listening on TCP 127.0.0.1:2228", 0); err != nil {
		t.Error("new server didn't start")
	}

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
		if err = conn.Close(); err != nil {
			t.Error(err)
		}
	}
	// now disable the server
	newerConf := newConf // copy the cmdConfg
	newerConf.Servers[1].IsEnabled = false
	if jsonbytes, err := json.Marshal(newerConf); err == nil {
		//fmt.Println(string(jsonbytes))
		if err = ioutil.WriteFile("configJsonA.json", jsonbytes, 0644); err != nil {
			t.Error(err)
		}
	} else {
		t.Error(err)
	}
	// send a sighup signal to the server
	sigHup()
	// detect config change
	if _, err := grepTestlog("Server [127.0.0.1:2228] has stopped accepting new clients", 27); err != nil {
		t.Error("127.0.0.1:2228 did not stop")
	}

	// it should not connect to the server
	if _, _, err := test.Connect(newConf.Servers[1], 20); err == nil {
		t.Error("127.0.0.1:2228 was disabled, but still accepting connections", newConf.Servers[1].ListenInterface)
	}
	// shutdown wait for exit
	d.Shutdown()

	// wait for shutdown
	if _, err := grepTestlog("Backend shutdown completed", 0); err != nil {
		t.Error("server didn't stop")
	}
}

// just a utility for debugging when using the debugger, skipped by default
func TestDebug(t *testing.T) {

	t.SkipNow()
	conf := guerrilla.ServerConfig{ListenInterface: "127.0.0.1:2526"}
	if conn, buffin, err := test.Connect(conf, 20); err != nil {
		t.Error("Could not connect to new server", conf.ListenInterface, err)
	} else {
		if result, err := test.Command(conn, buffin, "HELO"); err == nil {
			expect := "250 mai1.guerrillamail.com Hello"
			if strings.Index(result, expect) != 0 {
				t.Error("Expected", expect, "but got", result)
			} else {
				if result, err = test.Command(conn, buffin, "RCPT TO:<test@grr.la>"); err == nil {
					expect := "250 2.1.5 OK"
					if strings.Index(result, expect) != 0 {
						t.Error("Expected:", expect, "but got:", result)
					}
				}
			}
		}
		_ = conn.Close()
	}
}

// Start with configJsonD.json,
// then connect to 127.0.0.1:4655 & HELO & try RCPT TO with an invalid host [grr.la]
// then change the config to enable add new host [grr.la] to allowed_hosts
// then write the new config,
// then SIGHUP (to reload config & trigger config update events),
// connect to 127.0.0.1:4655 & HELO & try RCPT TO, grr.la should work

func TestAllowedHostsEvent(t *testing.T) {
	var err error
	err = testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	if err != nil {
		t.Error("failed to generate a test certificate", err)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
	mainlog, err = getTestLog()
	if err != nil {
		t.Error("could not get logger,", err)
		t.FailNow()
	}
	if err := ioutil.WriteFile("configJsonD.json", []byte(configJsonD), 0644); err != nil {
		t.Error(err)
		t.FailNow()
	}
	// start the server by emulating the serve command

	conf := &guerrilla.AppConfig{}                        // blank one
	if err = conf.Load([]byte(configJsonD)); err != nil { // load configJsonD
		t.Error(err)
	}
	cmd := &cobra.Command{}
	configPath = "configJsonD.json"

	go func() {
		serve(cmd, []string{})
	}()
	// wait for start
	if _, err := grepTestlog("Listening on TCP 127.0.0.1:2552", 0); err != nil {
		t.Error("server didn't start")
	}

	// now connect and try RCPT TO with an invalid host
	if conn, buffin, err := test.Connect(conf.Servers[1], 20); err != nil {
		t.Error("Could not connect to new server", conf.Servers[1].ListenInterface, err)
	} else {
		if result, err := test.Command(conn, buffin, "HELO"); err == nil {
			expect := "250 secure.test.com Hello"
			if strings.Index(result, expect) != 0 {
				t.Error("Expected", expect, "but got", result)
			} else {
				if result, err = test.Command(conn, buffin, "RCPT TO:<test@grr.la>"); err == nil {
					expect := "454 4.1.1 Error: Relay access denied: grr.la"
					if strings.Index(result, expect) != 0 {
						t.Error("Expected:", expect, "but got:", result)
					}
				}
			}
		}
		_ = conn.Close()
	}

	// now change the config by adding a host to allowed hosts

	newConf := conf
	newConf.AllowedHosts = append(newConf.AllowedHosts, "grr.la")
	if jsonbytes, err := json.Marshal(newConf); err == nil {
		if err = ioutil.WriteFile("configJsonD.json", jsonbytes, 0644); err != nil {
			t.Error(err)
		}
	} else {
		t.Error(err)
	}
	// send a sighup signal to the server to reload config
	sigHup()

	if _, err := grepTestlog("allowed_hosts config changed", 0); err != nil {
		t.Error("allowed_hosts config not changed")
		t.FailNow()
	}

	// now repeat the same conversion, RCPT TO should be accepted
	if conn, buffin, err := test.Connect(conf.Servers[1], 20); err != nil {
		t.Error("Could not connect to new server", conf.Servers[1].ListenInterface, err)
	} else {
		if result, err := test.Command(conn, buffin, "HELO"); err == nil {
			expect := "250 secure.test.com Hello"
			if strings.Index(result, expect) != 0 {
				t.Error("Expected", expect, "but got", result)
			} else {
				if result, err = test.Command(conn, buffin, "RCPT TO:<test@grr.la>"); err == nil {
					expect := "250 2.1.5 OK"
					if strings.Index(result, expect) != 0 {
						t.Error("Expected:", expect, "but got:", result)
					}
				}
			}
		}
		_ = conn.Close()
	}

	// shutdown wait for exit
	d.Shutdown()

	// wait for shutdown
	if _, err := grepTestlog("Backend shutdown completed", 0); err != nil {
		t.Error("server didn't stop")
	}

}

// Test TLS config change event
// start with configJsonD
// should be able to STARTTLS to 127.0.0.1:2525 with no problems
// generate new certs & reload config
// should get a new tls event & able to STARTTLS with no problem

func TestTLSConfigEvent(t *testing.T) {
	var err error
	err = testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	if err != nil {
		t.Error("failed to generate a test certificate", err)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
	mainlog, err = getTestLog()
	if err != nil {
		t.Error("could not get logger,", err)
		t.FailNow()
	}
	if err := ioutil.WriteFile("configJsonD.json", []byte(configJsonD), 0644); err != nil {
		t.Error(err)
		t.FailNow()
	}
	conf := &guerrilla.AppConfig{}                        // blank one
	if err = conf.Load([]byte(configJsonD)); err != nil { // load configJsonD
		t.Error(err)
		t.FailNow()
	}
	cmd := &cobra.Command{}
	configPath = "configJsonD.json"

	go func() {
		serve(cmd, []string{})
	}()

	// wait for server to start
	if _, err := grepTestlog("Listening on TCP 127.0.0.1:2552", 0); err != nil {
		t.Error("server didn't start")
	}

	// Test STARTTLS handshake
	testTlsHandshake := func() {
		if conn, buffin, err := test.Connect(conf.Servers[0], 20); err != nil {
			t.Error("Could not connect to server", conf.Servers[0].ListenInterface, err)
		} else {
			if result, err := test.Command(conn, buffin, "HELO"); err == nil {
				expect := "250 mail.test.com Hello"
				if strings.Index(result, expect) != 0 {
					t.Error("Expected", expect, "but got", result)
				} else {
					if result, err = test.Command(conn, buffin, "STARTTLS"); err == nil {
						expect := "220 2.0.0 Ready to start TLS"
						if strings.Index(result, expect) != 0 {
							t.Error("Expected:", expect, "but got:", result)
						} else {
							tlsConn := tls.Client(conn, &tls.Config{
								InsecureSkipVerify: true,
								ServerName:         "127.0.0.1",
							})
							if err := tlsConn.Handshake(); err != nil {
								t.Error("Failed to handshake", conf.Servers[0].ListenInterface)
							} else {
								conn = tlsConn
								mainlog.Info("TLS Handshake succeeded")
							}
						}
					}
				}
			}
			_ = conn.Close()
		}
	}
	testTlsHandshake()

	// TLS Handshake succeeded?
	if _, err := grepTestlog("TLS Handshake succeeded", 0); err != nil {
		t.Error("TLS Handshake did not succeed")
		t.FailNow()
	}

	// now delete old certs, configure new certs, and send a sighup to load them in
	if err := deleteIfExists("../../tests/mail2.guerrillamail.com.cert.pem"); err != nil {
		t.Error("could not delete ../../tests/mail2.guerrillamail.com.cert.pem", err)
	}
	if err := deleteIfExists("../../tests/mail2.guerrillamail.com.key.pem"); err != nil {
		t.Error("could not delete ../../tests/mail2.guerrillamail.com.key.pem", err)
	}
	time.Sleep(testPauseDuration) // need to pause so that the new certs have different timestamps!
	// generate a new cert
	err = testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	if err != nil {
		t.Error("failed to generate a test certificate", err)
		t.FailNow()
	}
	// pause for generated cert to output (don't need, since we've fsynced)
	// time.Sleep(testPauseDuration) // (don't need, since we've fsynced)
	// did cert output?
	if _, err := os.Stat("../../tests/mail2.guerrillamail.com.cert.pem"); err != nil {
		t.Error("Did not create cert ", err)
	}

	sigHup()

	// wait for config to reload
	if _, err := grepTestlog("Server [127.0.0.1:4655] re-opened", 0); err != nil {
		t.Error("server didn't catch sighup")
	}

	// did tls configuration reload as expected?
	if _, err := grepTestlog("new TLS configuration loaded", 0); err != nil {
		t.Error("server didn't catch sighup")
	}

	// test again
	testTlsHandshake()

	// after line 25
	if _, err := grepTestlog("TLS Handshake succeeded", 25); err != nil {
		t.Error("TLS Handshake did not succeed")
		t.FailNow()
	}

	d.Shutdown()

	// wait for shutdown
	if _, err := grepTestlog("Backend shutdown completed", 0); err != nil {
		t.Error("server didn't stop")
	}

}

// Testing starting a server with a bad TLS config
// It should not start, return exit code 1
func TestBadTLSStart(t *testing.T) {
	var err error
	mainlog, err = getTestLog()
	if err != nil {
		t.Error("could not get logger,", err)
		t.FailNow()
	}
	// Need to run the test in a different process by executing a command
	// because the serve() does os.Exit when starting with a bad TLS config
	if os.Getenv("BE_CRASHER") == "1" {
		// do the test
		// first, remove the good certs, if any
		if err := deleteIfExists("../../tests/mail2.guerrillamail.com.cert.pem"); err != nil {
			t.Error("could not delete ../../tests/mail2.guerrillamail.com.cert.pem", err)
		}
		if err := deleteIfExists("../../tests/mail2.guerrillamail.com.key.pem"); err != nil {
			t.Error("could not delete ../../tests/mail2.guerrillamail.com.key.pem", err)
		}
		// next run the server
		if err = ioutil.WriteFile("configJsonD.json", []byte(configJsonD), 0644); err != nil {
			t.Error(err)
		}
		conf := &guerrilla.AppConfig{}                        // blank one
		if err = conf.Load([]byte(configJsonD)); err != nil { // load configJsonD
			t.Error(err)
		}
		cmd := &cobra.Command{}
		configPath = "configJsonD.json"
		var serveWG sync.WaitGroup

		serveWG.Add(1)
		go func() {
			serve(cmd, []string{})
			serveWG.Done()
		}()
		// it should exit by now because the TLS config is incorrect
		time.Sleep(testPauseDuration)

		sigKill()
		serveWG.Wait()

		return
	}
	defer cleanTestArtifacts(t)

	cmd := exec.Command(os.Args[0], "-test.run=TestBadTLSStart")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	err = cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		if _, err := grepTestlog("level=fatal", 0); err != nil {
			t.Error("server didn't exit with a fatal error")
		}
		return
	}
	t.Error("Server started with a bad TLS config, was expecting exit status 0")

}

// Test config reload with a bad TLS config
// It should ignore the config reload, keep running with old settings
func TestBadTLSReload(t *testing.T) {
	var err error
	mainlog, err = getTestLog()
	if err != nil {
		t.Error("could not get logger,", err)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
	// start with a good cert
	err = testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	if err != nil {
		t.Error("failed to generate a test certificate", err)
		t.FailNow()
	}
	// start the server by emulating the serve command
	if err = ioutil.WriteFile("configJsonD.json", []byte(configJsonD), 0644); err != nil {
		t.Error(err)
		t.FailNow()
	}
	conf := &guerrilla.AppConfig{}                        // blank one
	if err = conf.Load([]byte(configJsonD)); err != nil { // load configJsonD
		t.Error(err)
		t.FailNow()
	}
	cmd := &cobra.Command{}
	configPath = "configJsonD.json"

	go func() {
		serve(cmd, []string{})
	}()
	// wait for server to start
	if _, err := grepTestlog("Listening on TCP 127.0.0.1:4655", 0); err != nil {
		t.Error("server didn't start")
	}

	if conn, buffin, err := test.Connect(conf.Servers[0], 20); err != nil {
		t.Error("Could not connect to server", conf.Servers[0].ListenInterface, err)
	} else {
		if result, err := test.Command(conn, buffin, "HELO"); err == nil {
			expect := "250 mail.test.com Hello"
			if strings.Index(result, expect) != 0 {
				t.Error("Expected", expect, "but got", result)
			}
		}
	}
	// write some trash data
	if err = ioutil.WriteFile("./../../tests/mail2.guerrillamail.com.cert.pem",
		[]byte("trash data"),
		0664); err != nil {
		t.Error(err)
	}
	if err = ioutil.WriteFile("./../../tests/mail2.guerrillamail.com.key.pem",
		[]byte("trash data"),
		0664); err != nil {
		t.Error(err)
	}

	newConf := conf // copy the cmdConfg

	if jsonbytes, err := json.Marshal(newConf); err == nil {
		if err = ioutil.WriteFile("configJsonD.json", jsonbytes, 0644); err != nil {
			t.Error(err)
		}
	} else {
		t.Error(err)
	}
	// send a sighup signal to the server to reload config
	sigHup()
	// did the config reload reload event fire? There should be config read error
	if _, err := grepTestlog("could not read config file", 0); err != nil {
		t.Error("was expecting an error reading config")
	}

	// we should still be able to to talk to it

	if conn, buffin, err := test.Connect(conf.Servers[0], 20); err != nil {
		t.Error("Could not connect to server", conf.Servers[0].ListenInterface, err)
	} else {
		if result, err := test.Command(conn, buffin, "HELO"); err == nil {
			expect := "250 mail.test.com Hello"
			if strings.Index(result, expect) != 0 {
				t.Error("Expected", expect, "but got", result)
			}
		}
	}

	// shutdown & wait for exit
	d.Shutdown()

	// wait for shutdown
	if _, err := grepTestlog("Backend shutdown completed", 0); err != nil {
		t.Error("server didn't stop")
	}

}

// Test for when the server config Timeout value changes
// Start with configJsonD.json

func TestSetTimeoutEvent(t *testing.T) {
	var err error
	mainlog, err = getTestLog()
	if err != nil {
		t.Error("could not get logger,", err)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
	err = testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	if err != nil {
		t.Error("failed to generate a test certificate", err)
		t.FailNow()
	}
	// start the server by emulating the serve command
	if err = ioutil.WriteFile("configJsonD.json", []byte(configJsonD), 0644); err != nil {
		t.Error(err)
	}
	conf := &guerrilla.AppConfig{}                        // blank one
	if err = conf.Load([]byte(configJsonD)); err != nil { // load configJsonD
		t.Error(err)
	}
	cmd := &cobra.Command{}
	configPath = "configJsonD.json"

	go func() {
		serve(cmd, []string{})
	}()
	// wait for start
	if _, err := grepTestlog("Listening on TCP 127.0.0.1:4655", 0); err != nil {
		t.Error("server didn't start")
	}

	// set the timeout to 1 second

	newConf := conf // copy the cmdConfg
	newConf.Servers[0].Timeout = 1
	if jsonbytes, err := json.Marshal(newConf); err == nil {
		if err = ioutil.WriteFile("configJsonD.json", jsonbytes, 0644); err != nil {
			t.Error(err)
		}
	} else {
		t.Error(err)
	}

	// send a sighup signal to the server to reload config
	sigHup()

	// did config update?
	if _, err := grepTestlog("a new config has been saved", 0); err != nil {
		t.Error("config didn't update")
	}

	var waitTimeout sync.WaitGroup
	if conn, buffin, err := test.Connect(conf.Servers[0], 20); err != nil {
		t.Error("Could not connect to server", conf.Servers[0].ListenInterface, err)
	} else {
		waitTimeout.Add(1)
		go func() {
			if result, err := test.Command(conn, buffin, "HELO"); err == nil {
				expect := "250 mail.test.com Hello"
				if strings.Index(result, expect) != 0 {
					t.Error("Expected", expect, "but got", result)
				} else {
					b := make([]byte, 1024)
					_, _ = conn.Read(b)
				}
			}
			waitTimeout.Done()
		}()
	}

	// wait for timeout
	waitTimeout.Wait()

	d.Shutdown()

	// wait for shutdown
	if _, err := grepTestlog("Backend shutdown completed", 0); err != nil {
		t.Error("server didn't stop")
	}
	// so the connection we have opened should timeout by now

	// did we get timeout as expected?
	if _, err := grepTestlog("i/o timeout", 0); err != nil {
		t.Error("it looks like the timeout config didnt change")
		t.FailNow()
	}

}

// Test debug level config change
// Start in log_level = debug
// Load config & start server
func TestDebugLevelChange(t *testing.T) {
	var err error
	mainlog, err = getTestLog()
	if err != nil {
		t.Error("could not get logger,", err)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
	err = testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	if err != nil {
		t.Error("failed to generate a test certificate", err)
		t.FailNow()
	} // start the server by emulating the serve command
	if err = ioutil.WriteFile("configJsonD.json", []byte(configJsonD), 0644); err != nil {
		t.Error(err)
	}
	conf := &guerrilla.AppConfig{}                        // blank one
	if err = conf.Load([]byte(configJsonD)); err != nil { // load configJsonD
		t.Error(err)
	}
	conf.LogLevel = "debug"
	cmd := &cobra.Command{}
	configPath = "configJsonD.json"

	go func() {
		serve(cmd, []string{})
	}()
	if _, err := grepTestlog("Listening on TCP 127.0.0.1:2552", 0); err != nil {
		t.Error("server didn't start")
	}

	if conn, buffin, err := test.Connect(conf.Servers[0], 20); err != nil {
		t.Error("Could not connect to server", conf.Servers[0].ListenInterface, err)
	} else {
		if result, err := test.Command(conn, buffin, "HELO"); err == nil {
			expect := "250 mail.test.com Hello"
			if strings.Index(result, expect) != 0 {
				t.Error("Expected", expect, "but got", result)
			}
		}
		_ = conn.Close()
	}
	// set the log_level to info

	newConf := conf // copy the cmdConfg
	newConf.LogLevel = log.InfoLevel.String()
	if jsonbytes, err := json.Marshal(newConf); err == nil {
		if err = ioutil.WriteFile("configJsonD.json", jsonbytes, 0644); err != nil {
			t.Error(err)
		}
	} else {
		t.Error(err)
	}
	// send a sighup signal to the server to reload config
	sigHup()
	// did the config reload?
	if _, err := grepTestlog("Configuration was reloaded", 0); err != nil {
		t.Error("config did not reload")
		t.FailNow()
	}

	// connect again, this time we should see info
	if conn, buffin, err := test.Connect(conf.Servers[0], 20); err != nil {
		t.Error("Could not connect to server", conf.Servers[0].ListenInterface, err)
	} else {
		if result, err := test.Command(conn, buffin, "NOOP"); err == nil {
			expect := "200 2.0.0 OK"
			if strings.Index(result, expect) != 0 {
				t.Error("Expected", expect, "but got", result)
			}
		}
		_ = conn.Close()
	}

	d.Shutdown()

	// did the log level change to info?
	if _, err := grepTestlog("log level changed to [info]", 0); err != nil {
		t.Error("log level did not change to [info]")
		t.FailNow()
	}

}

// When reloading with a bad backend config, it should revert to old backend config
func TestBadBackendReload(t *testing.T) {
	var err error
	err = testcert.GenerateCert("mail2.guerrillamail.com", "", 365*24*time.Hour, false, 2048, "P256", "../../tests/")
	if err != nil {
		t.Error("failed to generate a test certificate", err)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)

	mainlog, err = getTestLog()
	if err != nil {
		t.Error("could not get logger,", err)
		t.FailNow()
	}

	if err = ioutil.WriteFile("configJsonA.json", []byte(configJsonA), 0644); err != nil {
		t.Error(err)
	}
	cmd := &cobra.Command{}
	configPath = "configJsonA.json"
	go func() {
		serve(cmd, []string{})
	}()
	if _, err := grepTestlog("Listening on TCP 127.0.0.1:3536", 0); err != nil {
		t.Error("server didn't start")
	}

	// change the config file to the one with a broken backend
	if err = ioutil.WriteFile("configJsonA.json", []byte(configJsonE), 0644); err != nil {
		t.Error(err)
	}

	// test SIGHUP via the kill command
	// Would not work on windows as kill is not available.
	// TODO: Implement an alternative test for windows.
	if runtime.GOOS != "windows" {
		sigHup()
		// did config update?
		if _, err := grepTestlog("Configuration was reloaded", 0); err != nil {
			t.Error("config didn't update")
		}
		// did the pidfile change as expected?

		if _, err := grepTestlog("pid_file (./pidfile2.pid) written", 0); err != nil {
			t.Error("pid_file (./pidfile2.pid) not written")
		}
		if _, err := os.Stat("./pidfile2.pid"); os.IsNotExist(err) {
			t.Error("pidfile not changed after sighup SIGHUP", err)
		}
	}

	// send kill signal and wait for exit
	d.Shutdown()

	// did backend started as expected?
	if _, err := grepTestlog("reverted to old backend config", 0); err != nil {
		t.Error("did not revert to old backend config")
		t.FailNow()
	}
}
