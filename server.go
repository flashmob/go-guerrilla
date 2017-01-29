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
	"sync/atomic"

	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/envelope"
	"github.com/flashmob/go-guerrilla/response"
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

const (
	// server has just been created
	ServerStateNew = iota
	// Server has just been stopped
	ServerStateStopped
	// Server has been started and is running
	ServerStateRunning
	// Server could not start due to an error
	ServerStateStartError
)

// Server listens for SMTP clients on the port specified in its config
type server struct {
	configStore     atomic.Value // stores guerrilla.ServerConfig
	backend         backends.Backend
	tlsConfig       *tls.Config
	tlsConfigStore  atomic.Value
	timeout         atomic.Value // stores time.Duration
	listenInterface string
	clientPool      *Pool
	wg              sync.WaitGroup // for waiting to shutdown
	listener        net.Listener
	closedListener  chan (bool)
	hosts           allowedHosts // stores map[string]bool for faster lookup
	state           int
}

type allowedHosts struct {
	table map[string]bool // host lookup table
	m     sync.Mutex      // guard access to the map
}

// Creates and returns a new ready-to-run Server from a configuration
func newServer(sc *ServerConfig, b backends.Backend) (*server, error) {
	server := &server{
		backend:         b,
		clientPool:      NewPool(sc.MaxClients),
		closedListener:  make(chan (bool), 1),
		listenInterface: sc.ListenInterface,
		state:           ServerStateNew,
	}
	server.setConfig(sc)
	server.setTimeout(sc.Timeout)
	if err := server.configureSSL(); err != nil {
		return server, err
	}
	return server, nil
}

func (s *server) configureSSL() error {
	sConfig := s.configStore.Load().(ServerConfig)
	if sConfig.TLSAlwaysOn || sConfig.StartTLSOn {
		cert, err := tls.LoadX509KeyPair(sConfig.PublicKeyFile, sConfig.PrivateKeyFile)
		if err != nil {
			return fmt.Errorf("error while loading the certificate: %s", err)
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.VerifyClientCertIfGiven,
			ServerName:   sConfig.Hostname,
		}
		tlsConfig.Rand = rand.Reader
		s.tlsConfigStore.Store(tlsConfig)
	}
	return nil
}

// Set the timeout for the server and all clients
func (server *server) setTimeout(seconds int) {
	duration := time.Duration(int64(seconds))
	server.clientPool.SetTimeout(duration)
	server.timeout.Store(duration)
}

// goroutine safe config store
func (server *server) setConfig(sc *ServerConfig) {
	server.configStore.Store(*sc)
}

// goroutine safe
func (server *server) isEnabled() bool {
	sc := server.configStore.Load().(ServerConfig)
	return sc.IsEnabled
}

// Set the allowed hosts for the server
func (server *server) setAllowedHosts(allowedHosts []string) {
	defer server.hosts.m.Unlock()
	server.hosts.m.Lock()
	server.hosts.table = make(map[string]bool, len(allowedHosts))
	for _, h := range allowedHosts {
		server.hosts.table[strings.ToLower(h)] = true
	}
}

