// integration / smokeless
// =======================
// Tests are in a different package so we can test as a consumer of the guerrilla package
// The following are integration / smokeless, that test the overall server.
// (Please put unit tests to go in a different file)
// How it works:
// Server's log output is redirected to the testlog file which is then used by the tests to look for
// expected behaviour.
//
// (self signed certs are also generated on each run)
// server's responses from a connection are also used to check for expected behaviour
// to run:
// $ go test

package test

import (
	"encoding/json"
	"github.com/flashmob/go-guerrilla/mail/smtp"
	"testing"

	"time"

	"github.com/flashmob/go-guerrilla"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"

	"bufio"

	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strings"

	"os"

	"github.com/flashmob/go-guerrilla/tests/testcert"
)

type TestConfig struct {
	guerrilla.AppConfig
	BackendConfig backends.BackendConfig `json:"backend"`
}

var (

	// app config loaded here
	config *TestConfig

	app guerrilla.Guerrilla

	initErr error

	logger log.Logger
)

func init() {

	config = &TestConfig{}
	if err := json.Unmarshal([]byte(configJson), config); err != nil {
		initErr = errors.New("Could not Unmarshal config," + err.Error())
	} else {
		logger, _ = log.GetLogger(config.LogFile, "debug")
		initErr = setupCerts(config)
		if initErr != nil {
			return
		}
		backend, _ := getBackend(config.BackendConfig, logger)
		app, initErr = guerrilla.New(&config.AppConfig, logger, backend)
	}

}

// a configuration file with a dummy backend
var configJson = `
{
    "log_file" : "./testlog",
    "log_level" : "debug",
    "pid_file" : "go-guerrilla.pid",
    "allowed_hosts": ["spam4.me","grr.la"],
	
    "backend" : {
		"processors" : {
			"debugger" : {
				"log_received_mails" : true
			}
		}
	},
    "servers" : [
        {
            "is_enabled" : true,
            "host_name":"mail.guerrillamail.com",
            "max_size": 100017,
            "timeout":160,
            "listen_interface":"127.0.0.1:2526", 
            "max_clients": 2,
            "log_file" : "",
			"tls" : {
				"private_key_file":"/this/will/be/ignored/guerrillamail.com.key.pem",
            	"public_key_file":"/this/will/be/ignored//guerrillamail.com.crt",
				"start_tls_on":true,
            	"tls_always_on":false
			}
        },

        {
            "is_enabled" : true,
            "host_name":"mail.guerrillamail.com",
            "max_size":1000001,
            "timeout":180,
            "listen_interface":"127.0.0.1:4654",
            "max_clients":1,
            "log_file" : "",
			"tls" : {
				"private_key_file":"/this/will/be/ignored/guerrillamail.com.key.pem",
            	"public_key_file":"/this/will/be/ignored/guerrillamail.com.crt",
				"start_tls_on":false,
            	"tls_always_on":true
			}
        }
    ]
}
`

func getBackend(backendConfig backends.BackendConfig, l log.Logger) (backends.Backend, error) {
	_ = backendConfig.ConfigureDefaults()
	b, err := backends.New(backends.DefaultGateway, backendConfig, l)
	if err != nil {
		fmt.Println("backend init error", err)
		os.Exit(1)
	}
	return b, err
}

func setupCerts(c *TestConfig) error {
	for i := range c.Servers {
		err := testcert.GenerateCert(c.Servers[i].Hostname, "", 365*24*time.Hour, false, 2048, "P256", "./")
		if err != nil {
			return err
		}
		c.Servers[i].TLS.PrivateKeyFile = c.Servers[i].Hostname + ".key.pem"
		c.Servers[i].TLS.PublicKeyFile = c.Servers[i].Hostname + ".cert.pem"
	}
	return nil
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

	if err := truncateIfExists("./testlog"); err != nil {
		t.Error("could not clean tests/testlog:", err)
	}

	letters := []byte{'A', 'B', 'C', 'D', 'E'}
	for _, l := range letters {
		if err := deleteIfExists("configJson" + string(l) + ".json"); err != nil {
			t.Error("could not delete configJson"+string(l)+".json:", err)
		}
	}

	if err := deleteIfExists("./go-guerrilla.pid"); err != nil {
		t.Error("could not delete ./guerrilla", err)
	}
	if err := deleteIfExists("./go-guerrilla2.pid"); err != nil {
		t.Error("could not delete ./go-guerrilla2.pid", err)
	}

	if err := deleteIfExists("./mail.guerrillamail.com.cert.pem"); err != nil {
		t.Error("could not delete ./mail.guerrillamail.com.cert.pem", err)
	}
	if err := deleteIfExists("./mail.guerrillamail.com.key.pem"); err != nil {
		t.Error("could not delete ./mail.guerrillamail.com.key.pem", err)
	}

}

