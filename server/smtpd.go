package server

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"

	guerrilla "github.com/flashmob/go-guerrilla"
	"github.com/flashmob/go-guerrilla/util"
)

type SmtpdServer struct {
	tlsConfig       *tls.Config
	maxSize         int // max email DATA size
	timeout         time.Duration
	sem             chan int // currently active client list
	Config          guerrilla.ServerConfig
	allowedHostsStr string
}

// Upgrades the connection to TLS
// Sets up buffers with the upgraded connection
func (server *SmtpdServer) upgradeToTls(client *guerrilla.Client) bool {
	var tlsConn *tls.Conn
	tlsConn = tls.Server(client.Conn, server.tlsConfig)
	err := tlsConn.Handshake()
	if err == nil {
		client.Conn = net.Conn(tlsConn)
		client.Bufin = guerrilla.NewSMTPBufferedReader(client.Conn)
		client.Bufout = bufio.NewWriter(client.Conn)
		client.TLS = true

		return true
	}

	log.WithError(err).Warn("Failed to TLS handshake")
	return false
}

func (server *SmtpdServer) handleClient(client *guerrilla.Client, backend guerrilla.Backend) {
	defer server.closeClient(client)
	advertiseTLS := "250-STARTTLS\r\n"
	if server.Config.TLSAlwaysOn {
		if server.upgradeToTls(client) {
			advertiseTLS = ""
		}
	}
	greeting := fmt.Sprintf("220 %s SMTP guerrillad(%s) #%d (%d) %s",
		server.Config.Hostname, guerrilla.Version, client.ClientID,
		len(server.sem), time.Now().Format(time.RFC1123Z))

	if !server.Config.StartTLS {
		// STARTTLS turned off
		advertiseTLS = ""
	}
	for i := 0; i < 100; i++ {
		switch client.State {
		case 0:
			responseAdd(client, greeting)
			client.State = 1
		case 1:
			client.Bufin.SetLimit(guerrilla.CommandMaxLength)
			input, err := server.readSmtp(client)
			if err != nil {
				if err == io.EOF {
					log.WithError(err).Debugf("Client closed the connection already: %s", client.Address)
					return
				} else if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					log.WithError(err).Debugf("Timeout: %s", client.Address)
					return
				} else if err == guerrilla.InputLimitExceeded {
					responseAdd(client, "500 Line too long.")
					// kill it so that another one can connect
					killClient(client)
				}
				log.WithError(err).Warnf("Read error: %s", client.Address)
				break
			}
			input = strings.Trim(input, " \n\r")
			bound := len(input)
			if bound > 16 {
				bound = 16
			}
			cmd := strings.ToUpper(input[0:bound])
			switch {
			case strings.Index(cmd, "HELO") == 0:
				if len(input) > 5 {
					client.Helo = input[5:]
				}
				responseAdd(client, "250 "+server.Config.Hostname+" Hello ")
			case strings.Index(cmd, "EHLO") == 0:
				if len(input) > 5 {
					client.Helo = input[5:]
				}
				responseAdd(client, fmt.Sprintf(
					`250-%s Hello %s[%s]\r
250-SIZE %d\r
250-PIPELINING \r
%s250 HELP`,
					server.Config.Hostname, client.Helo, client.Address,
					server.Config.MaxSize, advertiseTLS))
			case strings.Index(cmd, "HELP") == 0:
				responseAdd(client, "250 Help! I need somebody...")
			case strings.Index(cmd, "MAIL FROM:") == 0:
				if len(input) > 10 {
					client.MailFrom = input[10:]
				}
				responseAdd(client, "250 Ok")
			case strings.Index(cmd, "XCLIENT") == 0:
				// Nginx sends this
				// XCLIENT ADDR=212.96.64.216 NAME=[UNAVAILABLE]
				client.Address = input[13:]
				client.Address = client.Address[0:strings.Index(client.Address, " ")]
				fmt.Println("client address:[" + client.Address + "]")
				responseAdd(client, "250 OK")
			case strings.Index(cmd, "RCPT TO:") == 0:
				if len(input) > 8 {
					client.RcptTo = input[8:]
				}
				responseAdd(client, "250 Accepted")
			case strings.Index(cmd, "NOOP") == 0:
				responseAdd(client, "250 OK")
			case strings.Index(cmd, "RSET") == 0:
				client.MailFrom = ""
				client.RcptTo = ""
				responseAdd(client, "250 OK")
			case strings.Index(cmd, "DATA") == 0:
				responseAdd(client, "354 Enter message, ending with \".\" on a line by itself")
				client.State = 2
			case (strings.Index(cmd, "STARTTLS") == 0) &&
				!client.TLS &&
				server.Config.StartTLS:
				responseAdd(client, "220 Ready to start TLS")
				// go to start TLS state
				client.State = 3
			case strings.Index(cmd, "QUIT") == 0:
				responseAdd(client, "221 Bye")
				killClient(client)
			default:
				responseAdd(client, "500 unrecognized command: "+cmd)
				client.Errors++
				if client.Errors > 3 {
					responseAdd(client, "500 Too many unrecognized commands")
					killClient(client)
				}
			}
		case 2:
			var err error
			client.Bufin.SetLimit(int64(server.Config.MaxSize) + 1024000) // This is a hard limit.
			client.Data, err = server.readSmtp(client)
			if err == nil {
				if user, host, mailErr := util.ValidateEmailData(client, server.allowedHostsStr); mailErr == nil {
					resp := backend.Process(client, user, host)
					responseAdd(client, resp)
				} else {
					responseAdd(client, "550 Error: "+mailErr.Error())
				}

			} else {
				if err == guerrilla.InputLimitExceeded {
					// hard limit reached, end to make room for other clients
					responseAdd(client, "550 Error: DATA limit exceeded by more than a megabyte!")
					killClient(client)
				} else {
					responseAdd(client, "550 Error: "+err.Error())
				}

				log.WithError(err).Warn("DATA read error")
			}
			client.State = 1
		case 3:
			// upgrade to TLS
			if server.upgradeToTls(client) {
				advertiseTLS = ""
				client.State = 1
			}
		}
		// Send a response back to the client
		err := server.responseWrite(client)
		if err != nil {
			if err == io.EOF {
				// client closed the connection already
				return
			}
			if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				// too slow, timeout
				return
			}
		}
		if client.KillTime > 1 {
			return
		}
	}

}

