package guerrilla

import (
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"io/ioutil"
	"testing"
	"time"
)

// Test Starting smtp without setting up logger / backend
func TestSMTP(t *testing.T) {

	smtp := SMTP{}
	err := smtp.Start()

	if err != nil {
		t.Error(err)
	}
	// it should set to stderr automatically
	if smtp.config.LogFile != log.OutputStderr.String() {
		t.Error("smtp.config.LogFile is not", log.OutputStderr.String())
	}

	if len(smtp.config.AllowedHosts) == 0 {
		t.Error("smtp.config.AllowedHosts len should be 1, not 0", smtp.config.AllowedHosts)
	}

	if smtp.config.LogLevel != "debug" {
		t.Error("smtp.config.LogLevel expected'debug', it is", smtp.config.LogLevel)
	}
	if len(smtp.config.Servers) != 1 {
		t.Error("len(smtp.config.Servers) should be 1, got", len(smtp.config.Servers))
	}
	time.Sleep(time.Second * 2)
	smtp.Shutdown()

}

// Suppressing log output
func TestSMTPNoLog(t *testing.T) {

	// configure a default server with no log output
	cfg := &AppConfig{LogFile: log.OutputOff.String()}
	smtp := SMTP{config: cfg}

	err := smtp.Start()
	if err != nil {
		t.Error(err)
	}
	time.Sleep(time.Second * 2)
	smtp.Shutdown()
}

// our custom server
func TestSMTPCustomServer(t *testing.T) {
	cfg := &AppConfig{LogFile: log.OutputStdout.String()}
	sc := ServerConfig{
		ListenInterface: "127.0.0.1:2526",
		IsEnabled:       true,
	}
	cfg.Servers = append(cfg.Servers, sc)
	smtp := SMTP{config: cfg}

	err := smtp.Start()
	if err != nil {
		t.Error("start error", err)
	} else {
		time.Sleep(time.Second * 2)
		smtp.Shutdown()
	}

}

// with a backend config
func TestSMTPCustomBackend(t *testing.T) {
	cfg := &AppConfig{LogFile: log.OutputStdout.String()}
	sc := ServerConfig{
		ListenInterface: "127.0.0.1:2526",
		IsEnabled:       true,
	}
	cfg.Servers = append(cfg.Servers, sc)
	bcfg := backends.BackendConfig{
		"save_workers_size":  3,
		"process_stack":      "HeadersParser|Header|Hasher|Debugger",
		"log_received_mails": true,
	}
	cfg.BackendConfig = bcfg
	smtp := SMTP{config: cfg}

	err := smtp.Start()
	if err != nil {
		t.Error("start error", err)
	} else {
		time.Sleep(time.Second * 2)
		smtp.Shutdown()
	}
}

// with a config from a json file
func TestSMTPLoadFile(t *testing.T) {
	json := `{
    "log_file" : "./tests/testlog",
    "log_level" : "debug",
    "pid_file" : "/var/run/go-guerrilla.pid",
    "allowed_hosts": ["spam4.me","grr.la"],
    "backend_config" :
        {
            "log_received_mails" : true,
            "process_stack": "HeadersParser|Header|Hasher|Debugger",
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

	/*

		cfg := &AppConfig{LogFile: log.OutputStdout.String()}
		sc := ServerConfig{
			ListenInterface: "127.0.0.1:2526",
			IsEnabled:       true,
		}
		cfg.Servers = append(cfg.Servers, sc)
		bcfg := backends.BackendConfig{
			"save_workers_size":  3,
			"process_stack":      "HeadersParser|Header|Hasher|Debugger",
			"log_received_mails": true,
		}
		cfg.BackendConfig = bcfg

		smtp := SMTP{config: cfg}
	*/
	smtp := SMTP{}
	err = smtp.ReadConfig("goguerrilla.conf.api")
	if err != nil {
		t.Error("ReadConfig error", err)
		return
	}

	err = smtp.Start()
	if err != nil {
		t.Error("start error", err)
		return
	} else {
		time.Sleep(time.Second * 2)
		smtp.Shutdown()
	}
}
