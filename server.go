package guerrilla

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
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
	tlsConfigStore  atomic.Value
	timeout         atomic.Value // stores time.Duration
	listenInterface string
	clientPool      *Pool
	wg              sync.WaitGroup // for waiting to shutdown
	listener        net.Listener
	closedListener  chan (bool)
	hosts           allowedHosts // stores map[string]bool for faster lookup
	state           int
	// If log changed after a config reload, newLogStore stores the value here until it's safe to change it
	logStore     atomic.Value
	mainlogStore atomic.Value
	backendStore atomic.Value
	envelopePool *mail.Pool
}

type allowedHosts struct {
	table      map[string]bool // host lookup table
	sync.Mutex                 // guard access to the map
}

// Creates and returns a new ready-to-run Server from a configuration
func newServer(sc *ServerConfig, b backends.Backend, l log.Logger) (*server, error) {
	server := &server{
		clientPool:      NewPool(sc.MaxClients),
		closedListener:  make(chan (bool), 1),
		listenInterface: sc.ListenInterface,
		state:           ServerStateNew,
		envelopePool:    mail.NewPool(sc.MaxClients),
	}
	server.logStore.Store(l)
	server.backendStore.Store(b)
	logFile := sc.LogFile
	if logFile == "" {
		// none set, use the same log file as mainlog
		logFile = server.mainlog().GetLogDest()
	}
	// set level to same level as mainlog level
	mainlog, logOpenError := log.GetLogger(logFile, server.mainlog().GetLevel())
	server.mainlogStore.Store(mainlog)
	if logOpenError != nil {
		server.log().WithError(logOpenError).Errorf("Failed creating a logger for server [%s]", sc.ListenInterface)
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

// setBackend sets the backend to use for processing email envelopes
func (s *server) setBackend(b backends.Backend) {
	s.backendStore.Store(b)
}

// backend gets the backend used to process email envelopes
func (s *server) backend() backends.Backend {
	if b, ok := s.backendStore.Load().(backends.Backend); ok {
		return b
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
	server.hosts.Lock()
	defer server.hosts.Unlock()
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

	server.log().Infof("Listening on TCP %s", server.listenInterface)
	server.state = ServerStateRunning
	startWG.Done() // start successful, don't wait for me

	for {
		server.log().Debugf("[%s] Waiting for a new client. Next Client ID: %d", server.listenInterface, clientID+1)
		conn, err := listener.Accept()
		clientID++
		if err != nil {
			if e, ok := err.(net.Error); ok && !e.Temporary() {
				server.log().Infof("Server [%s] has stopped accepting new clients", server.listenInterface)
				// the listener has been closed, wait for clients to exit
				server.log().Infof("shutting down pool [%s]", server.listenInterface)
				server.clientPool.ShutdownState()
				server.clientPool.ShutdownWait()
				server.state = ServerStateStopped
				server.closedListener <- true
				return nil
			}
			server.mainlog().WithError(err).Info("Temporary error accepting client")
			continue
		}
		go func(p Poolable, borrow_err error) {
			c := p.(*client)
			if borrow_err == nil {
				server.handleClient(c)
				server.envelopePool.Return(c.Envelope)
				server.clientPool.Return(c)
			} else {
				server.log().WithError(borrow_err).Info("couldn't borrow a new client")
				// we could not get a client, so close the connection.
				conn.Close()

			}
			// intentionally placed Borrow in args so that it's called in the
			// same main goroutine.
		}(server.clientPool.Borrow(conn, clientID, server.log(), server.envelopePool))

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
// host checking turned off if there is a single entry and it's a dot.
func (server *server) allowsHost(host string) bool {
	server.hosts.Lock()
	defer server.hosts.Unlock()
	if len(server.hosts.table) == 1 {
		if _, ok := server.hosts.table["."]; ok {
			return true
		}
	}
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

// flushResponse a response to the client. Flushes the client.bufout buffer to the connection
func (server *server) flushResponse(client *client) error {
	client.setTimeout(server.timeout.Load().(time.Duration))
	return client.bufout.Flush()
}

func (server *server) isShuttingDown() bool {
	return server.clientPool.IsShuttingDown()
}

// Handles an entire client SMTP exchange
func (server *server) handleClient(client *client) {
	defer client.closeConn()
	sc := server.configStore.Load().(ServerConfig)
	server.log().Infof("Handle client [%s], id: %d", client.RemoteIP, client.ID)

	// Initial greeting
	greeting := fmt.Sprintf("220 %s SMTP Guerrilla(%s) #%d (%d) %s",
		sc.Hostname, Version, client.ID,
		server.clientPool.GetActiveClientsCount(), time.Now().Format(time.RFC3339))

	helo := fmt.Sprintf("250 %s Hello", sc.Hostname)
	// ehlo is a multi-line reply and need additional \r\n at the end
	ehlo := fmt.Sprintf("250-%s Hello\r\n", sc.Hostname)

	// Extended feature advertisements
	messageSize := fmt.Sprintf("250-SIZE %d\r\n", sc.MaxSize)
	pipelining := "250-PIPELINING\r\n"
	advertiseTLS := "250-STARTTLS\r\n"
	advertiseEnhancedStatusCodes := "250-ENHANCEDSTATUSCODES\r\n"
	// The last line doesn't need \r\n since string will be printed as a new line.
	// Also, Last line has no dash -
	help := "250 HELP"

	if sc.TLSAlwaysOn {
		tlsConfig, ok := server.tlsConfigStore.Load().(*tls.Config)
		if !ok {
			server.mainlog().Error("Failed to load *tls.Config")
		} else if err := client.upgradeToTLS(tlsConfig); err == nil {
			advertiseTLS = ""
		} else {
			server.log().WithError(err).Warnf("[%s] Failed TLS handshake", client.RemoteIP)
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
			client.sendResponse(greeting)
			client.state = ClientCmd
		case ClientCmd:
			client.bufin.setLimit(CommandLineMaxLength)
			input, err := server.readCommand(client, sc.MaxSize)
			server.log().Debugf("Client sent: %s", input)
			if err == io.EOF {
				server.log().WithError(err).Warnf("Client closed the connection: %s", client.RemoteIP)
				return
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				server.log().WithError(err).Warnf("Timeout: %s", client.RemoteIP)
				return
			} else if err == LineLimitExceeded {
				client.sendResponse(response.Canned.FailLineTooLong)
				client.kill()
				break
			} else if err != nil {
				server.log().WithError(err).Warnf("Read error: %s", client.RemoteIP)
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
				client.sendResponse(helo)

			case strings.Index(cmd, "EHLO") == 0:
				client.Helo = strings.Trim(input[4:], " ")
				client.resetTransaction()
				client.sendResponse(ehlo,
					messageSize,
					pipelining,
					advertiseTLS,
					advertiseEnhancedStatusCodes,
					help)

			case strings.Index(cmd, "HELP") == 0:
				quote := response.GetQuote()
				client.sendResponse("214-OK\r\n" + quote)

			case sc.XClientOn && strings.Index(cmd, "XCLIENT ") == 0:
				if toks := strings.Split(input[8:], " "); len(toks) > 0 {
					for i := range toks {
						if vals := strings.Split(toks[i], "="); len(vals) == 2 {
							if vals[1] == "[UNAVAILABLE]" {
								// skip
								continue
							}
							if vals[0] == "ADDR" {
								client.RemoteIP = vals[1]
							}
							if vals[0] == "HELO" {
								client.Helo = vals[1]
							}
						}
					}
				}
				client.sendResponse(response.Canned.SuccessMailCmd)
			case strings.Index(cmd, "MAIL FROM:") == 0:
				if client.isInTransaction() {
					client.sendResponse(response.Canned.FailNestedMailCmd)
					break
				}
				addr := input[10:]
				if !(strings.Index(addr, "<>") == 0) &&
					!(strings.Index(addr, " <>") == 0) {
					// Not Bounce, extract mail.
					if from, err := extractEmail(addr); err != nil {
						client.sendResponse(err)
						break
					} else {
						client.MailFrom = from
					}

				} else {
					// bounce has empty from address
					client.MailFrom = mail.Address{}
				}
				client.sendResponse(response.Canned.SuccessMailCmd)

			case strings.Index(cmd, "RCPT TO:") == 0:
				if len(client.RcptTo) > RFC2821LimitRecipients {
					client.sendResponse(response.Canned.ErrorTooManyRecipients)
					break
				}
				to, err := extractEmail(input[8:])
				if err != nil {
					client.sendResponse(err.Error())
				} else {
					if !server.allowsHost(to.Host) {
						client.sendResponse(response.Canned.ErrorRelayDenied, to.Host)
					} else {
						client.PushRcpt(to)
						rcptError := server.backend().ValidateRcpt(client.Envelope)
						if rcptError != nil {
							client.PopRcpt()
							client.sendResponse(response.Canned.FailRcptCmd + " " + rcptError.Error())
						} else {
							client.sendResponse(response.Canned.SuccessRcptCmd)
						}
					}
				}

			case strings.Index(cmd, "RSET") == 0:
				client.resetTransaction()
				client.sendResponse(response.Canned.SuccessResetCmd)

			case strings.Index(cmd, "VRFY") == 0:
				client.sendResponse(response.Canned.SuccessVerifyCmd)

			case strings.Index(cmd, "NOOP") == 0:
				client.sendResponse(response.Canned.SuccessNoopCmd)

			case strings.Index(cmd, "QUIT") == 0:
				client.sendResponse(response.Canned.SuccessQuitCmd)
				client.kill()

			case strings.Index(cmd, "DATA") == 0:
				if len(client.RcptTo) == 0 {
					client.sendResponse(response.Canned.FailNoRecipientsDataCmd)
					break
				}
				client.sendResponse(response.Canned.SuccessDataCmd)
				client.state = ClientData

			case sc.StartTLSOn && strings.Index(cmd, "STARTTLS") == 0:

				client.sendResponse(response.Canned.SuccessStartTLSCmd)
				client.state = ClientStartTLS
			default:
				client.errors++
				if client.errors >= MaxUnrecognizedCommands {
					client.sendResponse(response.Canned.FailMaxUnrecognizedCmd)
					client.kill()
				} else {
					client.sendResponse(response.Canned.FailUnrecognizedCmd)
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
					client.sendResponse(response.Canned.FailReadLimitExceededDataCmd, LineLimitExceeded.Error())
					client.kill()
				} else if err == MessageSizeExceeded {
					client.sendResponse(response.Canned.FailMessageSizeExceeded, MessageSizeExceeded.Error())
					client.kill()
				} else {
					client.sendResponse(response.Canned.FailReadErrorDataCmd, err.Error())
					client.kill()
				}
				server.log().WithError(err).Warn("Error reading data")
				client.resetTransaction()
				break
			}

			res := server.backend().Process(client.Envelope)
			if res.Code() < 300 {
				client.messagesSent++
			}
			client.sendResponse(res.String())
			client.state = ClientCmd
			if server.isShuttingDown() {
				client.state = ClientShutdown
			}
			client.resetTransaction()

		case ClientStartTLS:
			if !client.TLS && sc.StartTLSOn {
				tlsConfig, ok := server.tlsConfigStore.Load().(*tls.Config)
				if !ok {
					server.mainlog().Error("Failed to load *tls.Config")
				} else if err := client.upgradeToTLS(tlsConfig); err == nil {
					advertiseTLS = ""
					client.resetTransaction()
				} else {
					server.log().WithError(err).Warnf("[%s] Failed TLS handshake", client.RemoteIP)
					// Don't disconnect, let the client decide if it wants to continue
				}
			}
			// change to command state
			client.state = ClientCmd
		case ClientShutdown:
			// shutdown state
			client.sendResponse(response.Canned.ErrorShutdown)
			client.kill()
		}

		if client.bufout.Buffered() > 0 {
			if server.log().IsDebug() {
				server.log().Debugf("Writing response to client: \n%s", client.response.String())
			}
			err := server.flushResponse(client)
			if err != nil {
				server.log().WithError(err).Debug("Error writing response")
				return
			}
		}

	}
}

func (s *server) log() log.Logger {
	if l, ok := s.logStore.Load().(log.Logger); ok {
		return l
	}
	l, err := log.GetLogger(log.OutputStderr.String(), log.InfoLevel.String())
	if err == nil {
		s.logStore.Store(l)
	}
	return l
}

func (s *server) mainlog() log.Logger {
	if l, ok := s.mainlogStore.Load().(log.Logger); ok {
		return l
	}
	l, err := log.GetLogger(log.OutputStderr.String(), log.InfoLevel.String())
	if err == nil {
		s.mainlogStore.Store(l)
	}
	return l
}
