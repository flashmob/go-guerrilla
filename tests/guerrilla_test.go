// integration / smokeless
// =======================
// Tests are in a different package so we can test as a consumer of the guerrilla package
// The following are integration / smokeless, that test the overall server.
// (Please put unit tests to go in a different file)
// How it works:
// Server's log output is redirected to the logBuffer which is then used by the tests to look for expected behaviour
// the package sets up the logBuffer & redirection by the use of package init()
// (self signed certs are also generated on each run)
// server's responses from a connection are also used to check for expected behaviour
// to run:
// $ go test

package test

import (
	"encoding/json"
	log "github.com/Sirupsen/logrus"
	"testing"

	"github.com/flashmob/go-guerrilla"
	"github.com/flashmob/go-guerrilla/backends"
	"time"

	"bufio"

	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strings"
)

type TestConfig struct {
	guerrilla.AppConfig
	BackendName   string                 `json:"backend_name"`
	BackendConfig map[string]interface{} `json:"backend_config"`
}

var (
	// hold the output of logs
	logBuffer bytes.Buffer
	// logs redirected to this writer
	logOut *bufio.Writer
	// read the logs
	logIn *bufio.Reader
	// app config loaded here
	config *TestConfig

	app guerrilla.Guerrilla

	initErr error
)

func init() {
	logOut = bufio.NewWriter(&logBuffer)
	logIn = bufio.NewReader(&logBuffer)
	log.SetLevel(log.DebugLevel)
	log.SetOutput(logOut)
	config = &TestConfig{}
	if err := json.Unmarshal([]byte(configJson), config); err != nil {
		initErr = errors.New("Could not unmartial config," + err.Error())
	} else {
		setupCerts(config)
		backend := getDummyBackend(config.BackendConfig)
		app = guerrilla.New(&config.AppConfig, &backend)
	}

}

// a configuration file with a dummy backend
var configJson = `
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
            "private_key_file":"/vagrant/projects/htdocs/guerrilla/config/ssl/guerrillamail.com.key",
            "public_key_file":"/vagrant/projects/htdocs/guerrilla/config/ssl/guerrillamail.com.crt",
            "timeout":160,
            "listen_interface":"127.0.0.1:2526",
            "start_tls_on":true,
            "tls_always_on":false,
            "max_clients": 2,
            "log_file":"/dev/stdout"
        },

        {
            "is_enabled" : true,
            "host_name":"mail.guerrillamail.com",
            "max_size":1000001,
            "private_key_file":"/vagrant/projects/htdocs/guerrilla/config/ssl/guerrillamail.com.key",
            "public_key_file":"/vagrant/projects/htdocs/guerrilla/config/ssl/guerrillamail.com.crt",
            "timeout":180,
            "listen_interface":"127.0.0.1:4654",
            "start_tls_on":false,
            "tls_always_on":true,
            "max_clients":1,
            "log_file":"/dev/stdout"
        }
    ]
}
`

func getDummyBackend(backendConfig map[string]interface{}) guerrilla.Backend {
	var backend guerrilla.Backend
	b := &backends.DummyBackend{}
	b.Initialize(backendConfig)
	backend = guerrilla.Backend(b)
	return backend
}

func setupCerts(c *TestConfig) {
	for i := range c.Servers {
		generateCert(c.Servers[i].Hostname, "", 365*24*time.Hour, false, 2048, "P256")
		c.Servers[i].PrivateKeyFile = c.Servers[i].Hostname + ".key.pem"
		c.Servers[i].PublicKeyFile = c.Servers[i].Hostname + ".cert.pem"
	}
}

// Testing start and stop of server
func TestStart(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	if startErrors := app.Start(); startErrors != nil {
		for _, err := range startErrors {
			t.Error(err)
		}
		t.FailNow()
	}
	time.Sleep(time.Second)
	app.Shutdown()
	logOut.Flush()
	if read, err := ioutil.ReadAll(logIn); err == nil {
		logOutput := string(read)
		if i := strings.Index(logOutput, "Listening on TCP 127.0.0.1:4654"); i < 0 {
			t.Error("Server did not listen on 127.0.0.1:4654")
		}
		if i := strings.Index(logOutput, "Listening on TCP 127.0.0.1:2526"); i < 0 {
			t.Error("Server did not listen on 127.0.0.1:2526")
		}
		if i := strings.Index(logOutput, "[127.0.0.1:4654] Waiting for a new client"); i < 0 {
			t.Error("Server did not wait on 127.0.0.1:4654")
		}
		if i := strings.Index(logOutput, "[127.0.0.1:2526] Waiting for a new client"); i < 0 {
			t.Error("Server did not wait on 127.0.0.1:2526")
		}
		if i := strings.Index(logOutput, "Server [127.0.0.1:4654] has stopped accepting new clients"); i < 0 {
			t.Error("Server did not stop on 127.0.0.1:4654")
		}
		if i := strings.Index(logOutput, "Server [127.0.0.1:2526] has stopped accepting new clients"); i < 0 {
			t.Error("Server did not stop on 127.0.0.1:2526")
		}
		if i := strings.Index(logOutput, "shutdown completed for [127.0.0.1:4654]"); i < 0 {
			t.Error("Server did not complete shutdown on 127.0.0.1:4654")
		}
		if i := strings.Index(logOutput, "shutdown completed for [127.0.0.1:2526]"); i < 0 {
			t.Error("Server did not complete shutdown on 127.0.0.1:2526")
		}
		if i := strings.Index(logOutput, "shutting down pool [127.0.0.1:4654]"); i < 0 {
			t.Error("Server did not shutdown pool on 127.0.0.1:4654")
		}
		if i := strings.Index(logOutput, "shutting down pool [127.0.0.1:2526]"); i < 0 {
			t.Error("Server did not shutdown pool on 127.0.0.1:2526")
		}
		if i := strings.Index(logOutput, "Backend shutdown completed"); i < 0 {
			t.Error("Backend didn't shut down")
		}

	}
	logBuffer.Reset()
	logIn.Reset(&logBuffer)

}

