// +build gofuzz

package guerrilla

import (
	"bufio"
	"fmt"
	"net/textproto"
	"sync"

	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/mocks"
)

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
		LogFile:         "/dev/stdout",
	}
	return sc
}

// getMockServerConn gets a new server using sc. Server will be using a mocked TCP connection
// using the dummy backend
// RCP TO command only allows test.com host
func getMockServerConn(sc *ServerConfig) (*mocks.Conn, *server) {

	backend, _ := backends.New("dummy", backends.BackendConfig{"log_received_mails": true})
	server, _ := newServer(sc, backend)
	server.setAllowedHosts([]string{"test.com"})
	conn := mocks.NewConn()
	return conn, server
}

func Fuzz(data []byte) int {
	sc := getMockServerConfig()
	conn, server := getMockServerConn(sc)
	// call the serve.handleClient() func in a goroutine.
	client := NewClient(conn.Server, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		server.handleClient(client)
		wg.Done()
	}()
	// Wait for the greeting from the server
	r := textproto.NewReader(bufio.NewReader(conn.Client))
	line, _ := r.ReadLine()
	fmt.Println(line)
	w := textproto.NewWriter(bufio.NewWriter(conn.Client))

	w.PrintfLine(string(data))

	if _, err := r.ReadLine(); err != nil {
		return 0
	}
	return 1
}
