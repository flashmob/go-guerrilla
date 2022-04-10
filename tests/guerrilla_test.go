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
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/flashmob/go-guerrilla/internal/tests"
	"github.com/flashmob/go-guerrilla/mail/rfc5321"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/flashmob/go-guerrilla"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"

	"github.com/flashmob/go-guerrilla/tests/testcert"
)

type GuerrillaSuite struct {
	suite.Suite
	config   *guerrilla.AppConfig
	app      guerrilla.Guerrilla
	cleanups []func()
}

func TestGuerrillaSuite(t *testing.T) {
	suite.Run(t, new(GuerrillaSuite))
}

func (s *GuerrillaSuite) SetupTest() {
	logFile, cleanup := tests.TemporaryFilenameCleanup(s.T())
	s.cleanups = append(s.cleanups, cleanup)
	s.config = &guerrilla.AppConfig{
		Servers: []guerrilla.ServerConfig{
			{
				IsEnabled: true,
				TLS: guerrilla.ServerTLSConfig{
					StartTLSOn: true,
					AlwaysOn:   false,
				},
				Hostname:        "mail.guerrillamail.com",
				ListenInterface: fmt.Sprintf("127.0.0.1:%d", tests.GetFreePort(s.T())),
				MaxSize:         100017,
				Timeout:         160,
				MaxClients:      2,
			},
			{
				TLS: guerrilla.ServerTLSConfig{
					AlwaysOn: true,
				},
				Hostname:        "mail.guerrillamail.com",
				ListenInterface: fmt.Sprintf("127.0.0.1:%d", tests.GetFreePort(s.T())),
				MaxSize:         1000001,
				Timeout:         180,
				MaxClients:      1,
				IsEnabled:       true,
			},
		},
		AllowedHosts: []string{
			"spam4.me", "grr.la",
		},
		PidFile:  "go-guerrilla.pid",
		LogFile:  logFile,
		LogLevel: "debug",
		BackendConfig: backends.BackendConfig{
			"log_received_mails": true,
		},
	}
	logger, err := log.GetLogger(s.config.LogFile, "debug")
	s.Require().NoError(err)

	for i := range s.config.Servers {
		err := testcert.GenerateCert(s.config.Servers[i].Hostname, "", 365*24*time.Hour, false, 2048, "P256", "./")
		s.Require().NoError(err)
		s.config.Servers[i].TLS.PrivateKeyFile = s.config.Servers[i].Hostname + ".key.pem"
		s.config.Servers[i].TLS.PublicKeyFile = s.config.Servers[i].Hostname + ".cert.pem"
	}

	backend := getBackend(s.T(), s.config.BackendConfig, logger)
	s.app, err = guerrilla.New(s.config, backend, logger)
	s.Require().NoError(err)
}

func (s *GuerrillaSuite) TearDownTest() {
	cleanTestArtifacts(s.T())
	for _, cleanup := range s.cleanups {
		cleanup()
	}
	s.cleanups = nil
}

func getBackend(t *testing.T, backendConfig map[string]interface{}, l log.Logger) backends.Backend {
	b, err := backends.New(backendConfig, l)
	require.NoError(t, err)
	return b
}

func truncateIfExists(filename string) error {
	_, err := os.Stat(filename)
	if !errors.Is(err, os.ErrNotExist) {
		return os.Truncate(filename, 0)
	}
	return nil
}

func deleteIfExists(filename string) error {
	_, err := os.Stat(filename)
	if !errors.Is(err, os.ErrNotExist) {
		return os.Remove(filename)
	}
	return nil
}

