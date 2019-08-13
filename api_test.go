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
	if strings.Index(string(b), "re-opened log file") < 0 {
		t.Error("Server log did not re-opened, expecting \"re-opened log file\"")
	}
	if strings.Index(string(b), "re-opened main log file") < 0 {
		t.Error("Main log did not re-opened, expecting \"re-opened main log file\"")
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
	if strings.Index(string(b), "127.0.0.1:2527") < 0 {
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
	if err := talkToServer("127.0.0.1:2525", ""); err != nil {
		t.Error(err)
	}

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

func talkToServer(address string, body string) (err error) {

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
	if body == "" {
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
	} else {
		_, err = fmt.Fprint(conn, body)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(conn, ".\r\n")
		if err != nil {
			return err
		}
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
	if strings.Index(string(b), "number1") < 0 {
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
	if err := talkToServer("127.0.0.1:2525", ""); err != nil {
		t.Error(err)
	}

	d.Shutdown()

	b, err := ioutil.ReadFile("tests/testlog")
	if err != nil {
		t.Error("could not read logfile")
		return
	}
	// lets check for fingerprints
	if strings.Index(string(b), "451 4.3.0 Error") < 0 {
		t.Error("did not log: 451 4.3.0 Error")
	}

	if strings.Index(string(b), "system shock") < 0 {
		t.Error("did not log: system shock")
	}

}

func TestStreamProcessor(t *testing.T) {
	if err := os.Truncate("tests/testlog", 0); err != nil {
		t.Error(err)
	}
	cfg := &AppConfig{
		LogFile:      "tests/testlog",
		AllowedHosts: []string{"grr.la"},
		BackendConfig: backends.BackendConfig{
			"save_process":        "HeadersParser|Debugger",
			"stream_save_process": "Header|headersparser|compress|Decompress|debug",
		},
	}
	d := Daemon{Config: cfg}

	if err := d.Start(); err != nil {
		t.Error(err)
	}
	body := "Subject: Test subject\r\n" +
		//"\r\n" +
		"A an email body.\r\n" +
		"Header|headersparser|compress|Decompress|debug Header|headersparser|compress|Decompress|debug.\r\n" +
		"Header|headersparser|compress|Decompress|debug Header|headersparser|compress|Decompress|debug.\r\n" +
		"Header|headersparser|compress|Decompress|debug Header|headersparser|compress|Decompress|debug.\r\n" +
		"Header|headersparser|compress|Decompress|debug Header|headersparser|compress|Decompress|debug.\r\n" +
		"Header|headersparser|compress|Decompress|debug Header|headersparser|compress|Decompress|debug.\r\n" +
		"Header|headersparser|compress|Decompress|debug Header|headersparser|compress|Decompress|debug.\r\n" +
		"Header|headersparser|compress|Decompress|debug Header|headersparser|compress|Decompress|debug.\r\n" +
		"Header|headersparser|compress|Decompress|debug Header|headersparser|compress|Decompress|debug.\r\n" +
		"Header|headersparser|compress|Decompress|debug Header|headersparser|compress|Decompress|debug.\r\n" +
		"Header|headersparser|compress|Decompress|debug Header|headersparser|compress|Decompress|debug.\r\n" +
		"Header|headersparser|compress|Decompress|debug Header|headersparser|compress|Decompress|debug.\r\n" +
		"Header|headersparser|compress|Decompress|debug Header|headersparser|compress|Decompress|debug.\r\n"

	// lets have a talk with the server
	if err := talkToServer("127.0.0.1:2525", body); err != nil {
		t.Error(err)
	}

	d.Shutdown()

	b, err := ioutil.ReadFile("tests/testlog")
	if err != nil {
		t.Error("could not read logfile")
		return
	}

	// lets check for fingerprints
	if strings.Index(string(b), "Debug stream") < 0 {
		t.Error("did not log: Debug stream")
	}

	if strings.Index(string(b), "Error") != -1 {
		t.Error("There was an error", string(b))
	}

}

var mime0 = `MIME-Version: 1.0
X-Mailer: MailBee.NET 8.0.4.428
Subject: test 
 subject
To: kevinm@datamotion.com
Content-Type: multipart/mixed;
       boundary="XXXXboundary text"

--XXXXboundary text
Content-Type: multipart/alternative;
       boundary="XXXXboundary text"

--XXXXboundary text
Content-Type: text/plain;
       charset="utf-8"
Content-Transfer-Encoding: quoted-printable

This is the body text of a sample message.
--XXXXboundary text
Content-Type: text/html;
       charset="utf-8"
Content-Transfer-Encoding: quoted-printable

<pre>This is the body text of a sample message.</pre>

--XXXXboundary text
Content-Type: text/plain;
       name="log_attachment.txt"
Content-Disposition: attachment;
       filename="log_attachment.txt"
Content-Transfer-Encoding: base64

TUlNRS1WZXJzaW9uOiAxLjANClgtTWFpbGVyOiBNYWlsQmVlLk5FVCA4LjAuNC40MjgNClN1Ympl
Y3Q6IHRlc3Qgc3ViamVjdA0KVG86IGtldmlubUBkYXRhbW90aW9uLmNvbQ0KQ29udGVudC1UeXBl
OiBtdWx0aXBhcnQvYWx0ZXJuYXRpdmU7DQoJYm91bmRhcnk9Ii0tLS09X05leHRQYXJ0XzAwMF9B
RTZCXzcyNUUwOUFGLjg4QjdGOTM0Ig0KDQoNCi0tLS0tLT1fTmV4dFBhcnRfMDAwX0FFNkJfNzI1
RTA5QUYuODhCN0Y5MzQNCkNvbnRlbnQtVHlwZTogdGV4dC9wbGFpbjsNCgljaGFyc2V0PSJ1dGYt
OCINCkNvbnRlbnQtVHJhbnNmZXItRW5jb2Rpbmc6IHF1b3RlZC1wcmludGFibGUNCg0KdGVzdCBi
b2R5DQotLS0tLS09X05leHRQYXJ0XzAwMF9BRTZCXzcyNUUwOUFGLjg4QjdGOTM0DQpDb250ZW50
LVR5cGU6IHRleHQvaHRtbDsNCgljaGFyc2V0PSJ1dGYtOCINCkNvbnRlbnQtVHJhbnNmZXItRW5j
b2Rpbmc6IHF1b3RlZC1wcmludGFibGUNCg0KPHByZT50ZXN0IGJvZHk8L3ByZT4NCi0tLS0tLT1f
TmV4dFBhcnRfMDAwX0FFNkJfNzI1RTA5QUYuODhCN0Y5MzQtLQ0K
--XXXXboundary text--
`

var mime2 = `From: abc@def.de
Content-Type: multipart/mixed;
        boundary="----_=_NextPart_001_01CBE273.65A0E7AA"
To: ghi@def.de

This is a multi-part message in MIME format.

------_=_NextPart_001_01CBE273.65A0E7AA
Content-Type: multipart/alternative;
        boundary="----_=_NextPart_002_01CBE273.65A0E7AA"


------_=_NextPart_002_01CBE273.65A0E7AA
Content-Type: text/plain;
        charset="UTF-8"
Content-Transfer-Encoding: base64

[base64-content]
------_=_NextPart_002_01CBE273.65A0E7AA
Content-Type: text/html;
        charset="UTF-8"
Content-Transfer-Encoding: base64

[base64-content]
------_=_NextPart_002_01CBE273.65A0E7AA--
------_=_NextPart_001_01CBE273.65A0E7AA
Content-Type: message/rfc822
Content-Transfer-Encoding: 7bit

X-MimeOLE: Produced By Microsoft Exchange V6.5
Content-class: urn:content-classes:message
MIME-Version: 1.0
Content-Type: multipart/mixed;
        boundary="----_=_NextPart_003_01CBE272.13692C80"
From: bla@bla.de
To: xxx@xxx.de

This is a multi-part message in MIME format.

------_=_NextPart_003_01CBE272.13692C80
Content-Type: multipart/alternative;
        boundary="----_=_NextPart_004_01CBE272.13692C80"


------_=_NextPart_004_01CBE272.13692C80
Content-Type: text/plain;
        charset="iso-8859-1"
Content-Transfer-Encoding: quoted-printable

=20

Viele Gr=FC=DFe

------_=_NextPart_004_01CBE272.13692C80
Content-Type: text/html;
        charset="iso-8859-1"
Content-Transfer-Encoding: quoted-printable

<html>...</html>
------_=_NextPart_004_01CBE272.13692C80--
------_=_NextPart_003_01CBE272.13692C80
Content-Type: application/x-zip-compressed;
        name="abc.zip"
Content-Transfer-Encoding: base64
Content-Disposition: attachment;
        filename="abc.zip"

[base64-content]

------_=_NextPart_003_01CBE272.13692C80--
------_=_NextPart_001_01CBE273.65A0E7AA--
`

var mime3 = `From lara_lars@hotmail.com Mon Feb 19 22:24:21 2001
Received: from [137.154.210.66] by hotmail.com (3.2) with ESMTP id MHotMailBC5B58230039400431D5899AD24289FA0; Mon Feb 19 22:22:29 2001
Received: from lancelot.cit.nepean.uws.edu.au (lancelot.cit.uws.edu.au [137.154.148.30])
        by day.uws.edu.au (8.11.1/8.11.1) with ESMTP id f1K6MN404936;
        Tue, 20 Feb 2001 17:22:24 +1100 (EST)
Received: from hotmail.com (law2-f35.hotmail.com [216.32.181.35])
        by lancelot.cit.nepean.uws.edu.au (8.10.0.Beta10/8.10.0.Beta10) with ESMTP id f1K6MJb13619;
        Tue, 20 Feb 2001 17:22:19 +1100 (EST)
Received: from mail pickup service by hotmail.com with Microsoft SMTPSVC;
         Mon, 19 Feb 2001 22:21:44 -0800
Received: from 203.54.221.89 by lw2fd.hotmail.msn.com with HTTP;        Tue, 20 Feb 2001 06:21:44 GMT
X-Originating-IP: [203.54.221.89]
From: "lara devine" <lara_lars@hotmail.com>
To: amalinow@cit.nepean.uws.edu.au, transmission_@hotmail.com,
   lalexand@cit.nepean.uws.edu.au, dconroy@cit.nepean.uws.edu.au,
   pumpkin7@bigpond.com, jwalker@cit.nepean.uws.edu.au,
   dgoerge@cit.nepean.uws.edu.au, batty_horny@hotmail.com,
   ikvesic@start.com.au
Subject: Fwd: Goldfish
Date: Tue, 20 Feb 2001 06:21:44
Mime-Version: 1.0
Content-Type: text/plain; format=flowed
Message-ID: <LAW2-F35R881Np7vXee00000222@hotmail.com>
X-OriginalArrivalTime: 20 Feb 2001 06:21:44.0718 (UTC) FILETIME=[658BDAE0:01C09B05]




>> >Two builders (Chris and James) are seated either side of a table in a
> > >rough
> > >pub when a well-dressed man enters, orders beer and sits on a stool at
> > >the bar.
> > >The two builders start to speculate about the occupation of the suit.
> > >
> > >Chris: - I reckon he's an accountant.
> > >
> > >James: - No way - he's a stockbroker.
> > >
> > >Chris: - He ain't no stockbroker! A stockbroker wouldn't come in here!
> > >
> > >The argument repeats itself for some time until the volume of beer gets
> > >the better of Chris and he makes for the toilet. On entering the toilet
> > >he
> > >sees that the suit is standing at a urinal. Curiosity and the several
> > >beers
> > >get the better of the builder...
> > >
> > >Chris: - 'scuse me.... no offence meant, but me and me mate were
> > wondering
> > >
> > >  what you do for a living?
> > >
> > >Suit: - No offence taken! I'm a Logical Scientist by profession!
> > >
> > >Chris: - Oh! What's that then?
> > >
> > >Suit:- I'll try to explain by example... Do you have a goldfish at
>home?
> > >
> > >Chris:- Er...mmm... well yeah, I do as it happens!
> > >
> > >Suit: - Well, it's logical to follow that you keep it in a bowl or in a
> > >pond. Which is it?
> > >
> > >Chris: - It's in a pond!
> > >
> > >Suit: - Well then it's reasonable to suppose that you have a large
> > >garden
> > >then?
> > >
> > >Chris: - As it happens, yes I have got a big garden!
> > >
> > >Suit: - Well then it's logical to assume that in this town that if you
> > >have a large garden that you have a large house?
> > >
> > >Chris: - As it happens I've got a five bedroom house... built it
>myself!
> > >
> > >Suit: - Well given that you've built a five-bedroom house it is logical
> > >to asume that you haven't built it just for yourself and that you are
> > >quite
> > >probably married?
> > >
> > >Chris: - Yes I am married, I live with my wife and three children!
> > >
> > >Suit: - Well then it is logical to assume that you are sexually active
> > >with your wife on a regular basis?
> > >
> > >Chris:- Yep! Four nights a week!
> > >
> > >Suit: - Well then it is logical to suggest that you do not masturbate
> > >very often?
> > >
> > >Chris: - Me? Never.
> > >
> > >Suit: - Well there you are! That's logical science at work!
> > >
> > >Chris:- How's that then?
> > >
> > >Suit: - Well from finding out that you had a goldfish, I've told you
> > >about the size of garden you have, size of house, your family and your
> > >sex
> > >life!
> > >
> > >Chris: - I see! That's pretty impressive... thanks mate!
> > >
> > >Both leave the toilet and Chris returns to his mate.
> > >
> > >James: - I see the suit was in there. Did you ask him what he does?
> > >
> > >Chris: - Yep! He's a logical scientist!
> > >
> > >James: What's a logical Scientist?
> > >
> > >Chris: - I'll try and explain. Do you have a goldfish?
> > >
> > >James: - Nope.
> > >
> > >Chris: - Well then, you're a wanker.
>

_________________________________________________________________________
Get Your Private, Free E-mail from MSN Hotmail at http://www.hotmail.com.
`

/*
1  0  166  1514
1.1  186  260  259
1.2  280  374  416
1.3  437  530  584
1.4  605  769  1514
*/
func TestStreamMimeProcessor(t *testing.T) {
	if err := os.Truncate("tests/testlog", 0); err != nil {
		t.Error(err)
	}
	cfg := &AppConfig{
		LogFile:      "tests/testlog",
		AllowedHosts: []string{"grr.la"},
		BackendConfig: backends.BackendConfig{
			"save_process":        "HeadersParser|Debugger",
			"stream_save_process": "mimeanalyzer|headersparser|compress|Decompress|debug",
		},
	}
	d := Daemon{Config: cfg}

	if err := d.Start(); err != nil {
		t.Error(err)
	}

	go func() {
		time.Sleep(time.Second * 15)
		//panic("here")
		//		*moo = *moo + 6

		// for debugging deadlocks
		//pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		//os.Exit(1)
	}()

	// change \n to \r\n
	mime0 = strings.Replace(mime2, "\n", "\r\n", -1)
	// lets have a talk with the server
	if err := talkToServer("127.0.0.1:2525", mime0); err != nil {
		t.Error(err)
	}

	d.Shutdown()

	b, err := ioutil.ReadFile("tests/testlog")
	if err != nil {
		t.Error("could not read logfile")
		return
	}

	// lets check for fingerprints
	if strings.Index(string(b), "Debug stream") < 0 {
		t.Error("did not log: Debug stream")
	}

	if strings.Index(string(b), "Error") != -1 {
		t.Error("There was an error", string(b))
	}

}
