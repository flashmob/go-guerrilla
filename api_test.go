package guerrilla

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/response"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// Test Starting smtp without setting up logger / backend
func TestSMTP(t *testing.T) {
	done := make(chan bool)
	go func() {
		select {
		case <-time.After(time.Second * 40):
			t.Error("timeout")
			return
		case <-done:
			return
		}
	}()

	d := Daemon{}
	err := d.Start()

	if err != nil {
		t.Error(err)
	}
	// it should set to stderr automatically
	if d.Config.LogFile != log.OutputStderr.String() {
		t.Error("smtp.config.LogFile is not", log.OutputStderr.String())
	}

	if len(d.Config.AllowedHosts) == 0 {
		t.Error("smtp.config.AllowedHosts len should be 1, not 0", d.Config.AllowedHosts)
	}

	if d.Config.LogLevel != "debug" {
		t.Error("smtp.config.LogLevel expected'debug', it is", d.Config.LogLevel)
	}
	if len(d.Config.Servers) != 1 {
		t.Error("len(smtp.config.Servers) should be 1, got", len(d.Config.Servers))
	}
	time.Sleep(time.Second * 2)
	d.Shutdown()
	done <- true

}

// Suppressing log output
func TestSMTPNoLog(t *testing.T) {

	// configure a default server with no log output
	cfg := &AppConfig{LogFile: log.OutputOff.String()}
	d := Daemon{Config: cfg}

	err := d.Start()
	if err != nil {
		t.Error(err)
	}
	time.Sleep(time.Second * 2)
	d.Shutdown()
}

// our custom server
func TestSMTPCustomServer(t *testing.T) {
	cfg := &AppConfig{LogFile: log.OutputOff.String()}
	sc := ServerConfig{
		ListenInterface: "127.0.0.1:2526",
		IsEnabled:       true,
	}
	cfg.Servers = append(cfg.Servers, sc)
	d := Daemon{Config: cfg}

	err := d.Start()
	if err != nil {
		t.Error("start error", err)
	} else {
		time.Sleep(time.Second * 2)
		d.Shutdown()
	}

}

// with a backend config
func TestSMTPCustomBackend(t *testing.T) {
	cfg := &AppConfig{LogFile: log.OutputOff.String()}
	sc := ServerConfig{
		ListenInterface: "127.0.0.1:2526",
		IsEnabled:       true,
	}
	cfg.Servers = append(cfg.Servers, sc)
	bcfg := backends.BackendConfig{
		"save_workers_size":  3,
		"save_process":       "HeadersParser|Header|Hasher|Debugger",
		"log_received_mails": true,
		"primary_mail_host":  "example.com",
	}
	cfg.BackendConfig = bcfg
	d := Daemon{Config: cfg}

	err := d.Start()
	if err != nil {
		t.Error("start error", err)
	} else {
		time.Sleep(time.Second * 2)
		d.Shutdown()
	}
}

// with a config from a json file
func TestSMTPLoadFile(t *testing.T) {
	json := `{
    "log_file" : "./tests/testlog",
    "log_level" : "debug",
    "pid_file" : "tests/go-guerrilla.pid",
    "allowed_hosts": ["spam4.me","grr.la"],
    "backend_config" :
        {
            "log_received_mails" : true,
            "save_process": "HeadersParser|Header|Hasher|Debugger",
            "save_workers_size":  3
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
				"private_key_file":"config_test.go",
            	"public_key_file":"config_test.go",
				"start_tls_on":false,
            	"tls_always_on":false
			}
        }
    ]
}

	`
	json2 := `{
    "log_file" : "./tests/testlog2",
    "log_level" : "debug",
    "pid_file" : "tests/go-guerrilla2.pid",
    "allowed_hosts": ["spam4.me","grr.la"],
    "backend_config" :
        {
            "log_received_mails" : true,
            "save_process": "HeadersParser|Header|Hasher|Debugger",
            "save_workers_size":  3
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
 				"private_key_file":"config_test.go",
				"public_key_file":"config_test.go",
				"start_tls_on":false,
            	"tls_always_on":false
			}
        }
    ]
}

	`
	err := ioutil.WriteFile("goguerrilla.conf.api", []byte(json), 0644)
	if err != nil {
		t.Error("could not write guerrilla.conf.api", err)
		return
	}

	d := Daemon{}
	_, err = d.LoadConfig("goguerrilla.conf.api")
	if err != nil {
		t.Error("ReadConfig error", err)
		return
	}

	err = d.Start()
	if err != nil {
		t.Error("start error", err)
		return
	} else {
		time.Sleep(time.Second * 2)
		if d.Config.LogFile != "./tests/testlog" {
			t.Error("d.Config.LogFile != \"./tests/testlog\"")
		}

		if d.Config.PidFile != "tests/go-guerrilla.pid" {
			t.Error("d.Config.LogFile != tests/go-guerrilla.pid")
		}

		err := ioutil.WriteFile("goguerrilla.conf.api", []byte(json2), 0644)
		if err != nil {
			t.Error("could not write guerrilla.conf.api", err)
			return
		}

		if err = d.ReloadConfigFile("goguerrilla.conf.api"); err != nil {
			t.Error(err)
		}

		if d.Config.LogFile != "./tests/testlog2" {
			t.Error("d.Config.LogFile != \"./tests/testlog\"")
		}

		if d.Config.PidFile != "tests/go-guerrilla2.pid" {
			t.Error("d.Config.LogFile != \"go-guerrilla.pid\"")
		}

		d.Shutdown()
	}
}