func cleanTestArtifacts(t *testing.T) {
	if err := truncateIfExists("./testlog.log"); err != nil {
		t.Error("could not clean tests/testlog.log:", err)
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

// Testing start and stop of server
func (s *GuerrillaSuite) TestStart() {
	err := s.app.Start()
	s.Require().NoError(err)
	time.Sleep(time.Second)
	s.app.Shutdown()
	b, err := ioutil.ReadFile(s.config.LogFile)
	s.Require().NoError(err)
	logOutput := string(b)
	for _, server := range s.config.Servers {
		s.Assert().Contains(logOutput, fmt.Sprintf("Listening on TCP %s", server.ListenInterface))
		s.Assert().Contains(logOutput, fmt.Sprintf("[%s] Waiting for a new client", server.ListenInterface))
		s.Assert().Contains(logOutput, fmt.Sprintf("Server [%s] has stopped accepting new clients", server.ListenInterface))
		s.Assert().Contains(logOutput, fmt.Sprintf("shutdown completed for [%s]", server.ListenInterface))
		s.Assert().Contains(logOutput, fmt.Sprintf("shutting down pool [%s]", server.ListenInterface))
	}
	s.Assert().Contains(logOutput, "Backend shutdown completed")
}

// Simple smoke-test to see if the server can listen & issues a greeting on connect
func (s *GuerrillaSuite) TestGreeting() {
	s.Require().NoError(s.app.Start())
	// 1. plaintext connection
	conn, err := net.Dial("tcp", s.config.Servers[0].ListenInterface)
	s.Require().NoError(err)
	s.Require().NoError(conn.SetReadDeadline(time.Now().Add(time.Millisecond * 500)))
	greeting, err := bufio.NewReader(conn).ReadString('\n')
	s.Require().NoError(err)
	s.Assert().Contains(greeting, "220 mail.guerrillamail.com SMTP Guerrilla")
	s.Require().NoError(conn.Close())

	// 2. tls connection
	//	roots, err := x509.SystemCertPool()
	conn, err = tls.Dial("tcp", s.config.Servers[1].ListenInterface, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "127.0.0.1",
	})
	s.Require().NoError(err)
	s.Require().NoError(conn.SetReadDeadline(time.Now().Add(time.Millisecond * 500)))
	greeting, err = bufio.NewReader(conn).ReadString('\n')
	s.Require().NoError(err)
	s.Assert().Contains(greeting, "220 mail.guerrillamail.com SMTP Guerrilla")
	s.Require().NoError(conn.Close())
	s.app.Shutdown()
	b, err := ioutil.ReadFile(s.config.LogFile)
	s.Require().NoError(err)
	logOutput := string(b)
	s.Assert().Contains(logOutput, "Handle client [127.0.0.1", "Server did not handle any clients")

}

// start up a server, connect a client, greet, then shutdown, then client sends a command
// expecting: 421 Server is shutting down. Please try again later. Sayonara!
// server should close connection after that
func (s *GuerrillaSuite) TestShutDown() {
	s.Require().NoError(s.app.Start())
	conn, bufin, err := Connect(s.config.Servers[0], 20)
	s.Require().NoError(err)
	// client goes into command state
	_, err = Command(conn, bufin, "HELO localtester")
	s.Require().NoError(err)

	// do a shutdown while the client is connected & in client state
	go s.app.Shutdown()
	time.Sleep(time.Millisecond * 150) // let server to Shutdown

	// issue a command while shutting down
	response, err := Command(conn, bufin, "HELP")
	s.Require().NoError(err)
	expected := "421 4.3.0 Server is shutting down. Please try again later. Sayonara!\r\n"
	s.Assert().Equal(expected, response)
	time.Sleep(time.Millisecond * 250) // let server to close
	s.Require().NoError(conn.Close())

	// assuming server has shutdown by now
	b, err := ioutil.ReadFile(s.config.LogFile)
	s.Require().NoError(err)
	logOutput := string(b)
	s.Assert().Contains(logOutput, "Handle client [127.0.0.1")
}

// add more than 100 recipients, it should fail at 101
func (s *GuerrillaSuite) TestRFC2821LimitRecipients() {
	s.Require().NoError(s.app.Start())
	conn, bufin, err := Connect(s.config.Servers[0], 20)
	s.Require().NoError(err)
	// client goes into command state
	_, err = Command(conn, bufin, "HELO localtester")
	s.Require().NoError(err)

	for i := 0; i < 101; i++ {
		s.T().Logf("RCPT TO:test%d@grr.la\n", i)
		_, err := Command(conn, bufin, fmt.Sprintf("RCPT TO:<test%d@grr.la>", i))
		s.Require().NoError(err)
	}
	response, err := Command(conn, bufin, "RCPT TO:<last@grr.la>")
	s.Require().NoError(err)
	expected := "452 4.5.3 Too many recipients\r\n"
	s.Assert().Equal(expected, response)

	s.Require().NoError(conn.Close())
	s.app.Shutdown()
}

// RCPT TO & MAIL FROM with 64 chars in local part, it should fail at 65
func (s *GuerrillaSuite) TestRFC2832LimitLocalPart() {
	s.Require().NoError(s.app.Start())
	conn, bufin, err := Connect(s.config.Servers[0], 20)
	s.Require().NoError(err)
	// client goes into command state
	_, err = Command(conn, bufin, "HELO localtester")
	s.Require().NoError(err)
	// repeat > 64 characters in local part
	response, err := Command(conn, bufin, fmt.Sprintf("RCPT TO:<%s@grr.la>", strings.Repeat("a", rfc5321.LimitLocalPart+1)))
	s.Require().NoError(err)
	expected := "550 5.5.4 Local part too long, cannot exceed 64 characters\r\n"
	s.Assert().Equal(expected, response)
	// what about if it's exactly 64?
	// repeat > 64 characters in local part
	response, err = Command(conn, bufin, fmt.Sprintf("RCPT TO:<%s@grr.la>", strings.Repeat("a", rfc5321.LimitLocalPart-1)))
	s.Require().NoError(err)
	expected = "250 2.1.5 OK\r\n"
	s.Assert().Equal(expected, response)
	s.Require().NoError(conn.Close())
	s.app.Shutdown()
}

// TestRFC2821LimitPath fail if path > 256 but different error if below
func (s *GuerrillaSuite) TestRFC2821LimitPath() {
	s.Require().NoError(s.app.Start())
	conn, bufin, err := Connect(s.config.Servers[0], 20)
	s.Require().NoError(err)
	// client goes into command state
	_, err = Command(conn, bufin, "HELO localtester")
	s.Require().NoError(err)
	// repeat > 256 characters in local part
	response, err := Command(conn, bufin, fmt.Sprintf("RCPT TO:<%s@grr.la>", strings.Repeat("a", 257-7)))
	s.Require().NoError(err)
	expected := "550 5.5.4 Path too long\r\n"
	s.Assert().Equal(expected, response)
	// what about if it's exactly 256?
	response, err = Command(conn, bufin,
		fmt.Sprintf("RCPT TO:<%s@%s.la>", strings.Repeat("a", 64), strings.Repeat("b", 186)))
	s.Require().NoError(err)
	expected = "454 4.1.1 Error: Relay access denied"
	s.Assert().Contains(response, expected)
	s.Require().NoError(conn.Close())
	s.app.Shutdown()
}

// RFC2821LimitDomain 501 Domain cannot exceed 255 characters
func (s *GuerrillaSuite) TestRFC2821LimitDomain() {
	s.Require().NoError(s.app.Start())
	conn, bufin, err := Connect(s.config.Servers[0], 20)
	s.Require().NoError(err)
	// client goes into command state
	_, err = Command(conn, bufin, "HELO localtester")
	s.Require().NoError(err)
	// repeat > 64 characters in local part
	response, err := Command(conn, bufin, fmt.Sprintf("RCPT TO:<a@%s.l>", strings.Repeat("a", 255-2)))
	s.Require().NoError(err)
	expected := "550 5.5.4 Path too long\r\n"
	s.Assert().Equal(expected, response)
	// what about if it's exactly 255?
	response, err = Command(conn, bufin,
		fmt.Sprintf("RCPT TO:<a@%s.la>", strings.Repeat("b", 255-6)))
	s.Require().NoError(err)
	expected = "454 4.1.1 Error: Relay access denied:"
	s.Assert().Contains(response, expected)
	s.Require().NoError(conn.Close())
	s.app.Shutdown()
}

// TestMailFromCmd tests several different inputs to MAIL FROM command
func (s *GuerrillaSuite) TestMailFromCmd() {
	s.Require().NoError(s.app.Start())
	conn, bufin, err := Connect(s.config.Servers[0], 20)
	s.Require().NoError(err)
	// client goes into command state
	_, err = Command(conn, bufin, "HELO localtester")
	s.Require().NoError(err)
	// Basic valid address
	response, err := Command(conn, bufin, "MAIL FROM:<test@grr.la>")
	expected := "250 2.1.0 OK\r\n"
	s.Require().NoError(err)
	s.Assert().Equal(expected, response)

	// Reset
	response, err = Command(conn, bufin, "RSET")
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)

	// Basic valid address (RfC)
	response, err = Command(conn, bufin, "MAIL FROM:<test@grr.la>")
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)

	// Reset
	response, err = Command(conn, bufin, "RSET")
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)

	// Bounce
	response, err = Command(conn, bufin, "MAIL FROM:<>")
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)

	// Reset
	response, err = Command(conn, bufin, "RSET")
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)

	// No mail from content
	response, err = Command(conn, bufin, "MAIL FROM:")
	s.Require().NoError(err)
	expected = "501 5.5.4 Invalid address\r\n"
	s.Assert().Equal(expected, response)

	// Reset
	response, err = Command(conn, bufin, "RSET")
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)

	// Short mail from content
	response, err = Command(conn, bufin, "MAIL FROM:<")
	s.Require().NoError(err)
	expected = "501 5.5.4 Invalid address\r\n"
	s.Assert().Equal(expected, response)

	// Reset
	response, err = Command(conn, bufin, "RSET")
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)

	// Short mail from content 2
	response, err = Command(conn, bufin, "MAIL FROM:x")
	s.Require().NoError(err)
	expected = "501 5.5.4 Invalid address\r\n"
	s.Assert().Equal(expected, response)

	// Reset
	response, err = Command(conn, bufin, "RSET")
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)

	// What?
	response, err = Command(conn, bufin, "MAIL FROM:<<>>")
	s.Require().NoError(err)
	expected = "501 5.5.4 Invalid address\r\n"
	s.Assert().Equal(expected, response)

	// Reset
	response, err = Command(conn, bufin, "RSET")
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)

	// Invalid address?
	response, err = Command(conn, bufin, "MAIL FROM:<justatest>")
	s.Require().NoError(err)
	expected = "501 5.5.4 Invalid address\r\n"
	s.Assert().Equal(expected, response)

	// Reset
	response, err = Command(conn, bufin, "RSET")
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)

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
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)

	// 8BITMIME (RfC 6152)
	response, err = Command(conn, bufin, "MAIL FROM:<test@grr.la> BODY=8BITMIME")
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)

	// Reset
	response, err = Command(conn, bufin, "RSET")
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)

	// 8BITMIME (RfC 6152) Bounce
	response, err = Command(conn, bufin, "MAIL FROM:<> BODY=8BITMIME")
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)

	// Reset
	response, err = Command(conn, bufin, "RSET")
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)
	s.Require().NoError(conn.Close())
	s.app.Shutdown()
}

