package guerrilla

import (
	"os"
	"testing"

	"bufio"
	"net/textproto"
	"strings"
	"sync"

	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"

	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mocks"
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
	if err := deleteIfExists("rootca.test.pem"); err != nil {
		t.Error(err)
	}
	if err := deleteIfExists("client.test.key"); err != nil {
		t.Error(err)
	}
	if err := deleteIfExists("client.test.pem"); err != nil {
		t.Error(err)
	}
	if err := deleteIfExists("./tests/mail.guerrillamail.com.key.pem"); err != nil {
		t.Error(err)
	}
	if err := deleteIfExists("./tests/mail.guerrillamail.com.cert.pem"); err != nil {
		t.Error(err)
	}
	if err := deleteIfExists("./tests/different-go-guerrilla.pid"); err != nil {
		t.Error(err)
	}
	if err := deleteIfExists("./tests/go-guerrilla.pid"); err != nil {
		t.Error(err)
	}
	if err := deleteIfExists("./tests/go-guerrilla2.pid"); err != nil {
		t.Error(err)
	}
	if err := deleteIfExists("./tests/pidfile.pid"); err != nil {
		t.Error(err)
	}
	if err := deleteIfExists("./tests/pidfile2.pid"); err != nil {
		t.Error(err)
	}

	if err := truncateIfExists("./tests/testlog"); err != nil {
		t.Error(err)
	}
	if err := truncateIfExists("./tests/testlog2"); err != nil {
		t.Error(err)
	}
}

