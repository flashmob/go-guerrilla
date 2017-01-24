package guerrilla

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"runtime"

	log "github.com/Sirupsen/logrus"

	"sync"
)

const (
	CommandVerbMaxLength = 16
	CommandLineMaxLength = 1024
	// Number of allowed unrecognized commands before we terminate the connection
	MaxUnrecognizedCommands = 5
	// The maximum total length of a reverse-path or forward-path is 256
	RFC2821LimitPath = 256
	// The maximum total length of a user name or other local-part is 64
	RFC2832LimitLocalPart = 64
	//The maximum total length of a domain name or number is 255
	RFC2821LimitDomain = 255
	// The minimum total number of recipients that must be buffered is 100
	RFC2821LimitRecipients = 100
)

// Server listens for SMTP clients on the port specified in its config
type server struct {
	config         *ServerConfig
	backend        Backend
	tlsConfig      *tls.Config
	maxSize        int64
	timeout        time.Duration
	clientPool     *Pool
	wg             sync.WaitGroup // for waiting to shutdown
	listener       net.Listener
	closedListener chan (bool)
}

// Creates and returns a new ready-to-run Server from a configuration
func newServer(sc ServerConfig, b *Backend) (*server, error) {
	server := &server{
		config:         &sc,
		backend:        *b,
		maxSize:        sc.MaxSize,
		timeout:        time.Duration(sc.Timeout),
		clientPool:     NewPool(sc.MaxClients),
		closedListener: make(chan (bool), 1),
	}
	if server.config.TLSAlwaysOn || server.config.StartTLSOn {
		cert, err := tls.LoadX509KeyPair(server.config.PublicKeyFile, server.config.PrivateKeyFile)
		if err != nil {
			return nil, fmt.Errorf("Error loading TLS certificate: %s", err.Error())
		}
		server.tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.VerifyClientCertIfGiven,
			ServerName:   server.config.Hostname,
			Rand:         rand.Reader,
		}
	}
	return server, nil
}

// Begin accepting SMTP clients
func (server *server) Start(startWG *sync.WaitGroup) error {
	var clientID uint64
	clientID = 0

	listener, err := net.Listen("tcp", server.config.ListenInterface)
	server.listener = listener
	if err != nil {
		return fmt.Errorf("[%s] Cannot listen on port: %s ", server.config.ListenInterface, err.Error())
	}

	log.Infof("Listening on TCP %s", server.config.ListenInterface)
	startWG.Done() // start successful

	for {
		log.Debugf("[%s] Waiting for a new client. Next Client ID: %d", server.config.ListenInterface, clientID+1)
		conn, err := listener.Accept()
		clientID++
		if err != nil {
			if e, ok := err.(net.Error); ok && !e.Temporary() {
				log.Infof("Server [%s] has stopped accepting new clients", server.config.ListenInterface)
				// the listener has been closed, wait for clients to exit
				log.Infof("shutting down pool [%s]", server.config.ListenInterface)
				server.clientPool.ShutdownWait()
				server.closedListener <- true
				return nil
			}
			log.WithError(err).Info("Temporary error accepting client")
			continue
		}
		go func(p Poolable, borrow_err error) {
			c := p.(*client)
			if borrow_err == nil {
				server.handleClient(c)
				server.clientPool.Return(c)
			} else {
				log.WithError(borrow_err).Info("couldn't borrow a new client")
				// we could not get a client, so close the connection.
				conn.Close()

			}
			// intentionally placed Borrow in args so that it's called in the
			// same main goroutine.
		}(server.clientPool.Borrow(conn, clientID))

	}
}

func (server *server) Shutdown() {
	server.clientPool.ShutdownState()
	if server.listener != nil {
		server.listener.Close()
		// wait for the listener to close.
		<-server.closedListener
		// At this point Start will exit and close down the pool
	} else {
		// listener already closed, wait for clients to exit
		server.clientPool.ShutdownWait()
	}
}

func (server *server) GetActiveClientsCount() int {
	return server.clientPool.GetActiveClientsCount()
}

// Verifies that the host is a valid recipient.
func (server *server) allowsHost(host string) bool {
	for _, allowed := range server.config.AllowedHosts {
		if host == allowed {
			return true
		}
	}
	return false
}

