package guerrilla

import (
	"bufio"
	"fmt"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
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
            "private_key_file":"config_test.go",
            "public_key_file":"config_test.go",
            "timeout":160,
            "listen_interface":"127.0.0.1:2526",
            "start_tls_on":false,
            "tls_always_on":false,
            "max_clients": 2
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
            "private_key_file":"config_test.go",
            "public_key_file":"config_test.go",
            "timeout":160,
            "listen_interface":"127.0.0.1:2526",
            "start_tls_on":false,
            "tls_always_on":false,
            "max_clients": 2
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

		d.ReloadConfigFile("goguerrilla.conf.api")

		if d.Config.LogFile != "./tests/testlog2" {
			t.Error("d.Config.LogFile != \"./tests/testlog\"")
		}

		if d.Config.PidFile != "tests/go-guerrilla2.pid" {
			t.Error("d.Config.LogFile != \"go-guerrilla.pid\"")
		}

		d.Shutdown()
	}
}

func TestReopenLog(t *testing.T) {
	os.Truncate("test/testlog", 0)
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
		d.ReopenLogs()
		time.Sleep(time.Second * 2)

		d.Shutdown()
	}

	b, err := ioutil.ReadFile("tests/testlog")
	if err != nil {
		t.Error("could not read logfile")
		return
	}
	if strings.Index(string(b), "re-opened log file") < 0 {
		t.Error("Server log did not re-opened, expecting \"re-opened log file\"")
	}
	if strings.Index(string(b), "re-opened main log file") < 0 {
		t.Error("Main log did not re-opened, expecting \"re-opened main log file\"")
	}
}

func TestSetConfig(t *testing.T) {

	os.Truncate("test/testlog", 0)
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
	if strings.Index(string(b), "127.0.0.1:2527") < 0 {
		t.Error("expecting 127.0.0.1:2527 to start")
	}

}

func TestSetConfigError(t *testing.T) {

	os.Truncate("tests/testlog", 0)
	cfg := AppConfig{LogFile: "tests/testlog"}
	sc := ServerConfig{
		ListenInterface: "127.0.0.1:2526",
		IsEnabled:       true,
	}
	cfg.Servers = append(cfg.Servers, sc)
	d := Daemon{Config: &cfg}

	// lets add a new server with bad TLS
	sc.ListenInterface = "127.0.0.1:2527"
	sc.StartTLSOn = true
	sc.PublicKeyFile = "tests/testlog" // totally wrong :->
	sc.PublicKeyFile = "tests/testlog" // totally wrong :->

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
					// validate the last recipient appended to e.Rcpt
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
	os.Truncate("tests/testlog", 0)
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

	d.Start()
	// lets have a talk with the server
	talkToServer("127.0.0.1:2525")

	d.Shutdown()

	b, err := ioutil.ReadFile("tests/testlog")
	if err != nil {
		t.Error("could not read logfile")
		return
	}
	// lets check for fingerprints
	if strings.Index(string(b), "another funky recipient") < 0 {
		t.Error("did not log: another funky recipient")
	}

	if strings.Index(string(b), "Another funky email!") < 0 {
		t.Error("Did not log: Another funky email!")
	}

	if strings.Index(string(b), "Funky logger is up & down to funk") < 0 {
		t.Error("Did not log: Funky logger is up & down to funk")
	}
	if strings.Index(string(b), "The funk has been stopped!") < 0 {
		t.Error("Did not log:The funk has been stopped!")
	}

}

func talkToServer(address string) {

	conn, err := net.Dial("tcp", address)
	if err != nil {

		return
	}
	in := bufio.NewReader(conn)
	str, err := in.ReadString('\n')
	fmt.Fprint(conn, "HELO maildiranasaurustester\r\n")
	str, err = in.ReadString('\n')
	fmt.Fprint(conn, "MAIL FROM:<test@example.com>r\r\n")
	str, err = in.ReadString('\n')
	fmt.Fprint(conn, "RCPT TO:test@grr.la\r\n")
	str, err = in.ReadString('\n')
	fmt.Fprint(conn, "DATA\r\n")
	str, err = in.ReadString('\n')
	fmt.Fprint(conn, "Subject: Test subject\r\n")
	fmt.Fprint(conn, "\r\n")
	fmt.Fprint(conn, "A an email body\r\n")
	fmt.Fprint(conn, ".\r\n")
	str, err = in.ReadString('\n')
	_ = str
}

// Test hot config reload
// Here we forgot to add FunkyLogger so backend will fail to init

func TestReloadConfig(t *testing.T) {
	os.Truncate("tests/testlog", 0)
	d := Daemon{}
	d.Start()
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
	d.ReloadConfig(cfg)

}

func TestPubSubAPI(t *testing.T) {

	os.Truncate("tests/testlog", 0)

	d := Daemon{Config: &AppConfig{LogFile: "tests/testlog"}}
	d.Start()
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
	d.Subscribe(EventConfigPidFile, pidEvHandler)

	d.ReloadConfig(cfg)

	d.Unsubscribe(EventConfigPidFile, pidEvHandler)
	cfg.PidFile = "tests/pidfile2.pid"
	d.Publish(EventConfigPidFile, &cfg)
	d.ReloadConfig(cfg)

	b, err := ioutil.ReadFile("tests/testlog")
	if err != nil {
		t.Error("could not read logfile")
		return
	}
	// lets interrogate the log
	if strings.Index(string(b), "number1") < 0 {
		t.Error("it lools like d.ReloadConfig(cfg) did not fire EventConfigPidFile, pidEvHandler not called")
	}

}

func TestAPILog(t *testing.T) {
	os.Truncate("tests/testlog", 0)
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
	if strings.Index(string(b), "logtest1") < 0 {
		t.Error("hai was not found in the log, it should have been in tests/testlog")
	}
}

// Test the allowed_hosts config option with a single entry of ".", which will allow all hosts.
func TestSkipAllowsHost(t *testing.T) {

	d := Daemon{}
	defer d.Shutdown()
	// setting the allowed hosts to a single entry with a dot will let any host through
	d.Config = &AppConfig{AllowedHosts: []string{"."}, LogFile: "off"}
	d.Start()

	conn, err := net.Dial("tcp", d.Config.Servers[0].ListenInterface)
	if err != nil {
		t.Error(t)
		return
	}
	in := bufio.NewReader(conn)
	fmt.Fprint(conn, "HELO test\r\n")
	fmt.Fprint(conn, "RCPT TO: test@funkyhost.com\r\n")
	in.ReadString('\n')
	in.ReadString('\n')
	str, _ := in.ReadString('\n')
	if strings.Index(str, "250") != 0 {
		t.Error("expected 250 reply, got:", str)
	}

}
