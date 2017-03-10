package guerrilla

import (
	"testing"

	"bufio"
	"net/textproto"
	"strings"
	"sync"

	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
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
	mainlog, logOpenError = log.GetLogger(sc.LogFile)
	if logOpenError != nil {
		mainlog.WithError(logOpenError).Errorf("Failed creating a logger for mock conn [%s]", sc.ListenInterface)
	}
	backend, err := backends.New("dummy", backends.BackendConfig{"log_received_mails": true}, mainlog)
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
	mainlog, logOpenError = log.GetLogger(sc.LogFile)
	if logOpenError != nil {
		mainlog.WithError(logOpenError).Errorf("Failed creating a logger for mock conn [%s]", sc.ListenInterface)
	}
	conn, server := getMockServerConn(sc, t)
	// call the serve.handleClient() func in a goroutine.
	client := NewClient(conn.Server, 1, mainlog)
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

func TestAllowsHosts(t *testing.T) {
	s := server{}
	allowedHosts := []string{
		"spam4.me",
		"grr.la",
		"newhost.com",
		"example.*",
		"*.test",
		"wild*.card",
		"multiple*wild*cards.*",
	}
	s.setAllowedHosts(allowedHosts)

	testTable := map[string]bool{
		"spam4.me":                true,
		"dont.match":              false,
		"example.com":             true,
		"another.example.com":     false,
		"anything.test":           true,
		"wild.card":               true,
		"wild.card.com":           false,
		"multipleXwildXcards.com": true,
	}

	for host, allows := range testTable {
		if res := s.allowsHost(host); res != allows {
			t.Error(host, ": expected", allows, "but got", res)
		}
	}

	// only wildcard - should match anything
	s.setAllowedHosts([]string{"*"})
	if !s.allowsHost("match.me") {
		t.Error("match.me: expected true but got false")
	}
}

// TODO
// - test github issue #44 and #42
// - test other commands

// also, could test
// - test allowsHost() and allowsHost()
// - test isInTransaction() (make sure it returns true after MAIL command, but false after HELO/EHLO/RSET/end of DATA
// - test to make sure client envelope
// - perhaps anything else that can be tested in server_test.go
