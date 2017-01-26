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
	"testing"

	log "github.com/Sirupsen/logrus"

	"time"

	"github.com/flashmob/go-guerrilla"
	"github.com/flashmob/go-guerrilla/backends"

	"bufio"

	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strings"

	"github.com/flashmob/go-guerrilla/tests/testcert"
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
		backend, _ := getBackend("dummy", config.BackendConfig)
		app, _ = guerrilla.New(&config.AppConfig, backend)
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

func getBackend(backendName string, backendConfig map[string]interface{}) (backends.Backend, error) {
	return backends.New(backendName, backendConfig)
}

func setupCerts(c *TestConfig) {
	for i := range c.Servers {
		testcert.GenerateCert(c.Servers[i].Hostname, "", 365*24*time.Hour, false, 2048, "P256", "./")
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
		t.Error(startErrors)
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
	// don't forget to reset
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
		//	roots, err := x509.SystemCertPool()
		conn, err = tls.Dial("tcp", config.Servers[1].ListenInterface, &tls.Config{

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
			t.Error(startErrors)
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
	// don't forget to reset
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
		conn, bufin, err := Connect(config.Servers[0], 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := Command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}

			// do a shutdown while the client is connected & in client state
			go app.Shutdown()
			time.Sleep(time.Millisecond * 150) // let server to Shutdown

			// issue a command while shutting down
			response, err := Command(conn, bufin, "HELP")
			if err != nil {
				t.Error("Help command failed", err.Error())
			}
			expected := "421 4.3.0 Server is shutting down. Please try again later. Sayonara!"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not shut down with", expected, ", it said:"+response)
			}
			time.Sleep(time.Millisecond * 250) // let server to close
		}

		conn.Close()

	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}
	// assuming server has shutdown by now
	logOut.Flush()
	if read, err := ioutil.ReadAll(logIn); err == nil {
		logOutput := string(read)
		//	fmt.Println(logOutput)
		if i := strings.Index(logOutput, "Handle client [127.0.0.1:"); i < 0 {
			t.Error("Server did not handle any clients")
		}
	}
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)

}

// add more than 100 recipients, it should fail at 101
func TestRFC2821LimitRecipients(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	if startErrors := app.Start(); startErrors == nil {
		conn, bufin, err := Connect(config.Servers[0], 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := Command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}

			for i := 0; i < 101; i++ {
				if _, err := Command(conn, bufin, fmt.Sprintf("RCPT TO:test%d@grr.la", i)); err != nil {
					t.Error("RCPT TO", err.Error())
					break
				}
			}
			response, err := Command(conn, bufin, "RCPT TO:last@grr.la")
			if err != nil {
				t.Error("rcpt command failed", err.Error())
			}
			expected := "452 4.5.3 Too many recipients"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
		}

		conn.Close()
		app.Shutdown()

	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}

	logOut.Flush()

	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)
}

// RCPT TO & MAIL FROM with 64 chars in local part, it should fail at 65
func TestRFC2832LimitLocalPart(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}

	if startErrors := app.Start(); startErrors == nil {
		conn, bufin, err := Connect(config.Servers[0], 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := Command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}
			// repeat > 64 characters in local part
			response, err := Command(conn, bufin, fmt.Sprintf("RCPT TO:%s@grr.la", strings.Repeat("a", 65)))
			if err != nil {
				t.Error("rcpt command failed", err.Error())
			}
			expected := "550 5.5.4 Local part too long"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
			// what about if it's exactly 64?
			// repeat > 64 characters in local part
			response, err = Command(conn, bufin, fmt.Sprintf("RCPT TO:%s@grr.la", strings.Repeat("a", 64)))
			if err != nil {
				t.Error("rcpt command failed", err.Error())
			}
			expected = "250 2.1.5 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
		}

		conn.Close()
		app.Shutdown()

	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}

	logOut.Flush()

	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)
}

//RFC2821LimitPath fail if path > 256 but different error if below

func TestRFC2821LimitPath(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	if startErrors := app.Start(); startErrors == nil {
		conn, bufin, err := Connect(config.Servers[0], 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := Command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}
			// repeat > 256 characters in local part
			response, err := Command(conn, bufin, fmt.Sprintf("RCPT TO:%s@grr.la", strings.Repeat("a", 257-7)))
			if err != nil {
				t.Error("rcpt command failed", err.Error())
			}
			expected := "550 5.5.4 Path too long"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
			// what about if it's exactly 256?
			response, err = Command(conn, bufin,
				fmt.Sprintf("RCPT TO:%s@%s.la", strings.Repeat("a", 64), strings.Repeat("b", 257-5-64)))
			if err != nil {
				t.Error("rcpt command failed", err.Error())
			}
			expected = "454 4.1.1 Error: Relay access denied"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
		}
		conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}
	logOut.Flush()
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)
}