// test re-opening the main log
func TestReopenLog(t *testing.T) {
	if err := os.Truncate("tests/testlog", 0); err != nil {
		t.Error(err)
	}
	cfg := &AppConfig{LogFile: "tests/testlog"}
	sc := ServerConfig{
		ListenInterface: "127.0.0.1:2526",
		IsEnabled:       true,
	}
	cfg.Servers = append(cfg.Servers, sc)
	d := Daemon{Config: cfg}

	err := d.Start()
	if err != nil {
		t.Error("start error", err)
	} else {
		if err = d.ReopenLogs(); err != nil {
			t.Error(err)
		}
		time.Sleep(time.Second * 2)

		d.Shutdown()
	}

	b, err := ioutil.ReadFile("tests/testlog")
	if err != nil {
		t.Error("could not read logfile")
		return
	}
	if !strings.Contains(string(b), "re-opened log file") {
		t.Error("Server log did not re-opened, expecting \"re-opened log file\"")
	}
	if !strings.Contains(string(b), "re-opened main log file") {
		t.Error("Main log did not re-opened, expecting \"re-opened main log file\"")
	}
}

const testServerLog = "tests/testlog-server.log"

// test re-opening the individual server log
func TestReopenServerLog(t *testing.T) {
	if err := os.Truncate("tests/testlog", 0); err != nil {
		t.Error(err)
	}

	defer func() {
		if _, err := os.Stat(testServerLog); err == nil {
			if err = os.Remove(testServerLog); err != nil {
				t.Error(err)
			}
		}
	}()

	cfg := &AppConfig{LogFile: "tests/testlog", LogLevel: log.DebugLevel.String(), AllowedHosts: []string{"grr.la"}}
	sc := ServerConfig{
		ListenInterface: "127.0.0.1:2526",
		IsEnabled:       true,
		LogFile:         testServerLog,
	}
	cfg.Servers = append(cfg.Servers, sc)
	d := Daemon{Config: cfg}

	err := d.Start()
	if err != nil {
		t.Error("start error", err)
	} else {
		if err := talkToServer("127.0.0.1:2526"); err != nil {
			t.Error(err)
		}
		if err = d.ReopenLogs(); err != nil {
			t.Error(err)
		}
		time.Sleep(time.Second * 2)
		if err := talkToServer("127.0.0.1:2526"); err != nil {
			t.Error(err)
		}
		d.Shutdown()
	}

	b, err := ioutil.ReadFile("tests/testlog")
	if err != nil {
		t.Error("could not read logfile")
		return
	}
	if !strings.Contains(string(b), "re-opened log file") {
		t.Error("Server log did not re-opened, expecting \"re-opened log file\"")
	}
	if !strings.Contains(string(b), "re-opened main log file") {
		t.Error("Main log did not re-opened, expecting \"re-opened main log file\"")
	}

	b, err = ioutil.ReadFile(testServerLog)
	if err != nil {
		t.Error("could not read logfile")
		return
	}

	if !strings.Contains(string(b), "Handle client") {
		t.Error("server log does not contain \"handle client\"")
	}

}

