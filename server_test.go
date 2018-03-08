package guerrilla

import (
	"testing"

	"bufio"
	"net/textproto"
	"strings"
	"sync"

	"fmt"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mocks"
	"net"
)

// getMockServerConfig gets a mock ServerConfig struct used for creating a new server
func getMockServerConfig() *ServerConfig {
	sc := &ServerConfig{
		IsEnabled:       true, // not tested here
		Hostname:        "saggydimes.test.com",
		MaxSize:         1024, // smtp message max size
		PrivateKeyFile:  "./tests/mail.guerrillamail.com.key.pem",
		PublicKeyFile:   "./tests/mail.guerrillamail.com.cert.pem",
		Timeout:         5,
		ListenInterface: "127.0.0.1:2529",
		StartTLSOn:      true,
		TLSAlwaysOn:     false,
		MaxClients:      30, // not tested here
		LogFile:         "./tests/testlog",
	}
	return sc
}

// getMockServerConn gets a new server using sc. Server will be using a mocked TCP connection
// using the dummy backend
// RCP TO command only allows test.com host
func getMockServerConn(sc *ServerConfig, t *testing.T) (*mocks.Conn, *server) {
	var logOpenError error
	var mainlog log.Logger
	mainlog, logOpenError = log.GetLogger(sc.LogFile, "debug")
	if logOpenError != nil {
		mainlog.WithError(logOpenError).Errorf("Failed creating a logger for mock conn [%s]", sc.ListenInterface)
	}
	backend, err := backends.New(
		backends.BackendConfig{"log_received_mails": true, "save_workers_size": 1},
		mainlog)
	if err != nil {
		t.Error("new dummy backend failed because:", err)
	}
	server, err := newServer(sc, backend, mainlog)
	if err != nil {
		//t.Error("new server failed because:", err)
	} else {
		server.setAllowedHosts([]string{"test.com"})
	}
	conn := mocks.NewConn()
	return conn, server
}

func TestHandleClient(t *testing.T) {
	var mainlog log.Logger
	var logOpenError error
	sc := getMockServerConfig()
	mainlog, logOpenError = log.GetLogger(sc.LogFile, "debug")
	if logOpenError != nil {
		mainlog.WithError(logOpenError).Errorf("Failed creating a logger for mock conn [%s]", sc.ListenInterface)
	}
	conn, server := getMockServerConn(sc, t)
	// call the serve.handleClient() func in a goroutine.
	client := NewClient(conn.Server, 1, mainlog, mail.NewPool(5))
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		server.handleClient(client)
		wg.Done()
	}()
	// Wait for the greeting from the server
	r := textproto.NewReader(bufio.NewReader(conn.Client))
	line, _ := r.ReadLine()
	//	fmt.Println(line)
	w := textproto.NewWriter(bufio.NewWriter(conn.Client))
	w.PrintfLine("HELO test.test.com")
	line, _ = r.ReadLine()
	//fmt.Println(line)
	w.PrintfLine("QUIT")
	line, _ = r.ReadLine()
	//fmt.Println("line is:", line)
	expected := "221 2.0.0 Bye"
	if strings.Index(line, expected) != 0 {
		t.Error("expected", expected, "but got:", line)
	}
	wg.Wait() // wait for handleClient to exit
}