// Upgrades a client connection to TLS
func (server *server) upgradeToTLS(client *client) bool {
	tlsConn := tls.Server(client.conn, server.tlsConfig)
	err := tlsConn.Handshake()
	if err != nil {
		log.WithError(err).Warn("[%s] Failed TLS handshake", client.RemoteAddress)
		return false
	}
	client.conn = net.Conn(tlsConn)
	client.bufout.Reset(client.conn)
	client.bufin.Reset(client.conn)
	client.TLS = true

	return true
}

// Closes a client connection
func (server *server) closeConn(client *client) {
	client.conn.Close()
	client.conn = nil
	log.WithFields(map[string]interface{}{
		"event":   "disconnect",
		"address": client.RemoteAddress,
		"helo":    client.Helo,
		"id":      client.ID,
	}).Info("Close client")
}

// Reads from the client until a terminating sequence is encountered,
// or until a timeout occurs.
func (server *server) read(client *client) (string, error) {
	var input, reply string
	var err error

	// In command state, stop reading at line breaks
	suffix := "\r\n"
	if client.state == ClientData {
		// In data state, stop reading at solo periods
		suffix = "\r\n.\r\n"
	}

	for {
		client.setTimeout(server.timeout)
		reply, err = client.bufin.ReadString('\n')
		input = input + reply
		if err == nil && client.state == ClientData {
			if reply != "" {
				// Extract the subject while we're at it
				client.scanSubject(reply)
			}
			if int64(len(input)) > server.config.MaxSize {
				return input, fmt.Errorf("Maximum DATA size exceeded (%d)", server.config.MaxSize)
			}
		}
		if err != nil {
			break
		}
		if strings.HasSuffix(input, suffix) {
			break
		}
	}
	return input, err
}

// Writes a response to the client.
func (server *server) writeResponse(client *client) error {
	client.setTimeout(server.timeout)
	size, err := client.bufout.WriteString(client.response)
	if err != nil {
		return err
	}
	err = client.bufout.Flush()
	if err != nil {
		return err
	}
	client.response = client.response[size:]
	return nil
}

func (server *server) isShuttingDown() bool {
	return server.clientPool.IsShuttingDown()
}

