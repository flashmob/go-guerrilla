package server

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	evbus "github.com/asaskevich/EventBus"

	log "github.com/Sirupsen/logrus"

	guerrilla "github.com/flashmob/go-guerrilla"
	"github.com/flashmob/go-guerrilla/util"
	"sync/atomic"
	"sync"
)

type SmtpdServer struct {
	mainConfigStore  atomic.Value // stores guerrilla.Config
	configStore      atomic.Value // stores guerrilla.ServerConfig
	tlsConfig        *tls.Config
	tlsConfigStore   atomic.Value
	maxSize          int // max email DATA size
	timeout          atomic.Value
	wg               sync.WaitGroup // for waiting to shutdown
	shuttingDownFlag atomic.Value
	bus              *evbus.EventBus
	clientPool       *Pool
}

func newSmtpdServer(mainConfig guerrilla.Config, sConfig guerrilla.ServerConfig, bus *evbus.EventBus) *SmtpdServer {
	server := SmtpdServer{
		bus:           bus,
		clientPool:    NewPool(sConfig.MaxClients),
	}
	server.mainConfigStore.Store(mainConfig)
	server.configStore.Store(sConfig)
	server.setTimeout(sConfig.Timeout)
	return &server
}


// Upgrades the connection to TLS
// Sets up buffers with the upgraded connection
func (server *SmtpdServer) upgradeToTls(client *guerrilla.Client) error {
	var tlsConn *tls.Conn
	// load the config thread-safely
	tlsConfig := server.tlsConfigStore.Load().(*tls.Config)
	tlsConn = tls.Server(client.Conn, tlsConfig)
	err := tlsConn.Handshake()
	if err == nil {
		client.Conn = net.Conn(tlsConn)
		client.Bufout.Reset(client.Conn)
		client.Bufin.Reset(client.Conn)
		client.TLS = true
		return err
	}
	log.WithError(err).Warn("Failed to TLS handshake")
	return err
}

func (server *SmtpdServer) Shutdown() {
	server.shuttingDownFlag.Store(true)
	cfg := server.configStore.Load().(guerrilla.ServerConfig)
	log.Infof("shutting down pool [%s]", cfg.ListenInterface)
	server.clientPool.Shutdown()
	log.Infof("Waiting for all [%s] clients to close", cfg.ListenInterface)
	server.waitAllClientsToClose()
}

func (server *SmtpdServer) isShuttingDown() bool {
	if is, really := server.shuttingDownFlag.Load().(bool); is && really {
		return true;
	}
	return false
}

// wait for all active clients to finish
func (server *SmtpdServer) waitAllClientsToClose() {
	server.wg.Wait()
}

func (server *SmtpdServer) GetActiveClientsCount() int {
	return server.clientPool.GetActiveClientsCount()
}