// Simple smoke-test to see if the server can listen & issues a greeting on connect
func TestGreeting(t *testing.T) {
	//log.SetOutput(os.Stdout)
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	if startErrors := app.Start(); startErrors == nil {

		// 1. plaintext connection
		conn, err := net.Dial("tcp", config.Servers[0].ListenInterface)
		if err != nil {
			// handle error
			t.Error("Cannot dial server", config.Servers[0].ListenInterface)
		}
		conn.SetReadDeadline(time.Now().Add(time.Duration(time.Millisecond * 500)))
		greeting, err := bufio.NewReader(conn).ReadString('\n')
		//fmt.Println(greeting)
		if err != nil {
			t.Error(err)
			t.FailNow()
		} else {
			expected := "220 mail.guerrillamail.com SMTP Guerrilla"
			if strings.Index(greeting, expected) != 0 {
				t.Error("Server[1] did not have the expected greeting prefix", expected)
			}
		}
		conn.Close()

		// 2. tls connection
		roots, err := x509.SystemCertPool()
		conn, err = tls.Dial("tcp", config.Servers[1].ListenInterface, &tls.Config{
			RootCAs:            roots,
			InsecureSkipVerify: true,
			ServerName:         "127.0.0.1",
		})
		if err != nil {
			// handle error
			t.Error(err, "Cannot dial server (TLS)", config.Servers[1].ListenInterface)
			t.FailNow()
		}
		conn.SetReadDeadline(time.Now().Add(time.Duration(time.Millisecond * 500)))
		greeting, err = bufio.NewReader(conn).ReadString('\n')
		//fmt.Println(greeting)
		if err != nil {
			t.Error(err)
			t.FailNow()
		} else {
			expected := "220 mail.guerrillamail.com SMTP Guerrilla"
			if strings.Index(greeting, expected) != 0 {
				t.Error("Server[2] (TLS) did not have the expected greeting prefix", expected)
			}
		}
		conn.Close()

	} else {
		if startErrors := app.Start(); startErrors != nil {
			for _, err := range startErrors {
				t.Error(err)
			}
			t.FailNow()
		}
	}
	app.Shutdown()
	logOut.Flush()
	if read, err := ioutil.ReadAll(logIn); err == nil {
		logOutput := string(read)
		//fmt.Println(logOutput)
		if i := strings.Index(logOutput, "Handle client [127.0.0.1:"); i < 0 {
			t.Error("Server did not handle any clients")
		}
	}
	logBuffer.Reset()
	logIn.Reset(&logBuffer)

}

// start up a server, connect a client, greet, then shutdown, then client sends a command
// expecting: 421 Server is shutting down. Please try again later. Sayonara!
// server should close connection after that
func TestShutDown(t *testing.T) {

	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	if startErrors := app.Start(); startErrors == nil {
		conn, err := net.Dial("tcp", config.Servers[0].ListenInterface)
		if err != nil {
			// handle error
			t.Error("Cannot dial server", config.Servers[0].ListenInterface)
		}
		bufin := bufio.NewReader(conn)

		// should be ample time to complete the test
		conn.SetDeadline(time.Now().Add(time.Duration(time.Second * 20)))
		// read greeting, ignore it
		_, err = bufin.ReadString('\n')

		if err != nil {
			t.Error(err)
			t.FailNow()
		} else {
			// client goes into command state
			n, err := fmt.Fprintln(conn, "HELO localtester\r")
			if err != nil {
				log.WithError(err).Info("n was %d", n)
			}
			_, err = bufin.ReadString('\n')

			// do a shutdown while the client is connected & in client state
			go app.Shutdown()

			// issue a command while shutting down
			n, err = fmt.Fprintln(conn, "HELP\r")
			if err != nil {
				log.WithError(err).Info("n was %d", n)
			}
			response, err := bufin.ReadString('\n')
			//fmt.Println(response)
			expected := "421 Server is shutting down. Please try again later. Sayonara!"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not shut down with", expected)
			}
			time.Sleep(time.Millisecond * 250) // let server to close

		}

		conn.Close()

	} else {
		if startErrors := app.Start(); startErrors != nil {
			for _, err := range startErrors {
				t.Error(err)
			}
			app.Shutdown()
			t.FailNow()
		}
	}
	logOut.Flush()
	if read, err := ioutil.ReadAll(logIn); err == nil {
		logOutput := string(read)
		//	fmt.Println(logOutput)
		if i := strings.Index(logOutput, "Handle client [127.0.0.1:"); i < 0 {
			t.Error("Server did not handle any clients")
		}
	}
	logBuffer.Reset()
	logIn.Reset(&logBuffer)

}