func TestSetConfig(t *testing.T) {

	if err := os.Truncate("tests/testlog", 0); err != nil {
		t.Error(err)
	}
	cfg := AppConfig{LogFile: "tests/testlog"}
	sc := ServerConfig{
		ListenInterface: "127.0.0.1:2526",
		IsEnabled:       true,
	}
	cfg.Servers = append(cfg.Servers, sc)
	d := Daemon{Config: &cfg}

	// lets add a new server
	sc.ListenInterface = "127.0.0.1:2527"
	cfg.Servers = append(cfg.Servers, sc)

	err := d.SetConfig(cfg)
	if err != nil {
		t.Error("SetConfig returned an error:", err)
		return
	}

	err = d.Start()
	if err != nil {
		t.Error("start error", err)
	} else {

		time.Sleep(time.Second * 2)

		d.Shutdown()
	}

	b, err := ioutil.ReadFile("tests/testlog")
	if err != nil {
		t.Error("could not read logfile")
		return
	}
	//fmt.Println(string(b))
	// has 127.0.0.1:2527 started?
	if !strings.Contains(string(b), "127.0.0.1:2527") {
		t.Error("expecting 127.0.0.1:2527 to start")
	}

}

func TestSetConfigError(t *testing.T) {

	if err := os.Truncate("tests/testlog", 0); err != nil {
		t.Error(err)
	}
	cfg := AppConfig{LogFile: "tests/testlog"}
	sc := ServerConfig{
		ListenInterface: "127.0.0.1:2526",
		IsEnabled:       true,
	}
	cfg.Servers = append(cfg.Servers, sc)
	d := Daemon{Config: &cfg}

	// lets add a new server with bad TLS
	sc.ListenInterface = "127.0.0.1:2527"
	sc.TLS.StartTLSOn = true
	sc.TLS.PublicKeyFile = "tests/testlog"  // totally wrong :->
	sc.TLS.PrivateKeyFile = "tests/testlog" // totally wrong :->

	cfg.Servers = append(cfg.Servers, sc)

	err := d.SetConfig(cfg)
	if err == nil {
		t.Error("SetConfig should have returned an error compalning about bad tls settings")
		return
	}
}

var funkyLogger = func() backends.Decorator {

	backends.Svc.AddInitializer(
		backends.InitializeWith(
			func(backendConfig backends.BackendConfig) error {
				backends.Log().Info("Funky logger is up & down to funk!")
				return nil
			}),
	)

	backends.Svc.AddShutdowner(
		backends.ShutdownWith(
			func() error {
				backends.Log().Info("The funk has been stopped!")
				return nil
			}),
	)

	return func(p backends.Processor) backends.Processor {
		return backends.ProcessWith(
			func(e *mail.Envelope, task backends.SelectTask) (backends.Result, error) {
				if task == backends.TaskValidateRcpt {
					// log the last recipient appended to e.Rcpt
					backends.Log().Infof(
						"another funky recipient [%s]",
						e.RcptTo[len(e.RcptTo)-1])
					// if valid then forward call to the next processor in the chain
					return p.Process(e, task)
					// if invalid, return a backend result
					//return backends.NewResult(response.Canned.FailRcptCmd), nil
				} else if task == backends.TaskSaveMail {
					backends.Log().Info("Another funky email!")
				}
				return p.Process(e, task)
			})
	}
}