func (server *SmtpdServer) handleClient(client *guerrilla.Client, backend guerrilla.Backend) {
	defer server.closeClient(client)
	server.wg.Add(1)
	// safely init the server config and main config
	sConfig := server.configStore.Load().(guerrilla.ServerConfig)
	mainConfig := server.mainConfigStore.Load().(guerrilla.Config)
	advertiseTLS := "250-STARTTLS\r\n"
	if sConfig.TLSAlwaysOn {
		if tlsErr := server.upgradeToTls(client); tlsErr == nil {
			advertiseTLS = ""
		} else {
			return
		}
	}
	greeting := fmt.Sprintf("220 %s SMTP guerrillad(%s) #%d (%d) %s",
		sConfig.Hostname, guerrilla.Version, client.ClientID,
		server.GetActiveClientsCount(), time.Now().Format(time.RFC1123Z))

	if !sConfig.StartTLS {
		// STARTTLS turned off
		advertiseTLS = ""
	}
	for ;; {
		switch client.State {
		case 0:
			responseAdd(client, greeting)
			client.State = 1
		case 1:
			client.Bufin.SetLimit(guerrilla.CommandMaxLength)
			input, err := server.readSmtp(client, sConfig.MaxSize)
			if err != nil {
				if err == io.EOF {
					log.WithError(err).Debugf("Client closed the connection already: %s", client.Address)
					return
				}
				if (server.isShuttingDown()) {
					break // do not accept anymore commands
				}
				if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					log.WithError(err).Debugf("Timeout: %s", client.Address)
					responseAdd(client, "421 Error: timeout exceeded")
					killClient(client)
					break // do not accept anymore commands
				} else if err == guerrilla.InputLimitExceeded {
					responseAdd(client, "500 Line too long.")
					// kill it so that another one can connect
					killClient(client)
				}
				log.WithError(err).Warnf("Read error: %s", client.Address)
				break // do not accept anymore commands
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
				responseAdd(client, "250 "+sConfig.Hostname+" Hello ")
			case strings.Index(cmd, "EHLO") == 0:
				if len(input) > 5 {
					client.Helo = input[5:]
				}
				responseAdd(client, fmt.Sprintf(
					"250-%s Hello %s[%s]\r\n"+
					"250-SIZE %d\r\n" +
					"250-PIPELINING\r\n" +
					"%s250 HELP",
					sConfig.Hostname, client.Helo, client.Address,
					sConfig.MaxSize, advertiseTLS))
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
				sConfig.StartTLS:
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
			client.Bufin.SetLimit(int64(sConfig.MaxSize) + 1024000) // This is a hard limit.
			client.Data, err = server.readSmtp(client, sConfig.MaxSize)
			if err == nil {
				from, mailErr := util.ExtractEmail(client.MailFrom)
				if mailErr != nil {
					responseAdd(client, fmt.Sprintf("550 Error: invalid from: ", mailErr.Error()))
				} else {
					// TODO: support multiple RcptTo
					to, mailErr := util.ExtractEmail(client.RcptTo)
					if mailErr != nil {
						responseAdd(client, fmt.Sprintf("550 Error: invalid from: ", mailErr.Error()))
					} else {
						client.MailFrom = from.String()
						client.RcptTo = to.String()
						if !mainConfig.IsHostAllowed(to.Host) {
							responseAdd(client, "550 Error: Rcpt-to host is not allowed")
						} else {
							toArray := []*guerrilla.EmailParts{to}
							resp := backend.Process(client, from, toArray)
							responseAdd(client, resp)
						}
					}
				}

			} else {
				if err == io.EOF {
					log.WithError(err).Debugf("Client closed the connection already: %s", client.Address)
					return
				}
				if (server.isShuttingDown()) {
					// When shutting down in DATA state, the server will let the client
					// finish the email, unless a timeout is detected.
					break
				}
				if err == guerrilla.InputLimitExceeded {
					// hard limit reached, end to make room for other clients
					responseAdd(client, "550 Error: DATA limit exceeded by more than a megabyte!")
					killClient(client)
				} else if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					log.WithError(err).Debugf("Timeout: %s", client.Address)
					responseAdd(client, "421 Error: timeout exceeded")
					killClient(client)
				} else {
					responseAdd(client, "550 Error: "+err.Error())
					killClient(client)
				}

				log.WithError(err).Warn("DATA read error")
			}
			client.State = 1

		case 3:
			// upgrade to TLS
			if tlsErr :=server.upgradeToTls(client); tlsErr == nil {
				advertiseTLS = ""
				client.State = 1
			} else {
				return
			}
		case 4:
			// shutdown state
			responseAdd(client, "421 Server is shutting down. Please try again later. Sayonara!")
		}

		// Send a response back to the client
		err := server.responseWrite(client)
		if err != nil {
			if err == io.EOF {
				// client already closed the connection
				log.WithError(err).Debugf("Connection reset by peer: %s", client.Address)
				return
			}
			if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				// too slow, timeout
				log.WithError(err).Debugf("Timeout: %s", client.Address)
				return
			}
		}
		if client.KillTime > 1 {
			if (server.isShuttingDown() && client.State != 4) {
				client.State = 4
			} else {
				return
			}

		}
	}

}

// add a response on the response buffer
func responseAdd(client *guerrilla.Client, line string) {
	client.Response = line + "\r\n"
}
func (server *SmtpdServer) closeClient(client *guerrilla.Client) {
	server.wg.Done()
	client.Conn.Close()
}
func killClient(client *guerrilla.Client) {
	client.KillTime = time.Now().Unix()
}

// Reads from the smtpBufferedReader, can be in command state or data state.
func (server *SmtpdServer) readSmtp(client *guerrilla.Client, maxSize int) (input string, err error) {
	var reply string
	// Command state terminator by default
	suffix := "\r\n"
	if client.State == 2 {
		// DATA state ends with a dot on a line by itself
		suffix = "\r\n.\r\n"
	}
	for err == nil {
		client.SetTimeout(server.timeout.Load().(time.Duration))
		reply, err = client.Bufin.ReadString('\n')
		if reply != "" {
			input = input + reply
			if len(input) > maxSize {
				err = fmt.Errorf("Maximum DATA size exceeded (%d)", maxSize)
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
	client.SetTimeout(server.timeout.Load().(time.Duration))
	if len(client.Response) > 0 {
		size, err = client.Bufout.WriteString(client.Response)
		client.Bufout.Flush()
		client.Response = client.Response[size:]
	}
	return err
}
