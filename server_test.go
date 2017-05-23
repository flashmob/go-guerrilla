package guerrilla

import (
	"testing"

	"bufio"
	"net/textproto"
	"strings"
	"sync"

	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mocks"
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
		server.setAllowedHosts([]string{"test.com"}, []string{})
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

// TODO
// - test github issue #44 and #42
