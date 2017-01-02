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
	//	"crypto/x509"
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
		app, _ = guerrilla.New(&config.AppConfig, &backend)
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

func connect(serverIndex int, deadline time.Duration) (net.Conn, *bufio.Reader, error) {
	var bufin *bufio.Reader

	conn, err := net.Dial("tcp", config.Servers[serverIndex].ListenInterface)
	if err != nil {
		// handle error
		//t.Error("Cannot dial server", config.Servers[0].ListenInterface)
		return conn, bufin, errors.New("Cannot dial server: " + config.Servers[serverIndex].ListenInterface + "," + err.Error())
	}
	bufin = bufio.NewReader(conn)

	// should be ample time to complete the test
	conn.SetDeadline(time.Now().Add(time.Duration(time.Second * deadline)))
	// read greeting, ignore it
	_, err = bufin.ReadString('\n')
	return conn, bufin, err
}

func command(conn net.Conn, bufin *bufio.Reader, command string) (reply string, err error) {
	_, err = fmt.Fprintln(conn, command+"\r")
	if err == nil {
		return bufin.ReadString('\n')
	}
	return "", err
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
		conn, bufin, err := connect(0, 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}

			// do a shutdown while the client is connected & in client state
			go app.Shutdown()

			// issue a command while shutting down
			response, err := command(conn, bufin, "HELP")
			if err != nil {
				t.Error("Help command failed", err.Error())
			}
			expected := "421 Server is shutting down. Please try again later. Sayonara!"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not shut down with", expected, ", it said:"+response)
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
		conn, bufin, err := connect(0, 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}

			for i := 0; i < 101; i++ {
				if _, err := command(conn, bufin, fmt.Sprintf("RCPT TO:test%d@grr.la", i)); err != nil {
					t.Error("RCPT TO", err.Error())
					break
				}
			}
			response, err := command(conn, bufin, "RCPT TO:last@grr.la")
			if err != nil {
				t.Error("rcpt command failed", err.Error())
			}
			expected := "452 Too many recipients"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
		}

		conn.Close()
		app.Shutdown()

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
		conn, bufin, err := connect(0, 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}
			// repeat > 64 characters in local part
			response, err := command(conn, bufin, fmt.Sprintf("RCPT TO:%s@grr.la", strings.Repeat("a", 65)))
			if err != nil {
				t.Error("rcpt command failed", err.Error())
			}
			expected := "501 Local part too long"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
			// what about if it's exactly 64?
			// repeat > 64 characters in local part
			response, err = command(conn, bufin, fmt.Sprintf("RCPT TO:%s@grr.la", strings.Repeat("a", 64)))
			if err != nil {
				t.Error("rcpt command failed", err.Error())
			}
			expected = "250 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
		}

		conn.Close()
		app.Shutdown()

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
		conn, bufin, err := connect(0, 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}
			// repeat > 256 characters in local part
			response, err := command(conn, bufin, fmt.Sprintf("RCPT TO:%s@grr.la", strings.Repeat("a", 257-7)))
			if err != nil {
				t.Error("rcpt command failed", err.Error())
			}
			expected := "501 Path too long"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
			// what about if it's exactly 256?
			response, err = command(conn, bufin,
				fmt.Sprintf("RCPT TO:%s@%s.la", strings.Repeat("a", 64), strings.Repeat("b", 257-5-64)))
			if err != nil {
				t.Error("rcpt command failed", err.Error())
			}
			expected = "454 Error: Relay access denied"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
		}
		conn.Close()
		app.Shutdown()
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
		conn, bufin, err := connect(0, 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}
			// repeat > 64 characters in local part
			response, err := command(conn, bufin, fmt.Sprintf("RCPT TO:a@%s.l", strings.Repeat("a", 255-2)))
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected := "501 Path too long"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
			// what about if it's exactly 255?
			response, err = command(conn, bufin,
				fmt.Sprintf("RCPT TO:a@%s.la", strings.Repeat("b", 255-4)))
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "454 Error: Relay access denied"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
		}
		conn.Close()
		app.Shutdown()
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
		conn, bufin, err := connect(0, 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}
			// repeat > 64 characters in local part
			response, err := command(conn, bufin, "MAIL FROM:test@grr.la")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			response, err = command(conn, bufin, "MAIL FROM:test@grr.la")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected := "503 Error: nested MAIL command"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
			// Plot twist: if you EHLO , it should allow MAIL FROM again
			if _, err := command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}
			response, err = command(conn, bufin, "MAIL FROM:test@grr.la")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
			// Plot twist: if you RSET , it should allow MAIL FROM again
			response, err = command(conn, bufin, "RSET")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

			response, err = command(conn, bufin, "MAIL FROM:test@grr.la")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

		}
		conn.Close()
		app.Shutdown()
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
		conn, bufin, err := connect(0, 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}
			// repeat > 1024 characters
			response, err := command(conn, bufin, strings.Repeat("s", guerrilla.CommandLineMaxLength+1))
			if err != nil {
				t.Error("command failed", err.Error())
			}

			expected := "500 Line too long"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

		}
		conn.Close()
		app.Shutdown()
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
		conn, bufin, err := connect(0, 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// client goes into command state
			if _, err := command(conn, bufin, "HELO localtester"); err != nil {
				t.Error("Hello command failed", err.Error())
			}

			response, err := command(conn, bufin, "MAIL FROM:test@grr.la")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			//fmt.Println(response)
			response, err = command(conn, bufin, "RCPT TO:test@grr.la")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			//fmt.Println(response)
			response, err = command(conn, bufin, "DATA")
			if err != nil {
				t.Error("command failed", err.Error())
			}

			response, err = command(
				conn,
				bufin,
				fmt.Sprintf("Subject:test\r\n\r\nHello %s\r\n.\r\n",
					strings.Repeat("n", int(config.Servers[0].MaxSize-26))))

			//expected := "500 Line too long"
			expected := "550 Error: Maximum line length exceeded"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response, err)
			}

		}
		conn.Close()
		app.Shutdown()
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
	// don't forget to reset
	logBuffer.Reset()
	logIn.Reset(&logBuffer)
}