// Test several different inputs to MAIL FROM command
func (s *GuerrillaSuite) TestHeloEhlo() {
	s.Require().NoError(s.app.Start())
	conn, bufin, err := Connect(s.config.Servers[0], 20)
	s.Require().NoError(err)
	hostname := s.config.Servers[0].Hostname
	// Test HELO
	response, err := Command(conn, bufin, "HELO localtester")
	s.Require().NoError(err)
	expected := fmt.Sprintf("250 %s Hello\r\n", hostname)
	s.Assert().Equal(expected, response)
	// Reset
	response, err = Command(conn, bufin, "RSET")
	s.Require().NoError(err)
	expected = "250 2.1.0 OK\r\n"
	s.Assert().Equal(expected, response)
	// Test EHLO
	// This is tricky as it is a multiline response
	var fullresp string
	response, err = Command(conn, bufin, "EHLO localtester")
	fullresp = fullresp + response
	s.Require().NoError(err)
	for err == nil {
		response, err = bufin.ReadString('\n')
		fullresp = fullresp + response
		if strings.HasPrefix(response, "250 ") { // Last response has a whitespace and no "-"
			break // bail
		}
	}

	expected = fmt.Sprintf("250-%s Hello\r\n250-SIZE 100017\r\n250-PIPELINING\r\n250-STARTTLS\r\n250-ENHANCEDSTATUSCODES\r\n250 HELP\r\n", hostname)
	s.Assert().Equal(expected, fullresp)
	// be kind, QUIT. And we are sure that bufin does not contain fragments from the EHLO command.
	response, err = Command(conn, bufin, "QUIT")
	s.Require().NoError(err)
	expected = "221 2.0.0 Bye\r\n"
	s.Assert().Equal(expected, response)
	s.Require().NoError(conn.Close())
	s.app.Shutdown()
}