// add a response on the response buffer
func responseAdd(client *guerrilla.Client, line string) {
	client.Response = line + "\r\n"
}
func (server *SmtpdServer) closeClient(client *guerrilla.Client) {
	client.Conn.Close()
	<-server.sem // Done; enable next client to run.
}
func killClient(client *guerrilla.Client) {
	client.KillTime = time.Now().Unix()
}

// Reads from the smtpBufferedReader, can be in command state or data state.
func (server *SmtpdServer) readSmtp(client *guerrilla.Client) (input string, err error) {
	var reply string
	// Command state terminator by default
	suffix := "\r\n"
	if client.State == 2 {
		// DATA state ends with a dot on a line by itself
		suffix = "\r\n.\r\n"
	}
	for err == nil {
		client.Conn.SetDeadline(time.Now().Add(server.timeout * time.Second))
		reply, err = client.Bufin.ReadString('\n')
		if reply != "" {
			input = input + reply
			if len(input) > server.Config.MaxSize {
				err = fmt.Errorf("Maximum DATA size exceeded (%d)", server.Config.MaxSize)
				return input, err
			}
			if client.State == 2 {
				// Extract the subject while we are at it.
				scanSubject(client, reply)
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

// Scan the data part for a Subject line. Can be a multi-line
func scanSubject(client *guerrilla.Client, reply string) {
	if client.Subject == "" && (len(reply) > 8) {
		test := strings.ToUpper(reply[0:9])
		if i := strings.Index(test, "SUBJECT: "); i == 0 {
			// first line with \r\n
			client.Subject = reply[9:]
		}
	} else if strings.HasSuffix(client.Subject, "\r\n") {
		// chop off the \r\n
		client.Subject = client.Subject[0 : len(client.Subject)-2]
		if (strings.HasPrefix(reply, " ")) || (strings.HasPrefix(reply, "\t")) {
			// subject is multi-line
			client.Subject = client.Subject + reply[1:]
		}
	}
}

func (server *SmtpdServer) responseWrite(client *guerrilla.Client) (err error) {
	var size int
	client.Conn.SetDeadline(time.Now().Add(server.timeout * time.Second))
	size, err = client.Bufout.WriteString(client.Response)
	client.Bufout.Flush()
	client.Response = client.Response[size:]
	return err
}