// How about a custom processor?
func TestSetAddProcessor(t *testing.T) {
	if err := os.Truncate("tests/testlog", 0); err != nil {
		t.Error(err)
	}
	cfg := &AppConfig{
		LogFile:      "tests/testlog",
		AllowedHosts: []string{"grr.la"},
		BackendConfig: backends.BackendConfig{
			"save_process":     "HeadersParser|Debugger|FunkyLogger",
			"validate_process": "FunkyLogger",
		},
	}
	d := Daemon{Config: cfg}
	d.AddProcessor("FunkyLogger", funkyLogger)

	if err := d.Start(); err != nil {
		t.Error(err)
	}
	// lets have a talk with the server
	if err := talkToServer("127.0.0.1:2525"); err != nil {
		t.Error(err)
	}

	d.Shutdown()

	b, err := ioutil.ReadFile("tests/testlog")
	if err != nil {
		t.Error("could not read logfile")
		return
	}
	// lets check for fingerprints
	if !strings.Contains(string(b), "another funky recipient") {
		t.Error("did not log: another funky recipient")
	}

	if !strings.Contains(string(b), "Another funky email!") {
		t.Error("Did not log: Another funky email!")
	}

	if !strings.Contains(string(b), "Funky logger is up & down to funk") {
		t.Error("Did not log: Funky logger is up & down to funk")
	}
	if !strings.Contains(string(b), "The funk has been stopped!") {
		t.Error("Did not log:The funk has been stopped!")
	}

}

func talkToServer(address string) (err error) {

	conn, err := net.Dial("tcp", address)
	if err != nil {
		return
	}
	in := bufio.NewReader(conn)
	str, err := in.ReadString('\n')
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(conn, "HELO maildiranasaurustester\r\n")
	if err != nil {
		return err
	}
	str, err = in.ReadString('\n')
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(conn, "MAIL FROM:<test@example.com>r\r\n")
	if err != nil {
		return err
	}
	str, err = in.ReadString('\n')
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(conn, "RCPT TO:<test@grr.la>\r\n")
	if err != nil {
		return err
	}
	str, err = in.ReadString('\n')
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(conn, "DATA\r\n")
	if err != nil {
		return err
	}
	str, err = in.ReadString('\n')
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(conn, "Subject: Test subject\r\n")
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(conn, "\r\n")
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(conn, "A an email body\r\n")
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(conn, ".\r\n")
	if err != nil {
		return err
	}
	str, err = in.ReadString('\n')
	if err != nil {
		return err
	}
	_ = str
	return nil
}

// Test hot config reload
// Here we forgot to add FunkyLogger so backend will fail to init
// it will log to stderr at the beginning, but then change to tests/testlog

func TestReloadConfig(t *testing.T) {
	if err := os.Truncate("tests/testlog", 0); err != nil {
		t.Error(err)
	}
	d := Daemon{}
	if err := d.Start(); err != nil {
		t.Error(err)
	}
	defer d.Shutdown()
	cfg := AppConfig{
		LogFile:      "tests/testlog",
		AllowedHosts: []string{"grr.la"},
		BackendConfig: backends.BackendConfig{
			"save_process":     "HeadersParser|Debugger|FunkyLogger",
			"validate_process": "FunkyLogger",
		},
	}
	// Look mom, reloading the config without shutting down!
	if err := d.ReloadConfig(cfg); err != nil {
		t.Error(err)
	}

}

func TestPubSubAPI(t *testing.T) {

	if err := os.Truncate("tests/testlog", 0); err != nil {
		t.Error(err)
	}

	d := Daemon{Config: &AppConfig{LogFile: "tests/testlog"}}
	if err := d.Start(); err != nil {
		t.Error(err)
	}
	defer d.Shutdown()
	// new config
	cfg := AppConfig{
		PidFile:      "tests/pidfilex.pid",
		LogFile:      "tests/testlog",
		AllowedHosts: []string{"grr.la"},
		BackendConfig: backends.BackendConfig{
			"save_process":     "HeadersParser|Debugger|FunkyLogger",
			"validate_process": "FunkyLogger",
		},
	}

	var i = 0
	pidEvHandler := func(c *AppConfig) {
		i++
		if i > 1 {
			t.Error("number > 1, it means d.Unsubscribe didn't work")
		}
		d.Logger.Info("number", i)
	}
	if err := d.Subscribe(EventConfigPidFile, pidEvHandler); err != nil {
		t.Error(err)
	}

	if err := d.ReloadConfig(cfg); err != nil {
		t.Error(err)
	}

	if err := d.Unsubscribe(EventConfigPidFile, pidEvHandler); err != nil {
		t.Error(err)
	}
	cfg.PidFile = "tests/pidfile2.pid"
	d.Publish(EventConfigPidFile, &cfg)
	if err := d.ReloadConfig(cfg); err != nil {
		t.Error(err)
	}

	b, err := ioutil.ReadFile("tests/testlog")
	if err != nil {
		t.Error("could not read logfile")
		return
	}
	// lets interrogate the log
	if !strings.Contains(string(b), "number1") {
		t.Error("it lools like d.ReloadConfig(cfg) did not fire EventConfigPidFile, pidEvHandler not called")
	}

}

