package guerrilla

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
)

const (
	CommandMaxLength        = 16
	CommandLineMaxLength    = 1024
	MaxUnrecognizedCommands = 5
)

type SMTPServer struct {
	config      *ServerConfig
	tlsConfig   *tls.Config
	maxMailSize int64
	timeout     time.Duration
	sem         chan int
}

func (server *SMTPServer) allowsHost(host string) bool {
	for _, allowed := range server.config.AllowedHosts {
		if host == allowed {
			return true
		}
	}
	return false
}

// Upgrades a client connection to TLS. Returns true if successful,
// false if unsucessful.
func (server *SMTPServer) upgradeToTLS(client *Client) bool {
	tlsConn := tls.Server(client.conn, server.tlsConfig)
	err := tlsConn.Handshake()
	if err != nil {
		return false
	}
	client.conn = net.Conn(tlsConn)
	client.bufin = NewSMTPBufferedReader(client.conn)
	client.bufout = bufio.NewWriter(client.conn)
	client.tls = true

	return true
}

func (server *SMTPServer) kill(client *Client) {
	client.killedAt = time.Now()
}

func (server *SMTPServer) closeConn(client *Client) {

}

func (server *SMTPServer) readSMTP(client *Client) (string, error) {
	var input string
	var err error

	// In command state, stop reading at line breaks
	suffix := "\r\n"
	if client.state == ClientData {
		// In data state, stop reading at solo periods
		suffix = "\r\n.\r\n"
	}

	for {
		// TODO create timeout
		// c.conn.SetDeadline(time.Now().Add(timeout))
		reply, err := client.bufin.ReadString('\n')
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

func (server *SMTPServer) handleClient(client *Client) {
	defer server.closeConn(client)

	// Initial greeting
	greeting := fmt.Sprintf("220 %s SMTP Guerrilla(%s) #%d (%d) %s",
		server.config.Hostname, Version, client.id,
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
			server.kill(client)
		}
		advertiseTLS = ""
	}

	for {
		switch client.state {
		case ClientHandshake:
			client.responseAdd(greeting)
			client.state = ClientCmd

		case ClientCmd:
			client.bufin.SetLimit(CommandLineMaxLength)
			input, err := server.readSMTP(client)
			if err == io.EOF {
				log.WithError(err).Debugf("Client closed the connection: %s", client.address)
				return
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.WithError(err).Debugf("Timeout: %s", client.address)
				return
			} else if err == InputLimitExceeded {
				client.responseAdd("500 Line too long.")
				server.kill(client)
			} else if err != nil {
				log.WithError(err).Warnf("Read error: %s", client.address)
			}

			input = strings.Trim(input, " \r\n")
			cmd := strings.ToUpper(input[0:CommandMaxLength])

			switch {
			case strings.Index(cmd, "HELO") == 0:
				client.helo = strings.Trim(input[4:], " ")
				client.responseAdd(helo)

			case strings.Index(cmd, "EHLO") == 0:
				client.helo = strings.Trim(input[4:], " ")
				client.responseAdd(ehlo + messageSize + pipelining + advertiseTLS + help)

			case strings.Index(cmd, "HELP") == 0:
				client.responseAdd(messageSize + pipelining + advertiseTLS + help)

			case strings.Index(cmd, "MAIL FROM:") == 0:
				from, err := ExtractEmail(input[10:])
				if err != nil {
					client.responseAdd("550 Error: Invalid Sender")
				} else {
					client.mailFrom = from
					client.responseAdd("250 OK")
				}

			case strings.Index(cmd, "RCPT TO:") == 0:
				to, err := ExtractEmail(input[8:])
				if err != nil {
					client.responseAdd("550 Error: Invalid Recipient")
				} else {
					client.rcptTo = append(client.rcptTo, to)
					client.responseAdd("250 OK")
				}

			case strings.Index(cmd, "RSET") == 0:
				client.mailFrom = &EmailParts{}
				client.rcptTo = []*EmailParts{}
				client.responseAdd("250 OK")

			case strings.Index(cmd, "VRFY") == 0:
				client.responseAdd("250 OK")

			case strings.Index(cmd, "NOOP") == 0:
				client.responseAdd("250 OK")

			case strings.Index(cmd, "QUIT") == 0:
				client.responseAdd("221 Bye")
				server.kill(client)

			case strings.Index(cmd, "DATA") == 0:
				client.responseAdd("354 Enter message, ending with '.' on a line by itself")
				client.state = ClientData

			case strings.Index(cmd, "STARTTLS") == 0:
				if !client.tls && server.config.AdvertiseTLS {
					if server.upgradeToTLS(client) {
						advertiseTLS = ""
						client.state = ClientCmd
					}
				}
				// TODO error checking

			default:
				client.responseAdd("500 Unrecognized command: " + cmd)
				client.errors++
				if client.errors > MaxUnrecognizedCommands {
					client.responseAdd("500 Too many unrecognized commands")
					server.kill(client)
				}
			}

		case ClientData:
			var err error

			client.bufin.SetLimit(server.config.MaxSize)
			client.data, err = server.readSMTP(client)
			if err != nil {
				if err == InputLimitExceeded {
					client.responseAdd("550 Error: DATA limit exceeded")
					server.kill(client)
				} else {
					client.responseAdd("550 Error: " + err.Error())
				}
				log.WithError(err).Warn("Error reading data")
				continue
			}

			if client.mailFrom.isEmpty() {
				client.responseAdd("550 Error: No sender")
				continue
			}
			if len(client.rcptTo) == 0 {
				client.responseAdd("550 Error: No recipients")
				continue
			}

			for _, rcpt := range client.rcptTo {
				if !server.allowsHost(rcpt.Host) {
					client.responseAdd("550 Error: Host not allowed")
					continue
				}
			}

			// Process email and send success message
			log.Info("Received data successfully")
		}
	}
}