// RFC2821LimitDomain 501 Domain cannot exceed 255 characters
func TestRFC2821LimitDomain(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	if startErrors := app.Start(); startErrors == nil {
		conn, bufin, err := Connect(config.Servers[0], 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := Command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}
			// repeat > 64 characters in local part
			response, err := Command(conn, bufin, fmt.Sprintf("RCPT TO:a@%s.l", strings.Repeat("a", 255-2)))
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected := "550 5.5.4 Path too long"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
			// what about if it's exactly 255?
			response, err = Command(conn, bufin,
				fmt.Sprintf("RCPT TO:a@%s.la", strings.Repeat("b", 255-4)))
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "454 4.1.1 Error: Relay access denied"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
		}
		conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}
	logOut.Flush()
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)
}

// Test several different inputs to MAIL FROM command
func TestMailFromCmd(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	if startErrors := app.Start(); startErrors == nil {
		conn, bufin, err := Connect(config.Servers[0], 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := Command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}
			// Basic valid address
			response, err := Command(conn, bufin, "MAIL FROM:test@grr.la")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected := "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			// Reset
			response, err = Command(conn, bufin, "RSET")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			// Basic valid address (RfC)
			response, err = Command(conn, bufin, "MAIL FROM:<test@grr.la>")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			// Reset
			response, err = Command(conn, bufin, "RSET")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			// Bounce
			response, err = Command(conn, bufin, "MAIL FROM:<>")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			// Reset
			response, err = Command(conn, bufin, "RSET")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			// What?
			response, err = Command(conn, bufin, "MAIL FROM:<<>>")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "501 5.5.4 Invalid address"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			// Reset
			response, err = Command(conn, bufin, "RSET")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			// Invalid address?
			response, err = Command(conn, bufin, "MAIL FROM:<justatest>")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "501 5.5.4 Invalid address"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			// Reset
			response, err = Command(conn, bufin, "RSET")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			// SMTPUTF8 not implemented for now, currently still accepted
			response, err = Command(conn, bufin, "MAIL FROM:<anöthertest@grr.la>")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			// Reset
			response, err = Command(conn, bufin, "RSET")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			// 8BITMIME (RfC 6152)
			response, err = Command(conn, bufin, "MAIL FROM:<test@grr.la> BODY=8BITMIME")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			// Reset
			response, err = Command(conn, bufin, "RSET")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			// 8BITMIME (RfC 6152) Bounce
			response, err = Command(conn, bufin, "MAIL FROM:<> BODY=8BITMIME")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			// Reset
			response, err = Command(conn, bufin, "RSET")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

		}
		conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}
	logOut.Flush()
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)
}

// It should error when MAIL FROM was given twice
func TestNestedMailCmd(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	if startErrors := app.Start(); startErrors == nil {
		conn, bufin, err := Connect(config.Servers[0], 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := Command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}
			// repeat > 64 characters in local part
			response, err := Command(conn, bufin, "MAIL FROM:test@grr.la")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			response, err = Command(conn, bufin, "MAIL FROM:test@grr.la")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected := "503 5.5.1 Error: nested MAIL command"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
			// Plot twist: if you EHLO , it should allow MAIL FROM again
			if _, err := Command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}
			response, err = Command(conn, bufin, "MAIL FROM:test@grr.la")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
			// Plot twist: if you RSET , it should allow MAIL FROM again
			response, err = Command(conn, bufin, "RSET")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			response, err = Command(conn, bufin, "MAIL FROM:test@grr.la")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

		}
		conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}
	logOut.Flush()
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)
}

// It should error on a very long command line, exceeding CommandLineMaxLength 1024
func TestCommandLineMaxLength(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}

	if startErrors := app.Start(); startErrors == nil {
		conn, bufin, err := Connect(config.Servers[0], 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := Command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}
			// repeat > 1024 characters
			response, err := Command(conn, bufin, strings.Repeat("s", guerrilla.CommandLineMaxLength+1))
			if err != nil {
				t.Error("command failed", err.Error())
			}

			expected := "554 5.5.1 Line too long"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

		}
		conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}
	logOut.Flush()
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)
}

// It should error on a very long message, exceeding servers config value
func TestDataMaxLength(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}

	if startErrors := app.Start(); startErrors == nil {
		conn, bufin, err := Connect(config.Servers[0], 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := Command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}

			response, err := Command(conn, bufin, "MAIL FROM:test@grr.la")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			//fmt.Println(response)
			response, err = Command(conn, bufin, "RCPT TO:test@grr.la")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			//fmt.Println(response)
			response, err = Command(conn, bufin, "DATA")
			if err != nil {
				t.Error("command failed", err.Error())
			}

			response, err = Command(
				conn,
				bufin,
				fmt.Sprintf("Subject:test\r\n\r\nHello %s\r\n.\r\n",
					strings.Repeat("n", int(config.Servers[0].MaxSize-20))))

			//expected := "500 Line too long"
			expected := "451 4.3.0 Error: Maximum DATA size exceeded"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response, err)
			}

		}
		conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}
	logOut.Flush()
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)
}