func TestAPILog(t *testing.T) {
	if err := os.Truncate("tests/testlog", 0); err != nil {
		t.Error(err)
	}
	d := Daemon{}
	l := d.Log()
	l.Info("logtest1") // to stderr
	if l.GetLevel() != log.InfoLevel.String() {
		t.Error("Log level does not eq info, it is ", l.GetLevel())
	}
	d.Logger = nil
	d.Config = &AppConfig{LogFile: "tests/testlog"}
	l = d.Log()
	l.Info("logtest1") // to tests/testlog

	//
	l = d.Log()
	if l.GetLogDest() != "tests/testlog" {
		t.Error("log dest is not tests/testlog, it was ", l.GetLogDest())
	}

	b, err := ioutil.ReadFile("tests/testlog")
	if err != nil {
		t.Error("could not read logfile")
		return
	}
	// lets interrogate the log
	if !strings.Contains(string(b), "logtest1") {
		t.Error("hai was not found in the log, it should have been in tests/testlog")
	}
}

// Test the allowed_hosts config option with a single entry of ".", which will allow all hosts.
func TestSkipAllowsHost(t *testing.T) {

	d := Daemon{}
	defer d.Shutdown()
	// setting the allowed hosts to a single entry with a dot will let any host through
	d.Config = &AppConfig{AllowedHosts: []string{"."}, LogFile: "off"}
	if err := d.Start(); err != nil {
		t.Error(err)
	}

	conn, err := net.Dial("tcp", d.Config.Servers[0].ListenInterface)
	if err != nil {
		t.Error(t)
		return
	}
	in := bufio.NewReader(conn)
	if _, err := fmt.Fprint(conn, "HELO test\r\n"); err != nil {
		t.Error(err)
	}
	if _, err := fmt.Fprint(conn, "RCPT TO:<test@funkyhost.com>\r\n"); err != nil {
		t.Error(err)
	}

	if _, err := in.ReadString('\n'); err != nil {
		t.Error(err)
	}
	if _, err := in.ReadString('\n'); err != nil {
		t.Error(err)
	}
	str, _ := in.ReadString('\n')
	if strings.Index(str, "250") != 0 {
		t.Error("expected 250 reply, got:", str)
	}

}

var customBackend2 = func() backends.Decorator {

	return func(p backends.Processor) backends.Processor {
		return backends.ProcessWith(
			func(e *mail.Envelope, task backends.SelectTask) (backends.Result, error) {
				if task == backends.TaskValidateRcpt {
					return p.Process(e, task)
				} else if task == backends.TaskSaveMail {
					backends.Log().Info("Another funky email!")
					err := errors.New("system shock")
					return backends.NewResult(response.Canned.FailReadErrorDataCmd, response.SP, err), err
				}
				return p.Process(e, task)
			})
	}
}

// Test a custom backend response
func TestCustomBackendResult(t *testing.T) {
	if err := os.Truncate("tests/testlog", 0); err != nil {
		t.Error(err)
	}
	cfg := &AppConfig{
		LogFile:      "tests/testlog",
		AllowedHosts: []string{"grr.la"},
		BackendConfig: backends.BackendConfig{
			"save_process":     "HeadersParser|Debugger|Custom",
			"validate_process": "Custom",
		},
	}
	d := Daemon{Config: cfg}
	d.AddProcessor("Custom", customBackend2)

	if err := d.Start(); err != nil {
		t.Error(err)
	}
	// lets have a talk with the server
	if err := talkToServer("127.0.0.1:2525"); err != nil {
		t.Error(err)
	}

	d.Shutdown()

	b, err := ioutil.ReadFile("tests/testlog")
	if err != nil {
		t.Error("could not read logfile")
		return
	}
	// lets check for fingerprints
	if !strings.Contains(string(b), "451 4.3.0 Error") {
		t.Error("did not log: 451 4.3.0 Error")
	}

	if !strings.Contains(string(b), "system shock") {
		t.Error("did not log: system shock")
	}

}