// Begin accepting SMTP clients. Will block unless there is an error or server.Shutdown() is called
func (server *server) Start(startWG *sync.WaitGroup) error {
	var clientID uint64
	clientID = 0

	listener, err := net.Listen("tcp", server.listenInterface)
	server.listener = listener
	if err != nil {
		startWG.Done() // don't wait for me
		server.state = ServerStateStartError
		return fmt.Errorf("[%s] Cannot listen on port: %s ", server.listenInterface, err.Error())
	}

	log.Infof("Listening on TCP %s", server.listenInterface)
	server.state = ServerStateRunning
	startWG.Done() // start successful, don't wait for me

	for {
		log.Debugf("[%s] Waiting for a new client. Next Client ID: %d", server.listenInterface, clientID+1)
		conn, err := listener.Accept()
		clientID++
		if err != nil {
			if e, ok := err.(net.Error); ok && !e.Temporary() {
				log.Infof("Server [%s] has stopped accepting new clients", server.listenInterface)
				// the listener has been closed, wait for clients to exit
				log.Infof("shutting down pool [%s]", server.listenInterface)
				server.clientPool.ShutdownState()
				server.clientPool.ShutdownWait()
				server.state = ServerStateStopped
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
	if server.listener != nil {
		// This will cause Start function to return, by causing an error on listener.Accept
		server.listener.Close()
		// wait for the listener to listener.Accept
		<-server.closedListener
		// At this point Start will exit and close down the pool
	} else {
		server.clientPool.ShutdownState()
		// listener already closed, wait for clients to exit
		server.clientPool.ShutdownWait()
		server.state = ServerStateStopped
	}
}

func (server *server) GetActiveClientsCount() int {
	return server.clientPool.GetActiveClientsCount()
}

// Verifies that the host is a valid recipient.
func (server *server) allowsHost(host string) bool {
	defer server.hosts.m.Unlock()
	server.hosts.m.Lock()
	if _, ok := server.hosts.table[strings.ToLower(host)]; ok {
		return true
	}
	return false
}

// Reads from the client until a terminating sequence is encountered,
// or until a timeout occurs.
func (server *server) readCommand(client *client, maxSize int64) (string, error) {
	var input, reply string
	var err error
	// In command state, stop reading at line breaks
	suffix := "\r\n"
	for {
		client.setTimeout(server.timeout.Load().(time.Duration))
		reply, err = client.bufin.ReadString('\n')
		input = input + reply
		if err != nil {
			break
		}
		if strings.HasSuffix(input, suffix) {
			// discard the suffix and stop reading
			input = input[0 : len(input)-len(suffix)]
			break
		}
	}
	return input, err
}

// Writes a response to the client.
func (server *server) writeResponse(client *client) error {
	client.setTimeout(server.timeout.Load().(time.Duration))
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
	defer client.closeConn()
	sc := server.configStore.Load().(ServerConfig)
	log.Infof("Handle client [%s], id: %d", client.RemoteAddress, client.ID)

	// Initial greeting
	greeting := fmt.Sprintf("220 %s SMTP Guerrilla(%s) #%d (%d) %s gr:%d",
		sc.Hostname, Version, client.ID,
		server.clientPool.GetActiveClientsCount(), time.Now().Format(time.RFC3339), runtime.NumGoroutine())

	helo := fmt.Sprintf("250 %s Hello", sc.Hostname)
	// ehlo is a multi-line reply and need additional \r\n at the end
	ehlo := fmt.Sprintf("250-%s Hello\r\n", sc.Hostname)

	// Extended feature advertisements
	messageSize := fmt.Sprintf("250-SIZE %d\r\n", sc.MaxSize)
	pipelining := "250-PIPELINING\r\n"
	advertiseTLS := "250-STARTTLS\r\n"
	advertiseEnhancedStatusCodes := "250-ENHANCEDSTATUSCODES\r\n"
	// the last line doesn't need \r\n since string will be printed as a new line
	help := "250 HELP"

	if sc.TLSAlwaysOn {
		tlsConfig, ok := server.tlsConfigStore.Load().(*tls.Config)
		if ok && client.upgradeToTLS(tlsConfig) {
			advertiseTLS = ""
		} else {
			// server requires TLS, but can't handshake
			client.kill()
		}
	}
	if !sc.StartTLSOn {
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
			input, err := server.readCommand(client, sc.MaxSize)
			log.Debugf("Client sent: %s", input)
			if err == io.EOF {
				log.WithError(err).Warnf("Client closed the connection: %s", client.RemoteAddress)
				return
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.WithError(err).Warnf("Timeout: %s", client.RemoteAddress)
				return
			} else if err == LineLimitExceeded {
				resp := &response.Response{
					EnhancedCode: response.InvalidCommand,
					BasicCode:    554,
					Class:        response.ClassPermanentFailure,
					Comment:      "Line too long.",
				}
				client.responseAdd(resp.String())
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
				client.responseAdd(ehlo + messageSize + pipelining + advertiseTLS + advertiseEnhancedStatusCodes + help)

			case strings.Index(cmd, "HELP") == 0:
				client.responseAdd("214 OK\r\n" + messageSize + pipelining + advertiseTLS + help)

			case strings.Index(cmd, "MAIL FROM:") == 0:
				if client.isInTransaction() {
					resp := &response.Response{
						EnhancedCode: response.InvalidCommand,
						BasicCode:    503,
						Class:        response.ClassPermanentFailure,
						Comment:      "Error: nested MAIL command",
					}
					client.responseAdd(resp.String())
					break
				}
				// Fix for issue #53 - MAIL FROM may only be <> if it is a bounce
				mail := input[10:]
				from := &envelope.EmailAddress{}
				if len(mail) < 2 {
					resp := &response.Response{
						EnhancedCode: response.SyntaxError,
						BasicCode:    501,
						Class:        response.ClassPermanentFailure,
						Comment:      "Bad sender address syntax",
					}
					client.responseAdd(resp.String())
					break
				}
				if mail[0:2] != "<>" {
					// Not Bounce, extract mail.
					from, err = extractEmail(mail)
				}

				if err != nil {
					client.responseAdd(err.Error())
				} else {
					client.MailFrom = from
					resp := &response.Response{
						EnhancedCode: response.OtherAddressStatus,
						Class:        response.ClassSuccess,
					}
					client.responseAdd(resp.String())
				}

			case strings.Index(cmd, "RCPT TO:") == 0:
				if len(client.RcptTo) > RFC2821LimitRecipients {
					resp := &response.Response{
						EnhancedCode: response.TooManyRecipients,
						BasicCode:    452,
						Class:        response.ClassTransientFailure,
						Comment:      "Too many recipients",
					}
					client.responseAdd(resp.String())
					break
				}
				to, err := extractEmail(input[8:])
				if err != nil {
					client.responseAdd(err.Error())
				} else {
					if !server.allowsHost(to.Host) {
						resp := &response.Response{
							EnhancedCode: response.BadDestinationMailboxAddress,
							BasicCode:    454,
							Class:        response.ClassTransientFailure,
							Comment:      "Error: Relay access denied: " + to.Host,
						}
						client.responseAdd(resp.String())
					} else {
						client.RcptTo = append(client.RcptTo, *to)
						resp := &response.Response{
							EnhancedCode: response.DestinationMailboxAddressValid,
							Class:        response.ClassSuccess,
						}
						client.responseAdd(resp.String())
					}
				}

			case strings.Index(cmd, "RSET") == 0:
				client.resetTransaction()
				resp := &response.Response{
					EnhancedCode: response.OtherAddressStatus,
					Class:        response.ClassSuccess,
				}
				client.responseAdd(resp.String())

			case strings.Index(cmd, "VRFY") == 0:
				resp := &response.Response{
					EnhancedCode: response.OtherOrUndefinedProtocolStatus,
					BasicCode:    252,
					Class:        response.ClassSuccess,
					Comment:      "Cannot verify user",
				}
				client.responseAdd(resp.String())

			case strings.Index(cmd, "NOOP") == 0:
				resp := &response.Response{
					EnhancedCode: response.OtherStatus,
					Class:        response.ClassSuccess,
				}
				client.responseAdd(resp.String())

			case strings.Index(cmd, "QUIT") == 0:
				resp := &response.Response{
					EnhancedCode: response.OtherStatus,
					BasicCode:    221,
					Class:        response.ClassSuccess,
					Comment:      "Bye",
				}
				client.responseAdd(resp.String())
				client.kill()

			case strings.Index(cmd, "DATA") == 0:
				if client.MailFrom.IsEmpty() {
					resp := &response.Response{
						EnhancedCode: response.InvalidCommand,
						BasicCode:    503,
						Class:        response.ClassPermanentFailure,
						Comment:      "Error: No sender",
					}
					client.responseAdd(resp.String())
					break
				}
				if len(client.RcptTo) == 0 {
					resp := &response.Response{
						EnhancedCode: response.InvalidCommand,
						BasicCode:    503,
						Class:        response.ClassPermanentFailure,
						Comment:      "Error: No recipients",
					}
					client.responseAdd(resp.String())
					break
				}
				client.responseAdd("354 Enter message, ending with '.' on a line by itself")
				client.state = ClientData

			case sc.StartTLSOn && strings.Index(cmd, "STARTTLS") == 0:
				resp := &response.Response{
					EnhancedCode: response.OtherStatus,
					BasicCode:    220,
					Class:        response.ClassSuccess,
					Comment:      "Ready to start TLS",
				}
				client.responseAdd(resp.String())
				client.state = ClientStartTLS
			default:
				resp := &response.Response{
					EnhancedCode: response.InvalidCommand,
					BasicCode:    554,
					Class:        response.ClassPermanentFailure,
					Comment:      fmt.Sprintf("Unrecognized command"),
				}
				client.responseAdd(resp.String())
				client.errors++
				if client.errors > MaxUnrecognizedCommands {
					resp := &response.Response{
						EnhancedCode: response.InvalidCommand,
						BasicCode:    554,
						Class:        response.ClassPermanentFailure,
						Comment:      "Too many unrecognized commands",
					}
					client.responseAdd(resp.String())
					client.kill()
				}

			}

		case ClientData:

			// intentionally placed the limit 1MB above so that reading does not return with an error
			// if the client goes a little over. Anything above will err
			client.bufin.setLimit(int64(sc.MaxSize) + 1024000) // This a hard limit.

			n, err := client.Data.ReadFrom(client.smtpReader.DotReader())
			if n > sc.MaxSize {
				err = fmt.Errorf("Maximum DATA size exceeded (%d)", sc.MaxSize)
			}
			if err != nil {
				if err == LineLimitExceeded {
					resp := &response.Response{
						EnhancedCode: response.SyntaxError,
						BasicCode:    550,
						Class:        response.ClassPermanentFailure,
						Comment:      "Error: " + LineLimitExceeded.Error(),
					}
					client.responseAdd(resp.String())
					client.kill()
				} else if err == MessageSizeExceeded {
					resp := &response.Response{
						EnhancedCode: response.SyntaxError,
						BasicCode:    550,
						Class:        response.ClassPermanentFailure,
						Comment:      "Error: " + MessageSizeExceeded.Error(),
					}
					client.responseAdd(resp.String())
					client.kill()
				} else {
					resp := &response.Response{
						EnhancedCode: response.OtherOrUndefinedMailSystemStatus,
						BasicCode:    451,
						Class:        response.ClassTransientFailure,
						Comment:      "Error: " + err.Error(),
					}
					client.responseAdd(resp.String())
					client.kill()
				}
				log.WithError(err).Warn("Error reading data")
				break
			}

			res := server.backend.Process(client.Envelope)
			if res.Code() < 300 {
				client.messagesSent++
			}
			client.responseAdd(res.String())
			client.state = ClientCmd
			if server.isShuttingDown() {
				client.state = ClientShutdown
			}
			client.resetTransaction()

		case ClientStartTLS:
			if !client.TLS && sc.StartTLSOn {
				tlsConfig, ok := server.tlsConfigStore.Load().(*tls.Config)
				if ok && client.upgradeToTLS(tlsConfig) {
					advertiseTLS = ""
					client.resetTransaction()
				}
			}
			// change to command state
			client.state = ClientCmd
		case ClientShutdown:
			// shutdown state
			resp := &response.Response{
				EnhancedCode: response.OtherOrUndefinedMailSystemStatus,
				BasicCode:    421,
				Class:        response.ClassTransientFailure,
				Comment:      "Server is shutting down. Please try again later. Sayonara!",
			}
			client.responseAdd(resp.String())
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
