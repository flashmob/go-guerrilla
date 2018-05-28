package guerrilla

import (
	"testing"

	"bufio"
	"net/textproto"
	"strings"
	"sync"

	"crypto/tls"
	"fmt"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mocks"
	"io/ioutil"
	"net"
	"os"
)

// getMockServerConfig gets a mock ServerConfig struct used for creating a new server
func getMockServerConfig() *ServerConfig {
	sc := &ServerConfig{
		IsEnabled: true, // not tested here
		Hostname:  "saggydimes.test.com",
		MaxSize:   1024, // smtp message max size
		TLS: ServerTLSConfig{
			PrivateKeyFile: "./tests/mail.guerrillamail.com.key.pem",
			PublicKeyFile:  "./tests/mail.guerrillamail.com.cert.pem",
			StartTLSOn:     true,
			AlwaysOn:       false,
		},
		Timeout:         5,
		ListenInterface: "127.0.0.1:2529",
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

// test the RootCAs tls config setting
var rootCAPK = `-----BEGIN CERTIFICATE-----
MIIDqjCCApKgAwIBAgIJALh2TrsBR5MiMA0GCSqGSIb3DQEBCwUAMGkxCzAJBgNV
BAYTAlVTMQswCQYDVQQIDAJDQTEWMBQGA1UEBwwNTW91bnRhaW4gVmlldzEhMB8G
A1UECgwYSW50ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMRIwEAYDVQQDDAlsb2NhbGhv
c3QwIBcNMTgwNTE4MDYzOTU2WhgPMjExODA0MjQwNjM5NTZaMGkxCzAJBgNVBAYT
AlVTMQswCQYDVQQIDAJDQTEWMBQGA1UEBwwNTW91bnRhaW4gVmlldzEhMB8GA1UE
CgwYSW50ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMRIwEAYDVQQDDAlsb2NhbGhvc3Qw
ggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDCcb0ulYT1o5ysor5UtWYW
q/ZY3PyK3/4YBZq5JoX4xk7GNQQ+3p/Km7QPoBXfgjFLZXEV2R0bE5hHMXfLa5Xb
64acb9VqCqDvPFXcaNP4rEdBKDVN2p0PEi917tcKBSrZn5Yl+iOhtcBpQDvhHgn/
9MdmIAKB3+yK+4l9YhT40XfDXCQqzfg4XcNaEgTzZHcDJz+KjWJuJChprcx27MTI
Ndxs9nmFA2rK16rjgjtwjZ4t9dXsljdOcx59s6dIQ0GnEM8qdKxi/vEx4+M/hbGf
v7H75LsuKRrVJINAmfy9fmc6VAXjFU0ZVxGK5eVnzsh/hY08TSSrlCCKAJpksjJz
AgMBAAGjUzBRMB0GA1UdDgQWBBSZsYWs+8FYe4z4c6LLmFB4TeeV/jAfBgNVHSME
GDAWgBSZsYWs+8FYe4z4c6LLmFB4TeeV/jAPBgNVHRMBAf8EBTADAQH/MA0GCSqG
SIb3DQEBCwUAA4IBAQAcXt/FaILkOCMj8bTUx42vi2N9ZTiEuRbYi24IyGOokbDR
pSsIxiz+HDPUuX6/X/mHidl24aS9wdv5JTXMr44/BeGK1WC7gMueZBxAqONpaG1Q
VU0e3q1YwXKcupKQ7kVWl0fuY3licv0+s4zBcTLKkmWAYqsb/n0KtCMyqewi+Rqa
Zj5Z3OcWOq9Ad9fZWKcG8k/sgeTk9z0X1mZcEyWWxqsUmxvN+SdWLoug1xJVVbMN
CipZ0vBIi9KOhQgzuIFhoTcd6myUtov52/EFqlX6UuFpY2gEWw/f/yu+SI08v4w9
KwxgAKBkhx2JYZKtu1EsPIMDyS0aahcDnHqnrGAi
-----END CERTIFICATE-----`

var clientPrvKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA5ZLmMBdKkVyVmN0VhDSFGvgKp24ejHPCv+wfuf3vlU9cwKfH
R3vejleZAVRcidscfA0Jsub/Glsr0XwecagtpvTI+Fp1ik6sICOz+VW3958qaAi8
TjbUMjcDHJeSLcjr725CH5uIvhRzR+daYaJQhAcL2MEt8M9WIF6AjtDZEH9R6oM8
t5FkO0amImlnipYXNBFghmzkZzfGXXRQLw2A+u6keLcjCrn9h2BaofGIjQfYcu/3
fH4cIFR4z/soGKameqnCUz7dWmbf4tAI+8QR0VXXBKhiHDm98tPSeH994hC52Uul
rjEVcM5Uox5hazS2PK06oSc1YuFZONqeeGqj6wIDAQABAoIBADERzRHKaK3ZVEBw
QQEZGLpC+kP/TZhHxgCvv7hJhsQrSnADbJzi5RcXsiSOm5j7tILvZntO1IgVpLAK
D5fLkrZ069/pteXyGuhjuTw6DjBnXPEPrPAq2ABDse6SlzQiFgv/TTLkU74NMPbV
hIQJ5ZvSxb12zRMDviz9Bg2ApmTX6k2iPjQBnEHgKzb64IdMcEb5HE1qNt0v0lRA
sGBMZZKQWbt2m0pSbAbnB3S9GcpJkRgFFMdTaUScIWO6ICT2hBP2pw2/4M2Zrmlt
bsyWu9uswBzhvu+/pg2E66V6mji0EzDMlXqjlO5jro6t7P33t1zkd/i/ykKmtDLp
IpR94UECgYEA9Y4EIjOyaBWJ6TRQ6a/tehGPbwIOgvEiTYXRJqdU49qn/i4YZjSm
F4iibJz+JeOIQXSwa9F7gRlaspIuHgIJoer7BrITMuhr+afqMLkxK0pijul/qAbm
HdpFn8IxjpNu4/GoAENbEVy50SMST9yWh5ulEkHHftd4/NJKoJQ2PZ8CgYEA71bb
lFVh1MFclxRKECmpyoqUAzwGlMoHJy/jaBYuWG4X7rzxqDRrgPH3as6gXpRiSZ+K
5fC+wcU7dKnHtJOkBDk6J5ev2+hbwg+yq3w4+l3bPDvf2TJyXjXjRDZo12pxFD58
ybCOF6ItbIDXqT5pvo3PMjgMwu1Ycie+h6hA3jUCgYEAsq93XpQT/R2/T44cWxEE
VFG2+GacvLhP5+26ttAJPA1/Nb3BT458Vp+84iCT6GpcWpVZU/wKTXVvxIYPPRLq
g4MEzGiFBASRngiMqIv6ta/ZbHmJxXHPvmV5SLn9aezrQsA1KovZFxdMuF03FBpH
B8NBKbnoO+r8Ra2ZVKTFm60CgYAZw8Dpi/N3IsWj4eRDLyj/C8H5Qyn2NHVmq4oQ
d2rPzDI5Wg+tqs7z15hp4Ap1hAW8pTcfn7X5SBEpculzr/0VE1AGWRbuVmoiTuxN
95ZupVHnfw6O5BZZu/VWL4FDx0qbAksOrznso2G+b3RH3NcnUz69yjjddw1xZIPn
OJ6bDQKBgDUcWYu/2amU18D5vJpppUgRq2084WPUeXsaniTbmWfOC8NAn8CKLY0N
V4yGSu98apDuqEVqL0VFQEgqK+5KTvRdXXYi36XYRbbVUgV13xveq2YTvjNbPM60
QWG9YmgH7hVYGusuh5nQeS0qiIpwyws2H5mBVrGXrQ1Xb0MLWj8/
-----END RSA PRIVATE KEY-----`

// signed using the Root (rootCAPK)
var clientPubKey = `-----BEGIN CERTIFICATE-----
MIIDWDCCAkACCQCHoh4OvUySOzANBgkqhkiG9w0BAQsFADBpMQswCQYDVQQGEwJV
UzELMAkGA1UECAwCQ0ExFjAUBgNVBAcMDU1vdW50YWluIFZpZXcxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDESMBAGA1UEAwwJbG9jYWxob3N0MCAX
DTE4MDUxODA2NDQ0NVoYDzMwMTcwOTE4MDY0NDQ1WjBxMQswCQYDVQQGEwJVUzET
MBEGA1UECAwKQ2FsaWZvcm5pYTEWMBQGA1UEBwwNTW91bnRhaW4gVmlldzEhMB8G
A1UECgwYSW50ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMRIwEAYDVQQDDAlsb2NhbGhv
c3QwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDlkuYwF0qRXJWY3RWE
NIUa+Aqnbh6Mc8K/7B+5/e+VT1zAp8dHe96OV5kBVFyJ2xx8DQmy5v8aWyvRfB5x
qC2m9Mj4WnWKTqwgI7P5Vbf3nypoCLxONtQyNwMcl5ItyOvvbkIfm4i+FHNH51ph
olCEBwvYwS3wz1YgXoCO0NkQf1Hqgzy3kWQ7RqYiaWeKlhc0EWCGbORnN8ZddFAv
DYD67qR4tyMKuf2HYFqh8YiNB9hy7/d8fhwgVHjP+ygYpqZ6qcJTPt1aZt/i0Aj7
xBHRVdcEqGIcOb3y09J4f33iELnZS6WuMRVwzlSjHmFrNLY8rTqhJzVi4Vk42p54
aqPrAgMBAAEwDQYJKoZIhvcNAQELBQADggEBAIQmlo8iCpyYggkbpfDmThBPHfy1
cZcCi/tRFoFe1ccwn2ezLMIKmW38ZebiroawwqrZgU6AP+dMxVKLMjpyLPSrpFKa
3o/LbVF7qMfH8/y2q8t7javd6rxoENH9uxLyHhauzI1iWy0whoDWBNiZrPBTBCjq
jDGZARZqGyrPeXi+RNe1cMvZCxAFy7gqEtWFLWWrp0gYNPvxkHhhQBrUcF+8T/Nf
9G4hKZSN/KAgC0CNBVuNrdyNc3l8H66BfwwL5X0+pesBYZM+MEfmBZOo+p7OWx2r
ug8tR8eSL1vGleONtFRBUVG7NbtjhBf9FhvPZcSRR10od/vWHku9E01i4xg=
-----END CERTIFICATE-----`

func TestTLSConfig(t *testing.T) {

	if err := ioutil.WriteFile("rootca.test.pem", []byte(rootCAPK), 0644); err != nil {
		t.Fatal("couldn't create rootca.test.pem file.", err)
		return
	}
	if err := ioutil.WriteFile("client.test.key", []byte(clientPrvKey), 0644); err != nil {
		t.Fatal("couldn't create client.test.key file.", err)
		return
	}
	if err := ioutil.WriteFile("client.test.pem", []byte(clientPubKey), 0644); err != nil {
		t.Fatal("couldn't create client.test.pem file.", err)
		return
	}

	s := server{}
	s.setConfig(&ServerConfig{
		TLS: ServerTLSConfig{
			StartTLSOn:     true,
			PrivateKeyFile: "client.test.key",
			PublicKeyFile:  "client.test.pem",
			RootCAs:        "rootca.test.pem",
			ClientAuthType: "NoClientCert",
			Curves:         []string{"P521", "P384"},
			Ciphers:        []string{"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384", "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA"},
			Protocols:      []string{"tls1.0", "tls1.2"},
		},
	})
	s.configureSSL()

	c := s.tlsConfigStore.Load().(*tls.Config)

	if len(c.CurvePreferences) != 2 {
		t.Error("c.CurvePreferences should have two elements")
	} else if c.CurvePreferences[0] != tls.CurveP521 && c.CurvePreferences[1] != tls.CurveP384 {
		t.Error("c.CurvePreferences curves not setup")
	}
	if strings.Index(string(c.RootCAs.Subjects()[0]), "Mountain View") == -1 {
		t.Error("c.RootCAs not correctly set")
	}
	if c.ClientAuth != tls.NoClientCert {
		t.Error("c.ClientAuth should be tls.NoClientCert")
	}

	if len(c.CipherSuites) != 2 {
		t.Error("c.CipherSuites length should be 2")
	}

	if c.CipherSuites[0] != tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384 && c.CipherSuites[1] != tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA {
		t.Error("c.CipherSuites not correctly set ")
	}

	if c.MinVersion != tls.VersionTLS10 {
		t.Error("c.MinVersion should be tls.VersionTLS10")
	}

	if c.MaxVersion != tls.VersionTLS12 {
		t.Error("c.MinVersion should be tls.VersionTLS10")
	}

	if c.PreferServerCipherSuites != false {
		t.Error("PreferServerCipherSuites should be false")
	}

	os.Remove("rootca.test.pem")
	os.Remove("client.test.key")
	os.Remove("client.test.pem")

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