func TestXClient(t *testing.T) {
	var mainlog log.Logger
	var logOpenError error
	sc := getMockServerConfig()
	sc.XClientOn = true
	mainlog, logOpenError = log.GetLogger(sc.LogFile, "debug")
	if logOpenError != nil {
		mainlog.WithError(logOpenError).Errorf("Failed creating a logger for mock conn [%s]", sc.ListenInterface)
	}
	conn, server := getMockServerConn(sc, t)
	// call the serve.handleClient() func in a goroutine.
	client := NewClient(conn.Server, 1, mainlog, mail.NewPool(5))
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		server.handleClient(client)
		wg.Done()
	}()
	// Wait for the greeting from the server
	r := textproto.NewReader(bufio.NewReader(conn.Client))
	line, _ := r.ReadLine()
	//	fmt.Println(line)
	w := textproto.NewWriter(bufio.NewWriter(conn.Client))
	w.PrintfLine("HELO test.test.com")
	line, _ = r.ReadLine()
	//fmt.Println(line)
	w.PrintfLine("XCLIENT ADDR=212.96.64.216 NAME=[UNAVAILABLE]")
	line, _ = r.ReadLine()

	if client.RemoteIP != "212.96.64.216" {
		t.Error("client.RemoteIP should be 212.96.64.216, but got:", client.RemoteIP)
	}
	expected := "250 2.1.0 OK"
	if strings.Index(line, expected) != 0 {
		t.Error("expected", expected, "but got:", line)
	}

	// try malformed input
	w.PrintfLine("XCLIENT c")
	line, _ = r.ReadLine()

	expected = "250 2.1.0 OK"
	if strings.Index(line, expected) != 0 {
		t.Error("expected", expected, "but got:", line)
	}

	w.PrintfLine("QUIT")
	line, _ = r.ReadLine()
	wg.Wait() // wait for handleClient to exit
}

// The backend gateway should time out after 1 second because it sleeps for 2 sec.
// The transaction should wait until finished, and then test to see if we can do
// a second transaction
func TestGatewayTimeout(t *testing.T) {

	bcfg := backends.BackendConfig{
		"save_workers_size":   1,
		"save_process":        "HeadersParser|Debugger",
		"log_received_mails":  true,
		"primary_mail_host":   "example.com",
		"gw_save_timeout":     "1s",
		"gw_val_rcpt_timeout": "1s",
		"sleep_seconds":       2,
	}

	cfg := &AppConfig{
		LogFile:      log.OutputOff.String(),
		AllowedHosts: []string{"grr.la"},
	}
	cfg.BackendConfig = bcfg

	d := Daemon{Config: cfg}
	err := d.Start()

	if err != nil {
		t.Error("server didn't start")
	} else {

		conn, err := net.Dial("tcp", "127.0.0.1:2525")
		if err != nil {

			return
		}
		in := bufio.NewReader(conn)
		str, err := in.ReadString('\n')
		fmt.Fprint(conn, "HELO host\r\n")
		str, err = in.ReadString('\n')
		// perform 2 transactions
		// both should panic.
		for i := 0; i < 2; i++ {
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
			expect := "transaction timeout"
			if strings.Index(str, expect) == -1 {
				t.Error("Expected the reply to have'", expect, "'but got", str)
			}
		}
		_ = str

		d.Shutdown()
	}
}

// The processor will panic and gateway should recover from it
func TestGatewayPanic(t *testing.T) {
	bcfg := backends.BackendConfig{
		"save_workers_size":   1,
		"save_process":        "HeadersParser|Debugger",
		"log_received_mails":  true,
		"primary_mail_host":   "example.com",
		"gw_save_timeout":     "2s",
		"gw_val_rcpt_timeout": "2s",
		"sleep_seconds":       1,
	}

	cfg := &AppConfig{
		LogFile:      log.OutputOff.String(),
		AllowedHosts: []string{"grr.la"},
	}
	cfg.BackendConfig = bcfg

	d := Daemon{Config: cfg}
	err := d.Start()

	if err != nil {
		t.Error("server didn't start")
	} else {

		conn, err := net.Dial("tcp", "127.0.0.1:2525")
		if err != nil {

			return
		}
		in := bufio.NewReader(conn)
		str, err := in.ReadString('\n')
		fmt.Fprint(conn, "HELO host\r\n")
		str, err = in.ReadString('\n')
		// perform 2 transactions
		// both should timeout. The reason why 2 is because we want to make
		// sure that the client waits until processing finishes, and the
		// timeout event is captured.
		for i := 0; i < 2; i++ {
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
			expect := "storage failed"
			if strings.Index(str, expect) == -1 {
				t.Error("Expected the reply to have'", expect, "'but got", str)
			}
		}
		_ = str
		d.Shutdown()
	}

}

// TODO
// - test github issue #44 and #42