func TestMatchConfig(t *testing.T) {
	str := `
time="2020-07-20T14:14:17+09:00" level=info msg="pid_file written" file=tests/go-guerrilla.pid pid=15247
time="2020-07-20T14:14:17+09:00" level=debug msg="making servers"
time="2020-07-20T14:14:17+09:00" level=info msg="processing worker started" gateway=default id=3
time="2020-07-20T14:14:17+09:00" level=info msg="processing worker started" gateway=default id=2
time="2020-07-20T14:14:17+09:00" level=info msg="starting server" iface="127.0.0.1:2526" serverID=0
time="2020-07-20T14:14:17+09:00" level=info msg="processing worker started" gateway=default id=1
time="2020-07-20T14:14:17+09:00" level=info msg="processing worker started" gateway=temp id=2
time="2020-07-20T14:14:17+09:00" level=info msg="processing worker started" gateway=default id=4
time="2020-07-20T14:14:17+09:00" level=info msg="processing worker started" gateway=temp id=3
time="2020-07-20T14:14:17+09:00" level=info msg="processing worker started" gateway=temp id=4
time="2020-07-20T14:14:17+09:00" level=info msg="processing worker started" gateway=temp id=1
time="2020-07-20T14:14:17+09:00" level=info msg="listening on TCP" iface="127.0.0.1:2526" serverID=0
time="2020-07-20T14:14:17+09:00" level=debug msg="waiting for a new client" nextSeq=1 serverID=0


`
	defer cleanTestArtifacts(t)
	if !MatchLog(str, 1, "msg", "making servers") {
		t.Error("making servers not matched")
	}

	if MatchLog(str, 10, "msg", "making servers") {
		t.Error("not expecting making servers matched")
	}

	if !MatchLog(str, 1, "msg", "listening on TCP", "serverID", 0) {
		t.Error("2 not pairs matched")
	}

}

// Testing start and stop of server
func TestStart(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
	if startErrors := app.Start(); startErrors != nil {
		t.Error(startErrors)
		t.FailNow()
	}
	time.Sleep(time.Second)
	app.Shutdown()
	if read, err := ioutil.ReadFile("./testlog"); err == nil {
		logOutput := string(read)
		if !MatchLog(logOutput, 1, "msg", "listening on TCP", "iface", "127.0.0.1:2526") {
			t.Error("Server did not listen on 127.0.0.1:4654")
		}

		if !MatchLog(logOutput, 1, "msg", "listening on TCP", "iface", "127.0.0.1:2526") {
			t.Error("Server did not listen on 127.0.0.1:2526")
		}

		if !MatchLog(logOutput, 1, "msg", "waiting for a new client", "iface", "127.0.0.1:4654") {
			t.Error("Server did not wait on 127.0.0.1:4654")
		}

		if !MatchLog(logOutput, 1, "msg", "waiting for a new client", "iface", "127.0.0.1:2526") {
			t.Error("Server did not wait on 127.0.0.1:2526")
		}

		if !MatchLog(logOutput, 1, "msg", "server has stopped accepting new clients", "iface", "127.0.0.1:4654") {
			t.Error("Server did not stop on 127.0.0.1:4654")
		}
		if !MatchLog(logOutput, 1, "msg", "server has stopped accepting new clients", "iface", "127.0.0.1:2526") {
			t.Error("Server did not stop on 127.0.0.1:2526")
		}

		if !MatchLog(logOutput, 1, "msg", "shutdown completed", "iface", "127.0.0.1:4654") {
			t.Error("Server did not complete shutdown on 127.0.0.1:4654")
		}

		if !MatchLog(logOutput, 1, "msg", "shutdown completed", "iface", "127.0.0.1:2526") {
			t.Error("Server did not complete shutdown on 127.0.0.1:2526")
		}

		if !MatchLog(logOutput, 1, "msg", "shutting down pool", "iface", "127.0.0.1:4654") {
			t.Error("Server did not shutdown pool on 127.0.0.1:4654")
		}

		if !MatchLog(logOutput, 1, "msg", "shutting down pool", "iface", "127.0.0.1:2526") {
			t.Error("Server did not shutdown pool on 127.0.0.1:2526")
		}

		if !MatchLog(logOutput, 1, "msg", "backend shutdown completed") {
			t.Error("Backend didn't shut down")
		}
	}

}