func TestDataCommand(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}

	testHeader :=
		"Subject: =?Shift_JIS?B?W4NYg06DRYNGg0GBRYNHg2qDYoNOg1ggg0GDSoNFg5ODZ12DQYNKg0WDk4Nn?=\r\n" +
			"\t=?Shift_JIS?B?k2+YXoqul7mCzIKokm2C54K5?=\r\n"

	email :=
		"Delivered-To: test@sharklasers.com\r\n" +
			"\tReceived: from mail.guerrillamail.com (mail.guerrillamail.com  [104.218.55.28:44246])\r\n" +
			"\tby grr.la with SMTP id 2ab4220fdd6a7b877ae81241cd5a406a@grr.la;\r\n" +
			"\tWed, 18 Jan 2017 15:43:29 +0000\r\n" +
			"Received: by 192.99.19.220 with HTTP; Wed, 18 Jan 2017 15:43:29 +0000\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Message-ID: <230b4719d1e4513654536bf00b90cfc18c33@guerrillamail.com>\r\n" +
			"Date: Wed, 18 Jan 2017 15:43:29 +0000\r\n" +
			"To: \"test@grr.la\" <test@grr.la>\r\n" +
			"From: <62vk44+nziwnkw@guerrillamail.com>\r\n" +
			"Subject: test\r\n" +
			"X-Originating-IP: [60.241.160.150]\r\n" +
			"Content-Type: text/plain; charset=\"utf-8\"\r\n" +
			"Content-Transfer-Encoding: quoted-printable\r\n" +
			"X-Domain-Signer: PHP mailDomainSigner 0.2-20110415 <http://code.google.com/p/php-mail-domain-signer/>\r\n" +
			"DKIM-Signature: v=1; a=rsa-sha256; s=highgrade; d=guerrillamail.com; l=182;\r\n" +
			"\tt=1484754209; c=relaxed/relaxed; h=to:from:subject;\r\n" +
			"\tbh=GHSgjHpBp5QjNn9tzfug681+RcWMOUgpwAuTzppM5wY=;\r\n" +
			"\tb=R7FxWgACnT+pKXqEg15qgzH4ywMFRx5pDlIFCnSt1BfwmLvZPZK7oOLrbiRoGGR2OJnSfyCxeASH\r\n" +
			"\t019LNeLB/B8o+fMRX87m/tBpqIZ2vgXdT9rUCIbSDJnYoCHXakGcF+zGtTE3SEksMbeJQ76aGj6M\r\n" +
			"\tG80p76IT2Xu3iDJLYYWxcAeX+7z4M/bbYNeqxMQcXYZp1wNYlSlHahL6RDUYdcqikDqKoXmzMNVd\r\n" +
			"\tDr0EbH9iiu1DQtfUDzVE5LLus1yn36WU/2KJvEak45gJvm9s9J+Xrcb882CaYkxlAbgQDz1KeQLf\r\n" +
			"\teUyNspyAabkh2yTg7kOvNZSOJtbMSQS6/GMxsg==\r\n" +
			"\r\n" +
			"test=0A.mooo=0A..mooo=0Atest=0A.=0A=0A=0A=0A=0A=0A----=0ASent using Guerril=\r\n" +
			"lamail.com=0ABlock or report abuse: https://www.guerrillamail.com//abuse/?a=\r\n" +
			"=3DVURnES0HUaZbhA8%3D=0A\r\n.\r\n"

	if startErrors := app.Start(); startErrors == nil {
		conn, bufin, err := Connect(config.Servers[0], 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := Command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}

			response, err := Command(conn, bufin, "MAIL FROM:test@grr.la")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			//fmt.Println(response)
			response, err = Command(conn, bufin, "RCPT TO:test@grr.la")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			//fmt.Println(response)
			response, err = Command(conn, bufin, "DATA")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			/*
				response, err = Command(
					conn,
					bufin,
					testHeader+"\r\nHello World\r\n.\r\n")
			*/
			_ = testHeader
			response, err = Command(
				conn,
				bufin,
				email+"\r\n.\r\n")
			//expected := "500 Line too long"
			expected := "250 2.0.0 OK : queued as s0m3l337Ha5hva1u3LOL"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response, err)
			}

		}
		conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}
	logOut.Flush()
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)
}
