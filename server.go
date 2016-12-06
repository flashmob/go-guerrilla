package guerrilla

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
)

const (
	CommandVerbMaxLength = 16
	CommandLineMaxLength = 1024
	// Number of allowed unrecognized commands before we terminate the connection
	MaxUnrecognizedCommands = 5
)

// Server listens for SMTP clients on the port specified in its config
type Server struct {
	config    *ServerConfig
	backend   Backend
	tlsConfig *tls.Config
	maxSize   int64
	timeout   time.Duration
	sem       chan int
}

// Creates and returns a new ready-to-run Server from a configuration
func NewServer(sc *ServerConfig, b Backend) (*Server, error) {
	server := &Server{
		config:  sc,
		backend: b,
		maxSize: sc.MaxSize,
		sem:     make(chan int, sc.MaxClients),
	}

	if server.config.RequireTLS || server.config.AdvertiseTLS {
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

	server.timeout = time.Duration(server.config.Timeout) * time.Second

	return server, nil
}

// Begin accepting SMTP clients
func (server *Server) Start() error {
	listener, err := net.Listen("tcp", server.config.ListenInterface)
	if err != nil {
		return fmt.Errorf("Cannot listen on port: %s", err.Error())
	}

	log.Infof("Listening on TCP %s", server.config.ListenInterface)
	var clientID int64
	clientID = 1
	for {
		log.Debugf("Waiting for a new client. Client ID: %d", clientID)
		conn, err := listener.Accept()
		if err != nil {
			log.WithError(err).Info("Error accepting client")
			continue
		}

		client := &Client{
			conn:        conn,
			Address:     conn.RemoteAddr().String(),
			ConnectedAt: time.Now(),
			bufin:       NewSMTPBufferedReader(conn),
			bufout:      bufio.NewWriter(conn),
			ID:          clientID,
		}
		server.sem <- 1
		go server.handleClient(client)
		clientID++
	}
}

// Verifies that the host is a valid recipient.
func (server *Server) allowsHost(host string) bool {
	for _, allowed := range server.config.AllowedHosts {
		if host == allowed {
			return true
		}
	}
	return false
}

// Upgrades a client connection to TLS
func (server *Server) upgradeToTLS(client *Client) bool {
	tlsConn := tls.Server(client.conn, server.tlsConfig)
	err := tlsConn.Handshake()
	if err != nil {
		return false
	}
	client.conn = net.Conn(tlsConn)
	client.bufin = NewSMTPBufferedReader(client.conn)
	client.bufout = bufio.NewWriter(client.conn)
	client.TLS = true

	return true
}

// Closes a client connection
func (server *Server) closeConn(client *Client) {
	client.conn.Close()
	<-server.sem
}

// Reads from the client until a terminating sequence is encountered,
// or until a timeout occurs.
func (server *Server) read(client *Client) (string, error) {
	var input, reply string
	var err error

	// In command state, stop reading at line breaks
	suffix := "\r\n"
	if client.state == ClientData {
		// In data state, stop reading at solo periods
		suffix = "\r\n.\r\n"
	}

	for {
		client.conn.SetDeadline(time.Now().Add(server.timeout))
		reply, err = client.bufin.ReadString('\n')
		input = input + reply
		if int64(len(input)) > server.config.MaxSize {
			return input, fmt.Errorf("Maximum DATA size exceeded (%d)", server.config.MaxSize)
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
func (server *Server) writeResponse(client *Client) error {
	client.conn.SetDeadline(time.Now().Add(server.timeout))
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

// Handles an entire client SMTP exchange
func (server *Server) handleClient(client *Client) {
	defer server.closeConn(client)
	log.Info("Handling client: ", client.ID)

	// Initial greeting
	greeting := fmt.Sprintf("220 %s SMTP Guerrilla(%s) #%d (%d) %s",
		server.config.Hostname, Version, client.ID,
		server.config.MaxClients, time.Now().Format(time.RFC3339))

	helo := fmt.Sprintf("250 %s Hello", server.config.Hostname)
	ehlo := fmt.Sprintf("250-%s Hello", server.config.Hostname)

	// Extended feature advertisements
	messageSize := fmt.Sprintf("250-SIZE %d\r\n", server.config.MaxSize)
	pipelining := "250-PIPELINING\r\n"
	advertiseTLS := "250-STARTTLS\r\n"
	help := "250 HELP"

	if server.config.RequireTLS {
		success := server.upgradeToTLS(client)
		if !success {
			client.kill()
		}
		advertiseTLS = ""
	}

	for client.isAlive() {
		switch client.state {
		case ClientHandshake:
			client.responseAdd(greeting)
			client.state = ClientCmd

		case ClientCmd:
			client.bufin.setLimit(CommandLineMaxLength)
			input, err := server.read(client)
			log.Debugf("Client sent: %s", input)
			if err == io.EOF {
				log.WithError(err).Warnf("Client closed the connection: %s", client.Address)
				return
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.WithError(err).Warnf("Timeout: %s", client.Address)
				return
			} else if err == LineLimitExceeded {
				client.responseAdd("500 Line too long.")
				client.kill()
			} else if err != nil {
				log.WithError(err).Warnf("Read error: %s", client.Address)
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
				client.responseAdd(helo)

			case strings.Index(cmd, "EHLO") == 0:
				client.Helo = strings.Trim(input[4:], " ")
				client.responseAdd(ehlo + messageSize + pipelining + advertiseTLS + help)

			case strings.Index(cmd, "HELP") == 0:
				client.responseAdd("214 OK\r\n" + messageSize + pipelining + advertiseTLS + help)

			case strings.Index(cmd, "MAIL FROM:") == 0:
				client.reset()
				from, err := extractEmail(input[10:])
				if err != nil {
					client.responseAdd("550 Error: Invalid Sender")
				} else {
					client.MailFrom = from
					client.responseAdd("250 OK")
				}

			case strings.Index(cmd, "RCPT TO:") == 0:
				to, err := extractEmail(input[8:])
				if err != nil {
					client.responseAdd("550 Error: Invalid Recipient")
				} else {
					client.RcptTo = append(client.RcptTo, to)
					client.responseAdd("250 OK")
				}

			case strings.Index(cmd, "RSET") == 0:
				client.reset()
				client.responseAdd("250 OK")

			case strings.Index(cmd, "VRFY") == 0:
				client.responseAdd("252 Cannot verify user")

			case strings.Index(cmd, "NOOP") == 0:
				client.responseAdd("250 OK")

			case strings.Index(cmd, "QUIT") == 0:
				client.responseAdd("221 Bye")
				client.kill()

			case strings.Index(cmd, "DATA") == 0:
				client.responseAdd("354 Enter message, ending with '.' on a line by itself")
				client.state = ClientData

			case strings.Index(cmd, "STARTTLS") == 0:
				client.responseAdd("220 Ready to start TLS")
				client.state = ClientStartTLS

			default:
				client.responseAdd("500 Unrecognized command: " + cmd)
				client.Errors++
				if client.Errors > MaxUnrecognizedCommands {
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
					client.responseAdd("451 Error: " + err.Error())
				}
				log.WithError(err).Warn("Error reading data")
				continue
			}

			if client.MailFrom.isEmpty() {
				client.responseAdd("550 Error: No sender")
				continue
			}
			if len(client.RcptTo) == 0 {
				client.responseAdd("550 Error: No recipients")
				continue
			}
			client.parseHeaders()

			for _, rcpt := range client.RcptTo {
				if !server.allowsHost(rcpt.Host) {
					client.responseAdd("550 Error: Host not allowed: " + rcpt.Host)
					continue
				}
			}

			res := server.backend.Process(client)
			client.responseAdd(res)
			client.state = ClientCmd

		case ClientStartTLS:
			if !client.TLS && server.config.AdvertiseTLS {
				if server.upgradeToTLS(client) {
					advertiseTLS = ""
					client.responseAdd("250 OK")
					client.reset()
					client.state = ClientCmd
				} else {
					client.responseAdd("454 Error: Upgrade to TLS failed")
				}
			}
		}

		log.Debugf("Writing response to client: \n%s", client.response)
		err := server.writeResponse(client)
		if err != nil {
			log.WithError(err).Debug("Error writing response")
			if err == io.EOF {
				return
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return
			}
		}
	}
}