// Simple smoke-test to see if the server can listen & issues a greeting on connect
func TestGreeting(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
	if startErrors := app.Start(); startErrors == nil {
		// 1. plaintext connection
		conn, err := net.Dial("tcp", config.Servers[0].ListenInterface)
		if err != nil {
			// handle error
			t.Error("Cannot dial server", config.Servers[0].ListenInterface)
		}
		if err := conn.SetReadDeadline(time.Now().Add(time.Millisecond * 500)); err != nil {
			t.Error(err)
		}
		greeting, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			t.Error(err)
			t.FailNow()
		} else {
			expected := "220 mail.guerrillamail.com SMTP Guerrilla"
			if strings.Index(greeting, expected) != 0 {
				t.Error("Server[1] did not have the expected greeting prefix", expected)
			}
		}
		_ = conn.Close()

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
		if err := conn.SetReadDeadline(time.Now().Add(time.Millisecond * 500)); err != nil {
			t.Error(err)
		}
		greeting, err = bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			t.Error(err)
			t.FailNow()
		} else {
			expected := "220 mail.guerrillamail.com SMTP Guerrilla"
			if strings.Index(greeting, expected) != 0 {
				t.Error("Server[2] (TLS) did not have the expected greeting prefix", expected)
			}
		}
		_ = conn.Close()

	} else {
		fmt.Println("Nope", startErrors)
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			t.FailNow()
		}
	}
	app.Shutdown()
	if read, err := ioutil.ReadFile("./testlog"); err == nil {
		logOutput := string(read)
		if !MatchLog(logOutput, 1, "msg", "handle client", "peer", "127.0.0.1") {
			t.Error("Server did not handle any clients")
		}
	}

}

// start up a server, connect a client, greet, then shutdown, then client sends a command
// expecting: 421 Server is shutting down. Please try again later. Sayonara!
// server should close connection after that
func TestShutDown(t *testing.T) {

	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
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

		_ = conn.Close()

	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}
	// assuming server has shutdown by now
	if read, err := ioutil.ReadFile("./testlog"); err == nil {
		logOutput := string(read)
		//	fmt.Println(logOutput)
		if !MatchLog(logOutput, 1, "msg", "handle client", "peer", "127.0.0.1") {
			t.Error("Server did not handle any clients")
		}

	}

}

// add more than 100 recipients, it should fail at 101
func TestRFC2821LimitRecipients(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
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
				//fmt.Println(fmt.Sprintf("RCPT TO:test%d@grr.la", i))
				if _, err := Command(conn, bufin, fmt.Sprintf("RCPT TO:<test%d@grr.la>", i)); err != nil {
					t.Error("RCPT TO", err.Error())
					break
				}
			}
			response, err := Command(conn, bufin, "RCPT TO:<last@grr.la>")
			if err != nil {
				t.Error("rcpt command failed", err.Error())
			}
			expected := "452 4.5.3 Too many recipients"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
		}

		_ = conn.Close()
		app.Shutdown()

	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}

}

// RCPT TO & MAIL FROM with 64 chars in local part, it should fail at 65
func TestRFC2832LimitLocalPart(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
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
			response, err := Command(conn, bufin, fmt.Sprintf("RCPT TO:<%s@grr.la>", strings.Repeat("a", smtp.LimitLocalPart+1)))
			if err != nil {
				t.Error("rcpt command failed", err.Error())
			}
			expected := "550 5.5.4 Local part too long"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
			// what about if it's exactly 64?
			// repeat > 64 characters in local part
			response, err = Command(conn, bufin, fmt.Sprintf("RCPT TO:<%s@grr.la>", strings.Repeat("a", smtp.LimitLocalPart-1)))
			if err != nil {
				t.Error("rcpt command failed", err.Error())
			}
			expected = "250 2.1.5 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
		}

		_ = conn.Close()
		app.Shutdown()

	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}

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
			response, err := Command(conn, bufin, fmt.Sprintf("RCPT TO:<%s@grr.la>", strings.Repeat("a", 257-7)))
			if err != nil {
				t.Error("rcpt command failed", err.Error())
			}
			expected := "550 5.5.4 Path too long"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
			// what about if it's exactly 256?
			response, err = Command(conn, bufin,
				fmt.Sprintf("RCPT TO:<%s@%s.la>", strings.Repeat("a", 64), strings.Repeat("b", 186)))
			if err != nil {
				t.Error("rcpt command failed", err.Error())
			}
			expected = "454 4.1.1 Error: Relay access denied"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
		}
		_ = conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}
}