// Handles an entire client SMTP exchange
func (server *server) handleClient(client *client) {
	defer server.closeConn(client)
	log.WithFields(map[string]interface{}{
		"event":   "connect",
		"address": client.RemoteAddress,
		"helo":    client.Helo,
		"id":      client.ID,
	}).Info("Handle client")

	// Initial greeting
	greeting := fmt.Sprintf("220 %s SMTP Guerrilla(%s) #%d (%d) %s gr:%d",
		server.config.Hostname, Version, client.ID,
		server.clientPool.GetActiveClientsCount(), time.Now().Format(time.RFC3339), runtime.NumGoroutine())

	helo := fmt.Sprintf("250 %s Hello", server.config.Hostname)
	ehlo := fmt.Sprintf("250-%s Hello", server.config.Hostname)

	// Extended feature advertisements
	messageSize := fmt.Sprintf("250-SIZE %d\r\n", server.config.MaxSize)
	pipelining := "250-PIPELINING\r\n"
	advertiseTLS := "250-STARTTLS\r\n"
	help := "250 HELP"

	if server.config.TLSAlwaysOn {
		success := server.upgradeToTLS(client)
		if !success {
			client.kill()
		}
		advertiseTLS = ""
	}
	if !server.config.StartTLSOn {
		// STARTTLS turned off, don't advertise it
		advertiseTLS = ""
	}

	for client.isAlive() {
		switch client.state {
		case ClientGreeting:
			client.responseAdd(greeting)
			client.state = ClientCmd
		case ClientCmd:
			client.bufin.setLimit(CommandLineMaxLength)
			input, err := server.read(client)
			log.Debugf("Client sent: %s", input)
			if err == io.EOF {
				log.WithError(err).Warnf("Client closed the connection: %s", client.RemoteAddress)
				return
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.WithError(err).Warnf("Timeout: %s", client.RemoteAddress)
				return
			} else if err == LineLimitExceeded {
				client.responseAdd("500 Line too long.")
				client.kill()
				break
			} else if err != nil {
				log.WithError(err).Warnf("Read error: %s", client.RemoteAddress)
				client.kill()
				break
			}
			if server.isShuttingDown() {
				client.state = ClientShutdown
				continue
			}

			input = strings.Trim(input, " \r\n")
			cmdLen := len(input)
			if cmdLen > CommandVerbMaxLength {
				cmdLen = CommandVerbMaxLength
			}
			cmd := strings.ToUpper(input[:cmdLen])

			switch {
			case strings.Index(cmd, "HELO") == 0:
				client.Helo = strings.Trim(input[4:], " ")
				client.resetTransaction()
				client.responseAdd(helo)

			case strings.Index(cmd, "EHLO") == 0:
				client.Helo = strings.Trim(input[4:], " ")
				client.resetTransaction()
				client.responseAdd(ehlo + messageSize + pipelining + advertiseTLS + help)

			case strings.Index(cmd, "HELP") == 0:
				client.responseAdd("214 OK\r\n" + messageSize + pipelining + advertiseTLS + help)

			case strings.Index(cmd, "MAIL FROM:") == 0:
				if client.isInTransaction() {
					client.responseAdd("503 Error: nested MAIL command")
					break
				}
				from, err := extractEmail(input[10:])
				if err != nil {
					client.responseAdd(err.Error())
				} else {
					client.MailFrom = from
					client.responseAdd("250 OK")
				}

			case strings.Index(cmd, "RCPT TO:") == 0:
				if len(client.RcptTo) > RFC2821LimitRecipients {
					client.responseAdd("452 Too many recipients")
					break
				}
				to, err := extractEmail(input[8:])
				if err != nil {
					client.responseAdd(err.Error())
				} else {
					if !server.allowsHost(to.Host) {
						client.responseAdd("454 Error: Relay access denied: " + to.Host)
					} else {
						client.RcptTo = append(client.RcptTo, *to)
						client.responseAdd("250 OK")
					}
				}

			case strings.Index(cmd, "RSET") == 0:
				client.resetTransaction()
				client.responseAdd("250 OK")

			case strings.Index(cmd, "VRFY") == 0:
				client.responseAdd("252 Cannot verify user")

			case strings.Index(cmd, "NOOP") == 0:
				client.responseAdd("250 OK")

			case strings.Index(cmd, "QUIT") == 0:
				client.responseAdd("221 Bye")
				client.kill()

			case strings.Index(cmd, "DATA") == 0:
				if client.MailFrom.isEmpty() {
					client.responseAdd("503 Error: No sender")
					break
				}
				if len(client.RcptTo) == 0 {
					client.responseAdd("503 Error: No recipients")
					break
				}
				client.responseAdd("354 Enter message, ending with '.' on a line by itself")
				client.state = ClientData

			case server.config.StartTLSOn && strings.Index(cmd, "STARTTLS") == 0:
				client.responseAdd("220 Ready to start TLS")
				client.state = ClientStartTLS
			default:

				client.responseAdd("500 Unrecognized command: " + cmd)
				client.errors++
				if client.errors > MaxUnrecognizedCommands {
					client.responseAdd("554 Too many unrecognized commands")
					client.kill()
				}
			}

		case ClientData:
			var err error

			client.bufin.setLimit(server.config.MaxSize)
			client.Data, err = server.read(client)
			if err != nil {
				if err == LineLimitExceeded {
					client.responseAdd("550 Error: " + LineLimitExceeded.Error())
					client.kill()
				} else if err == MessageSizeExceeded {
					client.responseAdd("550 Error: " + MessageSizeExceeded.Error())
					client.kill()
				} else {
					client.kill()
					client.responseAdd("451 Error: " + err.Error())
				}
				log.WithError(err).Warn("Error reading data")
				break
			}

			res := server.backend.Process(client.Envelope)
			if res.Code() < 300 {
				client.messagesSent++
				log.WithFields(map[string]interface{}{
					"helo":          client.Helo,
					"remoteAddress": client.RemoteAddress,
					"success":       true,
				}).Info("Received message")
			}
			client.responseAdd(res.String())
			client.state = ClientCmd
			if server.isShuttingDown() {
				client.state = ClientShutdown
			}
			client.resetTransaction()

		case ClientStartTLS:
			if !client.TLS && server.config.StartTLSOn {
				if server.upgradeToTLS(client) {
					advertiseTLS = ""
					client.resetTransaction()
				}
			}
			// change to command state
			client.state = ClientCmd
		case ClientShutdown:
			// shutdown state
			client.responseAdd("421 Server is shutting down. Please try again later. Sayonara!")
			client.kill()
		}

		if len(client.response) > 0 {
			log.Debugf("Writing response to client: \n%s", client.response)
			err := server.writeResponse(client)
			if err != nil {
				log.WithError(err).Debug("Error writing response")
				return
			}
		}

	}
}