// It should error when MAIL FROM was given twice
func (s *GuerrillaSuite) TestNestedMailCmd() {
	s.Require().NoError(s.app.Start())
	conn, bufin, err := Connect(s.config.Servers[0], 20)
	s.Require().NoError(err)
	// client goes into command state
	_, err = Command(conn, bufin, "HELO localtester")
	s.Require().NoError(err)
	// repeat > 64 characters in local part
	_, err = Command(conn, bufin, "MAIL FROM:<test@grr.la>")
	s.Require().NoError(err)
	response, err := Command(conn, bufin, "MAIL FROM:<test@grr.la>")
	s.Require().NoError(err)
	s.Assert().Equal("503 5.5.1 Error: nested MAIL command\r\n", response)
	// Plot twist: if you EHLO , it should allow MAIL FROM again
	_, err = Command(conn, bufin, "HELO localtester")
	s.Require().NoError(err)
	response, err = Command(conn, bufin, "MAIL FROM:<test@grr.la>")
	s.Require().NoError(err)
	s.Assert().Equal("250 2.1.0 OK\r\n", response)

	// Plot twist: if you RSET , it should allow MAIL FROM again
	response, err = Command(conn, bufin, "RSET")
	s.Require().NoError(err)
	s.Assert().Equal("250 2.1.0 OK\r\n", response)

	response, err = Command(conn, bufin, "MAIL FROM:<test@grr.la>")
	s.Require().NoError(err)
	s.Assert().Equal("250 2.1.0 OK\r\n", response)
	s.Require().NoError(conn.Close())
	s.app.Shutdown()
}