// RFC2821LimitDomain 501 Domain cannot exceed 255 characters
func TestRFC2821LimitDomain(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
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
			response, err := Command(conn, bufin, fmt.Sprintf("RCPT TO:<a@%s.l>", strings.Repeat("a", 255-2)))
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected := "550 5.5.4 Path too long"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
			// what about if it's exactly 255?
			response, err = Command(conn, bufin,
				fmt.Sprintf("RCPT TO:<a@%s.la>", strings.Repeat("b", 255-6)))
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "454 4.1.1 Error: Relay access denied"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
		}
		_ = conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}

}

// Test several different inputs to MAIL FROM command
func TestMailFromCmd(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
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
			response, err := Command(conn, bufin, "MAIL FROM:<test@grr.la>")
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

			// No mail from content
			response, err = Command(conn, bufin, "MAIL FROM:")
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

			// Short mail from content
			response, err = Command(conn, bufin, "MAIL FROM:<")
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

			// Short mail from content 2
			response, err = Command(conn, bufin, "MAIL FROM:x")
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

			/*
				// todo SMTPUTF8 not implemented for now,
				response, err = Command(conn, bufin, "MAIL FROM:<anöthertest@grr.la>")
				if err != nil {
					t.Error("command failed", err.Error())
				}
				expected = "250 2.1.0 OK"
				if strings.Index(response, expected) != 0 {
					t.Error("Server did not respond with", expected, ", it said:"+response)
				}
			*/

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
		_ = conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}

}

// Test several different inputs to MAIL FROM command
func TestHeloEhlo(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
	if startErrors := app.Start(); startErrors == nil {
		conn, bufin, err := Connect(config.Servers[0], 20)
		hostname := config.Servers[0].Hostname
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {
			// Test HELO
			response, err := Command(conn, bufin, "HELO localtester")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected := fmt.Sprintf("250 %s Hello", hostname)
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
			// Test EHLO
			// This is tricky as it is a multiline response
			var fullresp string
			response, err = Command(conn, bufin, "EHLO localtester")
			fullresp = fullresp + response
			if err != nil {
				t.Error("command failed", err.Error())
			}
			for err == nil {
				response, err = bufin.ReadString('\n')
				fullresp = fullresp + response
				if strings.HasPrefix(response, "250 ") { // Last response has a whitespace and no "-"
					break // bail
				}
			}

			expected = fmt.Sprintf("250-%s Hello\r\n250-SIZE 100017\r\n250-PIPELINING\r\n250-STARTTLS\r\n250-ENHANCEDSTATUSCODES\r\n250-8BITMIME\r\n250 HELP\r\n", hostname)
			if fullresp != expected {
				t.Error("Server did not respond with [" + expected + "], it said [" + fullresp + "]")
			}
			// be kind, QUIT. And we are sure that bufin does not contain fragments from the EHLO command.
			response, err = Command(conn, bufin, "QUIT")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "221 2.0.0 Bye"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}
		}
		_ = conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}

}

// It should error when MAIL FROM was given twice
func TestNestedMailCmd(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
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
			response, err := Command(conn, bufin, "MAIL FROM:<test@grr.la>")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			response, err = Command(conn, bufin, "MAIL FROM:<test@grr.la>")
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
			response, err = Command(conn, bufin, "MAIL FROM:<test@grr.la>")
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

			response, err = Command(conn, bufin, "MAIL FROM:<test@grr.la>")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			expected = "250 2.1.0 OK"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

		}
		_ = conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}
}

// It should error on a very long command line, exceeding CommandLineMaxLength 1024
func TestCommandLineMaxLength(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
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
		_ = conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}

}

// It should error on a very long message, exceeding servers config value
func TestDataMaxLength(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
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
			response, err = Command(conn, bufin, "RCPT TO:<test@grr.la>")
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
			expected := "451 4.3.0 Error: maximum DATA size exceeded"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response)
			}

		}
		_ = conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}

}

func TestDataCommand(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
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

			response, err := Command(conn, bufin, "MAIL FROM:<test@grr.la>")
			if err != nil {
				t.Error("command failed", err.Error())
			}
			//fmt.Println(response)
			response, err = Command(conn, bufin, "RCPT TO:<test@grr.la>")
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
			expected := "250 2.0.0 OK: queued as "
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response, err)
			}

		}
		_ = conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}
}