func TestTLSConfig(t *testing.T) {

	defer cleanTestArtifacts(t)
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
	if err := s.configureTLS(); err != nil {
		t.Error(err)
	}

	c := s.tlsConfigStore.Load().(*tls.Config)

	if len(c.CurvePreferences) != 2 {
		t.Error("c.CurvePreferences should have two elements")
	} else if c.CurvePreferences[0] != tls.CurveP521 && c.CurvePreferences[1] != tls.CurveP384 {
		t.Error("c.CurvePreferences curves not setup")
	}
	if !strings.Contains(string(c.RootCAs.Subjects()[0]), "Mountain View") {
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

}

func TestHandleClient(t *testing.T) {
	var mainlog log.Logger
	var logOpenError error
	defer cleanTestArtifacts(t)
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
	if err := w.PrintfLine("HELO test.test.com"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	//fmt.Println(line)
	if err := w.PrintfLine("QUIT"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	//fmt.Println("line is:", line)
	expected := "221 2.0.0 Bye"
	if strings.Index(line, expected) != 0 {
		t.Error("expected", expected, "but got:", line)
	}
	wg.Wait() // wait for handleClient to exit
}

func TestGithubIssue197(t *testing.T) {
	var mainlog log.Logger
	var logOpenError error
	defer cleanTestArtifacts(t)
	sc := getMockServerConfig()
	mainlog, logOpenError = log.GetLogger(sc.LogFile, "debug")
	if logOpenError != nil {
		mainlog.WithError(logOpenError).Errorf("Failed creating a logger for mock conn [%s]", sc.ListenInterface)
	}
	conn, server := getMockServerConn(sc, t)
	server.backend().Start()
	// we assume that 1.1.1.1 is a domain (ip-literal syntax is incorrect)
	// [2001:DB8::FF00:42:8329] is an address literal
	server.setAllowedHosts([]string{"1.1.1.1", "[2001:DB8::FF00:42:8329]"})

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
	if err := w.PrintfLine("HELO test.test.com"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	// Case 1
	if err := w.PrintfLine("rcpt to: <hi@[1.1.1.1]>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	if client.parser.IP == nil {
		t.Error("[1.1.1.1] not parsed as address-liteal")
	}

	// case 2, should be parsed as domain
	if err := w.PrintfLine("rcpt to: <hi@1.1.1.1>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	if client.parser.IP != nil {
		t.Error("1.1.1.1 should not be parsed as an IP (syntax requires IP addresses to be in braces, eg <hi@[1.1.1.1]>")
	}

	// case 3
	// prefix ipv6 is case insensitive
	if err := w.PrintfLine("rcpt to: <hi@[ipv6:2001:DB8::FF00:42:8329]>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	if client.parser.IP == nil {
		t.Error("[ipv6:2001:DB8::FF00:42:8329] should be parsed as an address-literal, it wasnt")
	}

	// case 4
	if err := w.PrintfLine("rcpt to: <hi@[IPv6:2001:0db8:0000:0000:0000:ff00:0042:8329]>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	if client.parser.Domain != "2001:DB8::FF00:42:8329" && client.parser.IP == nil {
		t.Error("[IPv6:2001:0db8:0000:0000:0000:ff00:0042:8329] is same as 2001:DB8::FF00:42:8329, lol")
	}

	if err := w.PrintfLine("QUIT"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	//fmt.Println("line is:", line)
	expected := "221 2.0.0 Bye"
	if strings.Index(line, expected) != 0 {
		t.Error("expected", expected, "but got:", line)
	}
	wg.Wait() // wait for handleClient to exit
}

var githubIssue198data string

var customBackend = func() backends.Decorator {
	return func(p backends.Processor) backends.Processor {
		return backends.ProcessWith(
			func(e *mail.Envelope, task backends.SelectTask) (backends.Result, error) {
				if task == backends.TaskSaveMail {
					githubIssue198data = e.DeliveryHeader + e.Data.String()
				}
				return p.Process(e, task)
			})
	}
}

// TestGithubIssue198 is an interesting test because it shows how to do an integration test for
// a backend using a custom backend.
func TestGithubIssue198(t *testing.T) {
	var mainlog log.Logger
	var logOpenError error
	defer cleanTestArtifacts(t)
	sc := getMockServerConfig()
	mainlog, logOpenError = log.GetLogger(sc.LogFile, "debug")

	backends.Svc.AddProcessor("custom", customBackend)

	if logOpenError != nil {
		mainlog.WithError(logOpenError).Errorf("Failed creating a logger for mock conn [%s]", sc.ListenInterface)
	}
	conn, server := getMockServerConn(sc, t)
	be, err := backends.New(map[string]interface{}{
		"save_process": "HeadersParser|Header|custom", "primary_mail_host": "example.com"},
		mainlog)
	if err != nil {
		t.Error(err)
		return
	}
	server.setBackend(be)
	if err := server.backend().Start(); err != nil {
		t.Error(err)
		return
	}

	server.setAllowedHosts([]string{"1.1.1.1", "[2001:DB8::FF00:42:8329]"})

	client := NewClient(conn.Server, 1, mainlog, mail.NewPool(5))
	client.RemoteIP = "127.0.0.1"

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		server.handleClient(client)
		wg.Done()
	}()
	// Wait for the greeting from the server
	r := textproto.NewReader(bufio.NewReader(conn.Client))
	line, _ := r.ReadLine()

	w := textproto.NewWriter(bufio.NewWriter(conn.Client))
	// Test with HELO greeting
	line = sendMessage("HELO", true, w, t, line, r, err, client)
	if !strings.Contains(githubIssue198data, " SMTPS ") {
		t.Error("'with SMTPS' not present")
	}

	if !strings.Contains(githubIssue198data, "from 127.0.0.1") {
		t.Error("'from 127.0.0.1' not present")
	}

	/////////////////////
	if err := w.PrintfLine("RSET"); err != nil {
		t.Error(err)
	}
	// Test with EHLO
	line, _ = r.ReadLine()
	line = sendMessage("EHLO", true, w, t, line, r, err, client)
	if !strings.Contains(githubIssue198data, " ESMTPS ") {
		t.Error("'with ESMTPS' not present")
	}
	/////////////////////

	if err := w.PrintfLine("RSET"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	// Test with EHLO & no TLS

	line = sendMessage("EHLO", false, w, t, line, r, err, client)

	/////////////////////

	if !strings.Contains(githubIssue198data, " ESMTP ") {
		t.Error("'with ESTMP' not present")
	}

	if err := w.PrintfLine("QUIT"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	expected := "221 2.0.0 Bye"
	if strings.Index(line, expected) != 0 {
		t.Error("expected", expected, "but got:", line)
	}
	wg.Wait() // wait for handleClient to exit
}

func sendMessage(greet string, TLS bool, w *textproto.Writer, t *testing.T, line string, r *textproto.Reader, err error, client *client) string {
	if err := w.PrintfLine(greet + " test.test.com"); err != nil {
		t.Error(err)
	}
	for {
		line, _ = r.ReadLine()
		if strings.Index(line, "250 ") == 0 {
			break
		}
		if strings.Index(line, "250") != 0 {
			t.Error(err)
		}
	}

	if err := w.PrintfLine("MAIL FROM: test@example.com>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	if err := w.PrintfLine("RCPT TO: <hi@[ipv6:2001:DB8::FF00:42:8329]>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	client.Hashes = append(client.Hashes, "abcdef1526777763")
	client.TLS = TLS
	if err := w.PrintfLine("DATA"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	if err := w.PrintfLine("Subject: Test subject\r\n\r\nHello Sir,\nThis is a test.\r\n."); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	return line
}

func TestGithubIssue199(t *testing.T) {
	var mainlog log.Logger
	var logOpenError error
	defer cleanTestArtifacts(t)
	sc := getMockServerConfig()
	mainlog, logOpenError = log.GetLogger(sc.LogFile, "debug")
	if logOpenError != nil {
		mainlog.WithError(logOpenError).Errorf("Failed creating a logger for mock conn [%s]", sc.ListenInterface)
	}
	conn, server := getMockServerConn(sc, t)
	server.backend().Start()

	server.setAllowedHosts([]string{"grr.la", "fake.com", "[1.1.1.1]", "[2001:db8::8a2e:370:7334]", "saggydimes.test.com"})

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
	if err := w.PrintfLine("HELO test"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	// case 1
	if err := w.PrintfLine(
		"MAIL FROM: <\"  yo-- man wazz'''up? surprise surprise, this is POSSIBLE@fake.com \"@example.com>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	// [SPACE][SPACE]yo--[SPACE]man[SPACE]wazz'''up?[SPACE]surprise[SPACE]surprise,[SPACE]this[SPACE]is[SPACE]POSSIBLE@fake.com[SPACE]
	if client.parser.LocalPart != "  yo-- man wazz'''up? surprise surprise, this is POSSIBLE@fake.com " {
		t.Error("expecting local part: [  yo-- man wazz'''up? surprise surprise, this is POSSIBLE@fake.com ], got client.parser.LocalPart")
	}
	if !client.parser.LocalPartQuotes {
		t.Error("was expecting client.parser.LocalPartQuotes true, got false")
	}
	// from should just as above but without angle brackets <>
	if from := client.MailFrom.String(); from != "\"  yo-- man wazz'''up? surprise surprise, this is POSSIBLE@fake.com \"@example.com" {
		t.Error("mail from was:", from)
	}
	if line != "250 2.1.0 OK" {
		t.Error("line did not have: 250 2.1.0 OK, got", line)
	}

	if err := w.PrintfLine("RSET"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	// case 2, address literal mailboxes
	if err := w.PrintfLine("MAIL FROM: <hi@[1.1.1.1]>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	// stringer should be aware its an ip and return the host part in angle brackets
	if from := client.MailFrom.String(); from != "hi@[1.1.1.1]" {
		t.Error("mail from was:", from)
	}

	if err := w.PrintfLine("RSET"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	// case 3

	if err := w.PrintfLine("MAIL FROM: <hi@[IPv6:2001:0db8:0000:0000:0000:8a2e:0370:7334]>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	// stringer should be aware its an ip and return the host part in angle brackets, and ipv6 should be normalized
	if from := client.MailFrom.String(); from != "hi@[2001:db8::8a2e:370:7334]" {
		t.Error("mail from was:", from)
	}

	if err := w.PrintfLine("RSET"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	// case 4
	// rcpt to: <hi@[IPv6:2001:0db8:0000:0000:0000:ff00:0042:8329]>

	if err := w.PrintfLine("MAIL FROM: <>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	if err := w.PrintfLine("RCPT TO: <Postmaster>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	// stringer should return an empty string
	if from := client.MailFrom.String(); from != "" {
		t.Error("mail from was:", from)
	}

	// note here the saggydimes.test.com was added because no host was specified in the RCPT TO command
	if rcpt := client.RcptTo[0].String(); rcpt != "postmaster@saggydimes.test.com" {
		t.Error("mail from was:", rcpt)
	}

	// additional cases

	/*
		user part:
		" al\ph\a "@grr.la should be " alpha "@grr.la.
		"alpha"@grr.la should be alpha@grr.la.
		"alp\h\a"@grr.la should be alpha@grr.la.
	*/

	if err := w.PrintfLine("RSET"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	if err := w.PrintfLine("RCPT TO: <\" al\\ph\\a \"@grr.la>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	if client.RcptTo[0].User != " alpha " {
		t.Error(client.RcptTo[0].User)
	}

	// the unnecessary \\ should be removed
	if rcpt := client.RcptTo[0].String(); rcpt != "\" alpha \"@grr.la" {
		t.Error(rcpt)
	}

	if err := w.PrintfLine("RSET"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	if err := w.PrintfLine("RCPT TO: <\"alpha\"@grr.la>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	// we don't need to quote, so stringer should return without the quotes
	if rcpt := client.RcptTo[0].String(); rcpt != "alpha@grr.la" {
		t.Error(rcpt)
	}

	if err := w.PrintfLine("RSET"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	if err := w.PrintfLine("RCPT TO: <\"a\\l\\pha\"@grr.la>"); err != nil {
		t.Error(err)
	}

	line, _ = r.ReadLine()

	// we don't need to quote, so stringer should return without the quotes
	if rcpt := client.RcptTo[0].String(); rcpt != "alpha@grr.la" {
		t.Error(rcpt)
	}

	if err := w.PrintfLine("QUIT"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	wg.Wait() // wait for handleClient to exit
}

func TestGithubIssue200(t *testing.T) {
	var mainlog log.Logger
	var logOpenError error
	defer cleanTestArtifacts(t)
	sc := getMockServerConfig()
	mainlog, logOpenError = log.GetLogger(sc.LogFile, "debug")
	if logOpenError != nil {
		mainlog.WithError(logOpenError).Errorf("Failed creating a logger for mock conn [%s]", sc.ListenInterface)
	}
	conn, server := getMockServerConn(sc, t)
	server.backend().Start()
	server.setAllowedHosts([]string{"1.1.1.1", "[2001:DB8::FF00:42:8329]"})

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
	if err := w.PrintfLine("HELO test\"><script>alert('hi')</script>test.com"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	if line != "550 5.5.2 Syntax error" {
		t.Error("line expected to be: 550 5.5.2 Syntax error, got", line)
	}

	if err := w.PrintfLine("HELO test.com"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	if !strings.Contains(line, "250") {
		t.Error("line did not have 250 code, got", line)
	}
	if err := w.PrintfLine("QUIT"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	//fmt.Println("line is:", line)
	expected := "221 2.0.0 Bye"
	if strings.Index(line, expected) != 0 {
		t.Error("expected", expected, "but got:", line)
	}
	wg.Wait() // wait for handleClient to exit
}

func TestGithubIssue201(t *testing.T) {
	var mainlog log.Logger
	var logOpenError error
	defer cleanTestArtifacts(t)
	sc := getMockServerConfig()
	mainlog, logOpenError = log.GetLogger(sc.LogFile, "debug")
	if logOpenError != nil {
		mainlog.WithError(logOpenError).Errorf("Failed creating a logger for mock conn [%s]", sc.ListenInterface)
	}
	conn, server := getMockServerConn(sc, t)
	server.backend().Start()
	// note that saggydimes.test.com is the hostname of the server, it comes form the config
	// it will be used for rcpt to:<postmaster> which does not specify a host
	server.setAllowedHosts([]string{"a.com", "saggydimes.test.com"})

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
	if err := w.PrintfLine("HELO test"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	// case 1
	if err := w.PrintfLine("RCPT TO: <postMaStER@a.com>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	if line != "250 2.1.5 OK" {
		t.Error("line did not have: 250 2.1.5 OK, got", line)
	}
	// case 2
	if err := w.PrintfLine("RCPT TO: <Postmaster@not-a.com>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	if line != "454 4.1.1 Error: Relay access denied: not-a.com" {
		t.Error("line is not:454 4.1.1 Error: Relay access denied: not-a.com, got", line)
	}
	// case 3 (no host specified)

	if err := w.PrintfLine("RCPT TO: <poSTmAsteR>"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	if line != "250 2.1.5 OK" {
		t.Error("line is not:[250 2.1.5 OK], got", line)
	}

	// case 4
	if err := w.PrintfLine("RCPT TO: <\"po\\ST\\mAs\\t\\eR\">"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	if line != "250 2.1.5 OK" {
		t.Error("line is not:[250 2.1.5 OK], got", line)
	}
	// the local part should be just "postmaster" (normalized)
	if client.parser.LocalPart != "postmaster" {
		t.Error("client.parser.LocalPart was not postmaster, got:", client.parser.LocalPart)
	}

	if client.parser.LocalPartQuotes {
		t.Error("client.parser.LocalPartQuotes was true, expecting false")
	}

	if err := w.PrintfLine("QUIT"); err != nil {
		t.Error(err)
	}
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
	defer cleanTestArtifacts(t)
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
	if err := w.PrintfLine("HELO test.test.com"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	//fmt.Println(line)
	if err := w.PrintfLine("XCLIENT ADDR=212.96.64.216 NAME=[UNAVAILABLE]"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	if client.RemoteIP != "212.96.64.216" {
		t.Error("client.RemoteIP should be 212.96.64.216, but got:", client.RemoteIP)
	}
	expected := "250 2.1.0 OK"
	if strings.Index(line, expected) != 0 {
		t.Error("expected", expected, "but got:", line)
	}

	// try malformed input
	if err := w.PrintfLine("XCLIENT c"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()

	expected = "250 2.1.0 OK"
	if strings.Index(line, expected) != 0 {
		t.Error("expected", expected, "but got:", line)
	}

	if err := w.PrintfLine("QUIT"); err != nil {
		t.Error(err)
	}
	line, _ = r.ReadLine()
	wg.Wait() // wait for handleClient to exit
}

// The backend gateway should time out after 1 second because it sleeps for 2 sec.
// The transaction should wait until finished, and then test to see if we can do
// a second transaction
func TestGatewayTimeout(t *testing.T) {
	defer cleanTestArtifacts(t)
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
		if err != nil {
			t.Error(err)
		}
		if _, err := fmt.Fprint(conn, "HELO host\r\n"); err != nil {
			t.Error(err)
		}
		str, err = in.ReadString('\n')
		// perform 2 transactions
		// both should panic.
		for i := 0; i < 2; i++ {
			if _, err := fmt.Fprint(conn, "MAIL FROM:<test@example.com>r\r\n"); err != nil {
				t.Error(err)
			}
			if str, err = in.ReadString('\n'); err != nil {
				t.Error(err)
			}
			if _, err := fmt.Fprint(conn, "RCPT TO:<test@grr.la>\r\n"); err != nil {
				t.Error(err)
			}
			if str, err = in.ReadString('\n'); err != nil {
				t.Error(err)
			}
			if _, err := fmt.Fprint(conn, "DATA\r\n"); err != nil {
				t.Error(err)
			}
			if str, err = in.ReadString('\n'); err != nil {
				t.Error(err)
			}
			if _, err := fmt.Fprint(conn, "Subject: Test subject\r\n"); err != nil {
				t.Error(err)
			}
			if _, err := fmt.Fprint(conn, "\r\n"); err != nil {
				t.Error(err)
			}
			if _, err := fmt.Fprint(conn, "A an email body\r\n"); err != nil {
				t.Error(err)
			}
			if _, err := fmt.Fprint(conn, ".\r\n"); err != nil {
				t.Error(err)
			}
			str, err = in.ReadString('\n')
			expect := "transaction timeout"
			if err != nil {
				t.Error(err)
			} else if !strings.Contains(str, expect) {
				t.Error("Expected the reply to have'", expect, "'but got", str)
			}
		}
		_ = str

		d.Shutdown()
	}
}

// The processor will panic and gateway should recover from it
func TestGatewayPanic(t *testing.T) {
	defer cleanTestArtifacts(t)
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
		if _, err := in.ReadString('\n'); err != nil {
			t.Error(err)
		}
		if _, err := fmt.Fprint(conn, "HELO host\r\n"); err != nil {
			t.Error(err)
		}
		if _, err = in.ReadString('\n'); err != nil {
			t.Error(err)
		}
		// perform 2 transactions
		// both should timeout. The reason why 2 is because we want to make
		// sure that the client waits until processing finishes, and the
		// timeout event is captured.
		for i := 0; i < 2; i++ {
			if _, err := fmt.Fprint(conn, "MAIL FROM:<test@example.com>r\r\n"); err != nil {
				t.Error(err)
			}
			if _, err = in.ReadString('\n'); err != nil {
				t.Error(err)
			}
			if _, err := fmt.Fprint(conn, "RCPT TO:<test@grr.la>\r\n"); err != nil {
				t.Error(err)
			}
			if _, err = in.ReadString('\n'); err != nil {
				t.Error(err)
			}
			if _, err := fmt.Fprint(conn, "DATA\r\n"); err != nil {
				t.Error(err)
			}
			if _, err = in.ReadString('\n'); err != nil {
				t.Error(err)
			}
			if _, err := fmt.Fprint(conn, "Subject: Test subject\r\n"); err != nil {
				t.Error(err)
			}
			if _, err := fmt.Fprint(conn, "\r\n"); err != nil {
				t.Error(err)
			}
			if _, err := fmt.Fprint(conn, "A an email body\r\n"); err != nil {
				t.Error(err)
			}
			if _, err := fmt.Fprint(conn, ".\r\n"); err != nil {
				t.Error(err)
			}
			if str, err := in.ReadString('\n'); err != nil {
				t.Error(err)
			} else {
				expect := "storage failed"
				if !strings.Contains(str, expect) {
					t.Error("Expected the reply to have'", expect, "'but got", str)
				}
			}
		}
		d.Shutdown()
	}

}

func TestAllowsHosts(t *testing.T) {
	defer cleanTestArtifacts(t)
	s := server{}
	allowedHosts := []string{
		"spam4.me",
		"grr.la",
		"newhost.com",
		"example.*",
		"*.test",
		"wild*.card",
		"multiple*wild*cards.*",
		"[::FFFF:C0A8:1]",          // ip4 in ipv6 format. It's actually 192.168.0.1
		"[2001:db8::ff00:42:8329]", // same as 2001:0db8:0000:0000:0000:ff00:0042:8329
		"[127.0.0.1]",
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

	testTableIP := map[string]bool{

		"192.168.0.1": true,
		"2001:0db8:0000:0000:0000:ff00:0042:8329": true,
		"127.0.0.1": true,
	}

	for host, allows := range testTableIP {
		if res := s.allowsIp(net.ParseIP(host)); res != allows {
			t.Error(host, ": expected", allows, "but got", res)
		}
	}

	// only wildcard - should match anything
	s.setAllowedHosts([]string{"*"})
	if !s.allowsHost("match.me") {
		t.Error("match.me: expected true but got false")
	}

	// turns off
	s.setAllowedHosts([]string{"."})
	if !s.allowsHost("match.me") {
		t.Error("match.me: expected true but got false")
	}

	// no wilcards
	s.setAllowedHosts([]string{"grr.la", "example.com"})

}