// It should error on a very long command line, exceeding CommandLineMaxLength 1024
func (s *GuerrillaSuite) TestCommandLineMaxLength() {
	s.Require().NoError(s.app.Start())
	conn, bufin, err := Connect(s.config.Servers[0], 20)
	s.Require().NoError(err)
	// client goes into command state
	_, err = Command(conn, bufin, "HELO localtester")
	s.Require().NoError(err)
	// repeat > 1024 characters
	response, err := Command(conn, bufin, strings.Repeat("s", guerrilla.CommandLineMaxLength+1))
	s.Require().NoError(err)

	expected := "554 5.5.1 Line too long.\r\n"
	s.Assert().Equal(expected, response)
	s.Require().NoError(conn.Close())
	s.app.Shutdown()
}

// It should error on a very long message, exceeding servers config value
func (s *GuerrillaSuite) TestDataMaxLength() {
	s.Require().NoError(s.app.Start())
	conn, bufin, err := Connect(s.config.Servers[0], 20)
	s.Require().NoError(err)
	// client goes into command state
	_, err = Command(conn, bufin, "HELO localtester")
	s.Require().NoError(err)

	response, err := Command(conn, bufin, "MAIL FROM:test@grr.la")
	s.Require().NoError(err)
	s.T().Log(response)
	response, err = Command(conn, bufin, "RCPT TO:<test@grr.la>")
	s.Require().NoError(err)
	s.T().Log(response)
	response, err = Command(conn, bufin, "DATA")
	s.Require().NoError(err)
	s.T().Log(response)

	response, err = Command(
		conn,
		bufin,
		fmt.Sprintf("Subject:test\r\n\r\nHello %s\r\n.\r\n",
			strings.Repeat("n", int(s.config.Servers[0].MaxSize-20))))
	s.Require().NoError(err)

	s.Assert().Equal(response, fmt.Sprintf("451 4.3.0 Error: maximum DATA size exceeded (%d)\r\n", s.config.Servers[0].MaxSize))

	s.Require().NoError(conn.Close())
	s.app.Shutdown()
}

func (s *GuerrillaSuite) TestDataCommand() {
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

	s.Require().NoError(s.app.Start())
	conn, bufin, err := Connect(s.config.Servers[0], 20)
	s.Require().NoError(err)
	// client goes into command state
	_, err = Command(conn, bufin, "HELO localtester")
	s.Require().NoError(err)

	response, err := Command(conn, bufin, "MAIL FROM:<test@grr.la>")
	s.Require().NoError(err)
	s.T().Log(response)
	response, err = Command(conn, bufin, "RCPT TO:<test@grr.la>")
	s.Require().NoError(err)
	s.T().Log(response)
	response, err = Command(conn, bufin, "DATA")
	s.Require().NoError(err)
	s.T().Log(response)
	response, err = Command(
		conn,
		bufin,
		email+"\r\n.\r\n")
	s.Require().NoError(err)
	s.Assert().Contains(response, "250 2.0.0 OK: queued as ")
	s.Require().NoError(conn.Close())
	s.app.Shutdown()
}

// Fuzzer crashed the server by submitting "DATA\r\n" as the first command
func (s *GuerrillaSuite) TestFuzz86f25b86b09897aed8f6c2aa5b5ee1557358a6de() {
	s.Require().NoError(s.app.Start())
	conn, bufin, err := Connect(s.config.Servers[0], 20)
	s.Require().NoError(err)
	response, err := Command(
		conn,
		bufin,
		"DATA\r\n")
	s.Require().NoError(err)
	expected := "503 5.5.1 Error: No recipients\r\n"
	s.Assert().Equal(expected, response)
	s.Require().NoError(conn.Close())
	s.app.Shutdown()
}

// Appears to hang the fuzz test, but not server.
func (s *GuerrillaSuite) TestFuzz21c56f89989d19c3bbbd81b288b2dae9e6dd2150() {
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

	s.Require().NoError(s.app.Start())
	conn, bufin, err := Connect(s.config.Servers[0], 20)
	s.Require().NoError(err)
	response, err := Command(
		conn,
		bufin,
		str)
	s.Require().NoError(err)
	expected := "554 5.5.1 Unrecognized command\r\n"
	s.Assert().Equal(expected, response)
	s.Require().NoError(conn.Close())
	s.app.Shutdown()
}