// Fuzzer crashed the server by submitting "DATA\r\n" as the first command
func TestFuzz86f25b86b09897aed8f6c2aa5b5ee1557358a6de(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
	if startErrors := app.Start(); startErrors == nil {
		conn, bufin, err := Connect(config.Servers[0], 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {

			response, err := Command(
				conn,
				bufin,
				"DATA\r\n")
			expected := "503 5.5.1 Error: No recipients"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response, err)
			}

		}
		_ = conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}
}

// Appears to hang the fuzz test, but not server.
func TestFuzz21c56f89989d19c3bbbd81b288b2dae9e6dd2150(t *testing.T) {
	if initErr != nil {
		t.Error(initErr)
		t.FailNow()
	}
	defer cleanTestArtifacts(t)
	str := "X_\r\nMAIL FROM:<u\xfd\xfdrU" +
		"\x10c22695140\xfd727235530" +
		" Walter Sobchak\x1a\tDon" +
		"ny, x_6_, Donnyre   " +
		"\t\t outof89 !om>\r\nMAI" +
		"L\t\t \t\tFROM:<C4o\xfd\xfdr@e" +
		"xample.c22695140\xfd727" +
		"235530 Walter Sobcha" +
		"k: Donny, you>re out" +
		" of your element!om>" +
		"\r\nMAIL RCPT TO:t@IRS" +
		"ETRCPTIRSETRCP:<\x00\xfd\xfdr" +
		"@example 7A924_F__4_" +
		"c22695140\xfd-061.0x30C" +
		"8bC87fE4d3 Walter MA" +
		"IL Donny, youiq__n_l" +
		"wR8qs_0RBcw_0hIY_pS_" +
		"___x9_E0___sL598_G82" +
		"_6 out   your elemen" +
		"t!>\r\nX _9KB___X_p:<o" +
		"ut\xfd\xfdr@example9gTnr2N" +
		"__Vl_T7U_AqfU_dPfJ_0" +
		"HIqKK0037f6W_KGM_y_Z" +
		"_9_96_w_815Q572py2_9" +
		"F\xfd727235530Walter\tSo" +
		"bchakRSET MAIL from:" +
		" : cows eat\t\t  grass" +
		" , _S___46_PbG03_iW'" +
		"__v5L2_2L_J61u_38J55" +
		"_PpwQ_Fs_7L_3p7S_t__" +
		"g9XP48T_9HY_EDl_c_C3" +
		"3_3b708EreT_OR out 9" +
		"9_pUY4 \t\t\t     \x05om>\r" +
		"\n FROM<u\xfd\xfdr@example." +
		"<\xfd-05110602 Walter S" +
		"obchak: Donny, \t\t  w" +
		"50TI__m_5EsC___n_l_d" +
		"__57GP9G02_32n_FR_xw" +
		"_2_103___rnED5PGIKN7" +
		"BBs3VIuNV_514qDBp_Gs" +
		"_qj4\tre out all cows" +
		" eatof your element\x03" +
		"om>\r\n_2 FROM:<u\x10\xfdr@e" +
		"xample.oQ_VLq909_E_5" +
		"AQ7_4_\xfd1935012674150" +
		"6773818422493001838." +
		"-010\tWalter\tSobchak:" +
		" Donny, youyouteIz2y" +
		"__Z2q5_qoA're Q6MP2_" +
		"CT_z70____0c0nU7_83d" +
		"4jn_eFD7h_9MbPjr_s_L" +
		"9_X23G_7 of _kU_L9Yz" +
		"_K52345QVa902H1__Hj_" +
		"Nl_PP2tW2ODi0_V80F15" +
		"_i65i_V5uSQdiG eleme" +
		"nt!om>\r\n"

	if startErrors := app.Start(); startErrors == nil {
		conn, bufin, err := Connect(config.Servers[0], 20)
		if err != nil {
			// handle error
			t.Error(err.Error(), config.Servers[0].ListenInterface)
			t.FailNow()
		} else {

			response, err := Command(
				conn,
				bufin,
				str)
			expected := "554 5.5.1 Unrecognized command"
			if strings.Index(response, expected) != 0 {
				t.Error("Server did not respond with", expected, ", it said:"+response, err)
			}

		}
		_ = conn.Close()
		app.Shutdown()
	} else {
		if startErrors := app.Start(); startErrors != nil {
			t.Error(startErrors)
			app.Shutdown()
			t.FailNow()
		}
	}
}
