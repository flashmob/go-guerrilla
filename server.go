package guerrilla

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/smtp"
	"github.com/flashmob/go-guerrilla/response"
)

const (
	CommandVerbMaxLength = 16
	CommandLineMaxLength = 1024
	// Number of allowed unrecognized commands before we terminate the connection
	MaxUnrecognizedCommands = 5
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
	serverID        int
	clientPool      *Pool
	wg              sync.WaitGroup // for waiting to shutdown
	listener        net.Listener
	closedListener  chan bool
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
	wildcards  []string        // host wildcard list (* is used as a wildcard)
	sync.Mutex                 // guard access to the map
}

type command []byte

var (
	cmdHELO     command = []byte("HELO")
	cmdEHLO     command = []byte("EHLO")
	cmdHELP     command = []byte("HELP")
	cmdXCLIENT  command = []byte("XCLIENT")
	cmdMAIL     command = []byte("MAIL FROM:")
	cmdRCPT     command = []byte("RCPT TO:")
	cmdRSET     command = []byte("RSET")
	cmdVRFY     command = []byte("VRFY")
	cmdNOOP     command = []byte("NOOP")
	cmdQUIT     command = []byte("QUIT")
	cmdDATA     command = []byte("DATA")
	cmdSTARTTLS command = []byte("STARTTLS")
)

func (c command) match(in []byte) bool {
	return bytes.Index(in, c) == 0
}

// Creates and returns a new ready-to-run Server from a ServerConfig configuration
func newServer(sc *ServerConfig, b backends.Backend, mainlog log.Logger, serverID int) (*server, error) {
	server := &server{
		clientPool:      NewPool(sc.MaxClients),
		closedListener:  make(chan bool, 1),
		listenInterface: sc.ListenInterface,
		serverID:        serverID,
		state:           ServerStateNew,
		envelopePool:    mail.NewPool(sc.MaxClients * 2),
	}
	server.mainlogStore.Store(mainlog)
	server.backendStore.Store(b)
	if sc.LogFile == "" {
		// none set, use the mainlog for the server log
		server.logStore.Store(mainlog)
		server.log().Fields("iface", sc.ListenInterface).Info("server did not configure a separate log file, so using the main log")
	} else {
		// set level to same level as mainlog level
		if l, logOpenError := log.GetLogger(sc.LogFile, server.mainlog().GetLevel()); logOpenError != nil {
			server.log().Fields("error", logOpenError, "iface", sc.ListenInterface).Error("Failed creating a logger for server")
			return server, logOpenError
		} else {
			server.logStore.Store(l)
		}
	}
	server.setConfig(sc)
	server.setTimeout(sc.Timeout)
	if err := server.configureTLS(); err != nil {
		return server, err
	}
	return server, nil
}

func (s *server) configureTLS() error {
	sConfig := s.configStore.Load().(ServerConfig)
	if sConfig.TLS.AlwaysOn || sConfig.TLS.StartTLSOn {
		cert, err := tls.LoadX509KeyPair(sConfig.TLS.PublicKeyFile, sConfig.TLS.PrivateKeyFile)
		if err != nil {
			return fmt.Errorf("error while loading the certificate: %s", err)
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.VerifyClientCertIfGiven,
			ServerName:   sConfig.Hostname,
		}
		if len(sConfig.TLS.Protocols) > 0 {
			if min, ok := TLSProtocols[sConfig.TLS.Protocols[0]]; ok {
				tlsConfig.MinVersion = min
			}
		}
		if len(sConfig.TLS.Protocols) > 1 {
			if max, ok := TLSProtocols[sConfig.TLS.Protocols[1]]; ok {
				tlsConfig.MaxVersion = max
			}
		}
		if len(sConfig.TLS.Ciphers) > 0 {
			for _, val := range sConfig.TLS.Ciphers {
				if c, ok := TLSCiphers[val]; ok {
					tlsConfig.CipherSuites = append(tlsConfig.CipherSuites, c)
				}
			}
		}
		if len(sConfig.TLS.Curves) > 0 {
			for _, val := range sConfig.TLS.Curves {
				if c, ok := TLSCurves[val]; ok {
					tlsConfig.CurvePreferences = append(tlsConfig.CurvePreferences, c)
				}
			}
		}
		if len(sConfig.TLS.RootCAs) > 0 {
			caCert, err := ioutil.ReadFile(sConfig.TLS.RootCAs)
			if err != nil {
				s.log().Fields("error", err, "file", sConfig.TLS.RootCAs).Error("failed opening TLSRootCAs file")
				return err
			} else {
				caCertPool := x509.NewCertPool()
				caCertPool.AppendCertsFromPEM(caCert)
				tlsConfig.RootCAs = caCertPool
			}

		}
		if len(sConfig.TLS.ClientAuthType) > 0 {
			if ca, ok := TLSClientAuthTypes[sConfig.TLS.ClientAuthType]; ok {
				tlsConfig.ClientAuth = ca
			}
		}
		tlsConfig.PreferServerCipherSuites = sConfig.TLS.PreferServerCipherSuites
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
func (s *server) setTimeout(seconds int) {
	duration := time.Duration(int64(seconds))
	s.clientPool.SetTimeout(duration)
	s.timeout.Store(duration)
}

// goroutine safe config store
func (s *server) setConfig(sc *ServerConfig) {
	s.configStore.Store(*sc)
}

// goroutine safe
func (s *server) isEnabled() bool {
	sc := s.configStore.Load().(ServerConfig)
	return sc.IsEnabled
}

// Set the allowed hosts for the server
func (s *server) setAllowedHosts(allowedHosts []string) {
	s.hosts.Lock()
	defer s.hosts.Unlock()
	s.hosts.table = make(map[string]bool, len(allowedHosts))
	s.hosts.wildcards = nil
	for _, h := range allowedHosts {
		if strings.Contains(h, "*") {
			s.hosts.wildcards = append(s.hosts.wildcards, strings.ToLower(h))
		} else if len(h) > 5 && h[0] == '[' && h[len(h)-1] == ']' {
			if ip := net.ParseIP(h[1 : len(h)-1]); ip != nil {
				// this will save the normalized ip, as ip.String always returns ipv6 in short form
				s.hosts.table["["+ip.String()+"]"] = true
			}
		} else {
			s.hosts.table[strings.ToLower(h)] = true
		}
	}
}

// Begin accepting SMTP clients. Will block unless there is an error or server.Shutdown() is called
func (s *server) Start(startWG *sync.WaitGroup) error {
	var clientID uint64
	clientID = 0

	listener, err := net.Listen("tcp", s.listenInterface)
	s.listener = listener
	if err != nil {
		startWG.Done() // don't wait for me
		s.state = ServerStateStartError
		return fmt.Errorf("[%s] cannot listen on port: %s ", s.listenInterface, err.Error())
	}

	s.log().Fields("iface", s.listenInterface, "serverID", s.serverID).Info("listening on TCP")
	s.state = ServerStateRunning
	startWG.Done() // start successful, don't wait for me

	for {
		s.log().Fields("serverID", s.serverID, "nextSeq", clientID+1, "iface", s.listenInterface).
			Debug("waiting for a new client")
		conn, err := listener.Accept()
		clientID++
		if err != nil {
			if e, ok := err.(net.Error); ok && !e.Temporary() {
				s.log().Fields("iface", s.listenInterface, "serverID", s.serverID).Info("server has stopped accepting new clients")
				// the listener has been closed, wait for clients to exit
				s.log().Fields("iface", s.listenInterface, "serverID", s.serverID).Info("shutting down pool")
				s.clientPool.ShutdownState()
				s.clientPool.ShutdownWait()
				s.state = ServerStateStopped
				s.closedListener <- true
				return nil
			}
			s.mainlog().Fields("error", err, "serverID", s.serverID).Error("temporary error accepting client")
			continue
		}
		go func(p Poolable, borrowErr error) {
			c := p.(*client)
			if borrowErr == nil {
				s.handleClient(c)
				s.envelopePool.Return(c.Envelope)
				s.clientPool.Return(c)
			} else {
				s.log().Fields("error", borrowErr, "serverID", s.serverID).Error("couldn't borrow a new client")
				// we could not get a client, so close the connection.
				_ = conn.Close()
			}
			// intentionally placed Borrow in args so that it's called in the
			// same main goroutine.
		}(s.clientPool.Borrow(conn, clientID, s.log(), s.envelopePool, s.serverID))
	}
}

func (s *server) Shutdown() {
	if s.listener != nil {
		// This will cause Start function to return, by causing an error on listener.Accept
		_ = s.listener.Close()
		// wait for the listener to listener.Accept
		<-s.closedListener
		// At this point Start will exit and close down the pool
	} else {
		s.clientPool.ShutdownState()
		// listener already closed, wait for clients to exit
		s.clientPool.ShutdownWait()
		s.state = ServerStateStopped
	}
}

func (s *server) GetActiveClientsCount() int {
	return s.clientPool.GetActiveClientsCount()
}

// Verifies that the host is a valid recipient.
// host checking turned off if there is a single entry and it's a dot.
func (s *server) allowsHost(host string) bool {
	s.hosts.Lock()
	defer s.hosts.Unlock()
	// if hosts contains a single dot, further processing is skipped
	if len(s.hosts.table) == 1 {
		if _, ok := s.hosts.table["."]; ok {
			return true
		}
	}
	if _, ok := s.hosts.table[strings.ToLower(host)]; ok {
		return true
	}
	// check the wildcards
	for _, w := range s.hosts.wildcards {
		if matched, err := filepath.Match(w, strings.ToLower(host)); matched && err == nil {
			return true
		}
	}
	return false
}

func (s *server) allowsIp(ip net.IP) bool {
	ipStr := ip.String()
	return s.allowsHost("[" + ipStr + "]")
}

const commandSuffix = "\r\n"

// Reads from the client until a \n terminator is encountered,
// or until a timeout occurs.
func (s *server) readCommand(client *client) ([]byte, error) {
	//var input string
	var err error
	var bs []byte
	// In command state, stop reading at line breaks
	bs, err = client.bufin.ReadSlice('\n')
	if err != nil {
		return bs, err
	} else if bytes.HasSuffix(bs, []byte(commandSuffix)) {
		return bs[:len(bs)-2], err
	}
	return bs[:len(bs)-1], err
}

// flushResponse a response to the client. Flushes the client.bufout buffer to the connection
func (s *server) flushResponse(client *client) error {
	if err := client.setTimeout(s.timeout.Load().(time.Duration)); err != nil {
		return err
	}
	return client.bufout.Flush()
}

func (s *server) isShuttingDown() bool {
	return s.clientPool.IsShuttingDown()
}

const advertisePipelining = "250-PIPELINING\r\n"
const advertiseStartTLS = "250-STARTTLS\r\n"
const advertiseEnhancedStatusCodes = "250-ENHANCEDSTATUSCODES\r\n"
const advertise8BitMime = "250-8BITMIME\r\n"

// The last line doesn't need \r\n since string will be printed as a new line.
// Also, Last line has no dash -
const advertiseHelp = "250 HELP"

// handleClient handles an entire client SMTP exchange
func (s *server) handleClient(client *client) {
	defer func() {
		s.log().Fields(
			"peer", client.RemoteIP,
			"event", "disconnect",
			"id", client.ID,
			"queuedID", client.QueuedId,
		).Info("Disconnect client")
		client.closeConn()
	}()
	sc := s.configStore.Load().(ServerConfig)
	s.log().Fields(
		"peer", client.RemoteIP,
		"id", client.ID,
		"event", "connect",
		"queuedID", client.QueuedId,
	).Info("handle client")

	// Initial greeting
	greeting := fmt.Sprintf("220 %s SMTP Guerrilla(%s) #%d (%d) %s",
		sc.Hostname, Version, client.ID,
		s.clientPool.GetActiveClientsCount(), time.Now().Format(time.RFC3339))

	helo := fmt.Sprintf("250 %s Hello", sc.Hostname)
	// ehlo is a multi-line reply and need additional \r\n at the end
	ehlo := fmt.Sprintf("250-%s Hello\r\n", sc.Hostname)

	// Extended feature advertisements
	messageSize := fmt.Sprintf("250-SIZE %d\r\n", sc.MaxSize)
	advertiseTLS := advertiseStartTLS
	// The last line doesn't need \r\n since string will be printed as a new line.
	// Also, Last line has no dash -

	if sc.TLS.AlwaysOn {
		tlsConfig, ok := s.tlsConfigStore.Load().(*tls.Config)
		if !ok {
			s.mainlog().Error("Failed to load *tls.Config")
		} else if err := client.upgradeToTLS(tlsConfig); err == nil {
			advertiseTLS = ""
		} else {
			s.log().Fields("error", err, "peer", client.RemoteIP).Warn("failed TLS handshake")
			// server requires TLS, but can't handshake
			client.kill()
		}
	}
	if !sc.TLS.StartTLSOn {
		// STARTTLS turned off, don't advertise it
		advertiseTLS = ""
	}
	r := response.Canned
	for client.isAlive() {
		switch client.state {
		case ClientGreeting:
			client.sendResponse(greeting)
			client.state = ClientCmd
		case ClientCmd:
			client.bufin.setLimit(CommandLineMaxLength)
			input, err := s.readCommand(client)
			s.log().Fields("input", string(input)).Debug("client said")
			if err == io.EOF {
				s.log().Fields("error", err, "peer", client.RemoteIP).Warn("client closed the connection")
				return
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				s.log().Fields("error", err, "peer", client.RemoteIP).Warn("timeout")
				return
			} else if err == LineLimitExceeded {
				client.sendResponse(r.FailLineTooLong)
				client.kill()
				break
			} else if err != nil {
				s.log().Fields("error", err, "peer", client.RemoteIP).Warn("read error")
				client.kill()
				break
			}
			if s.isShuttingDown() {
				client.state = ClientShutdown
				continue
			}

			cmdLen := len(input)
			if cmdLen > CommandVerbMaxLength {
				cmdLen = CommandVerbMaxLength
			}
			cmd := bytes.ToUpper(input[:cmdLen])
			switch {
			case cmdHELO.match(cmd):
				if h, err := client.parser.Helo(input[4:]); err == nil {
					client.Helo = h
				} else {
					s.log().Fields("helo", h, "seq", client.ID).Warn("invalid helo")
					client.sendResponse(r.FailSyntaxError)
					break
				}
				client.resetTransaction()
				client.sendResponse(helo)

			case cmdEHLO.match(cmd):
				if h, _, err := client.parser.Ehlo(input[4:]); err == nil {
					client.Helo = h
				} else {
					client.sendResponse(r.FailSyntaxError)
					s.log().Fields("ehlo", h, "seq", client.ID).Warn("invalid ehlo")
					client.sendResponse(r.FailSyntaxError)
					break
				}
				client.ESMTP = true
				client.resetTransaction()
				client.sendResponse(ehlo,
					messageSize,
					advertisePipelining,
					advertiseTLS,
					advertiseEnhancedStatusCodes,
					advertise8BitMime,
					advertiseHelp)

			case cmdHELP.match(cmd):
				quote := response.GetQuote()
				client.sendResponse("214-OK\r\n", quote)

			case sc.XClientOn && cmdXCLIENT.match(cmd):
				if toks := bytes.Split(input[8:], []byte{' '}); len(toks) > 0 {
					for i := range toks {
						if vals := bytes.Split(toks[i], []byte{'='}); len(vals) == 2 {
							if bytes.Equal(vals[1], []byte("[UNAVAILABLE]")) {
								// skip
								continue
							}
							if bytes.Equal(vals[0], []byte("ADDR")) {
								client.RemoteIP = string(vals[1])
							}
							if bytes.Equal(vals[0], []byte("HELO")) {
								client.Helo = string(vals[1])
							}
						}
					}
				}
				client.sendResponse(r.SuccessMailCmd)
			case cmdMAIL.match(cmd):
				if client.isInTransaction() {
					client.sendResponse(r.FailNestedMailCmd)
					break
				}
				client.MailFrom, err = client.parsePath(input[10:], client.parser.MailFrom)
				if err != nil {
					s.log().Fields("error", err, "raw", string(input[10:])).Error("MAIL parse error")
					client.sendResponse(err)
					break
				} else if client.parser.NullPath {
					// bounce has empty from address
					client.MailFrom = mail.Address{}
				} else {
					s.log().Fields(
						"event", "mailfrom",
						"helo", client.Helo,
						"domain", client.MailFrom.Host,
						"address", client.RemoteIP,
						"id", client.ID,
						"queuedID", client.QueuedId,
					).Info("mail from")
				}
				client.TransportType = smtp.TransportTypeUnspecified
				for i := range client.MailFrom.PathParams {
					if tt := client.MailFrom.PathParams[i].Transport(); tt != smtp.TransportTypeUnspecified {
						client.TransportType = tt
						if tt == smtp.TransportTypeInvalid {
							continue
						}
						break
					}
				}
				client.sendResponse(r.SuccessMailCmd)

			case cmdRCPT.match(cmd):
				if len(client.RcptTo) > smtp.LimitRecipients {
					client.sendResponse(r.ErrorTooManyRecipients)
					break
				}
				to, err := client.parsePath(input[8:], client.parser.RcptTo)
				if err != nil {
					s.log().Fields("error", err, "raw", string(input[8:])).Error("RCPT parse error")
					client.sendResponse(err.Error())
					break
				}
				s.defaultHost(&to)
				if (to.IP != nil && !s.allowsIp(to.IP)) || (to.IP == nil && !s.allowsHost(to.Host)) {
					client.sendResponse(r.ErrorRelayDenied, " ", to.Host)
				} else {
					client.PushRcpt(to)
					rcptError := s.backend().ValidateRcpt(client.Envelope)
					if rcptError != nil {
						client.PopRcpt()
						client.sendResponse(r.FailRcptCmd, " ", rcptError.Error())
					} else {
						client.sendResponse(r.SuccessRcptCmd)
					}
				}

			case cmdRSET.match(cmd):
				client.resetTransaction()
				client.sendResponse(r.SuccessResetCmd)

			case cmdVRFY.match(cmd):
				client.sendResponse(r.SuccessVerifyCmd)

			case cmdNOOP.match(cmd):
				client.sendResponse(r.SuccessNoopCmd)

			case cmdQUIT.match(cmd):
				client.sendResponse(r.SuccessQuitCmd)
				client.kill()

			case cmdDATA.match(cmd):
				if len(client.RcptTo) == 0 {
					client.sendResponse(r.FailNoRecipientsDataCmd)
					break
				}
				client.sendResponse(r.SuccessDataCmd)
				client.state = ClientData

			case sc.TLS.StartTLSOn && cmdSTARTTLS.match(cmd):

				client.sendResponse(r.SuccessStartTLSCmd)
				client.state = ClientStartTLS
			default:
				client.errors++
				if client.errors >= MaxUnrecognizedCommands {
					client.sendResponse(r.FailMaxUnrecognizedCmd)
					client.kill()
				} else {
					client.sendResponse(r.FailUnrecognizedCmd)
				}
			}
		case ClientData:
			// intentionally placed the limit 1MB above so that reading does not return with an error
			// if the client goes a little over. Anything above will err
			client.bufin.setLimit(sc.MaxSize + 1024000) // This a hard limit.
			be := s.backend()
			var (
				n   int64
				err error
				res backends.Result
			)
			fields := []interface{}{
				"event", "data",
				"id", client.ID,
				"queuedID", client.QueuedId,
				"messageID", client.MessageID,
				"peer", client.RemoteIP,
				"serverID", s.serverID,
			}
			s.log().Fields(fields...).Info("receive DATA")
			if be.StreamOn() {
				// process the message as a stream
				res, n, err = be.ProcessStream(client.smtpReader.DotReader(), client.Envelope)
				if err == nil && res.Code() < 300 {
					e := s.envelopePool.Borrow(
						client.Envelope.RemoteIP,
						client.ID,
						client.Envelope.ServerID,
					)
					s.copyEnvelope(client.Envelope, e)
					// process in the background then return the envelope
					go func() {
						be.ProcessBackground(e)
						s.envelopePool.Return(e)
					}()
				}
			} else {
				// or buffer the entire message (parse headers & mime structure as we go along)
				n, err = client.Data.ReadFrom(client.smtpReader)
				if n > sc.MaxSize {
					err = fmt.Errorf("maximum DATA size exceeded (%d)", sc.MaxSize)
				} else {
					if p := client.smtpReader.Parts(); p != nil && len(p) > 0 {
						client.Envelope.Header = p[0].Headers
					}
				}
				// All done. we can close the smtpReader, the protocol will reset the transaction, expecting a new message
				if closeErr := client.smtpReader.Close(); closeErr != nil {
					s.log().WithError(closeErr).Error("could not close DATA reader")
				}
			}

			if err != nil {
				if err == LineLimitExceeded {
					client.sendResponse(r.FailReadLimitExceededDataCmd, " ", LineLimitExceeded.Error())
					client.kill()
				} else if err == MessageSizeExceeded {
					client.sendResponse(r.FailMessageSizeExceeded, " ", MessageSizeExceeded.Error())
					client.kill()
				} else {
					client.sendResponse(r.FailReadErrorDataCmd, " ", err.Error())
					client.kill()
				}
				s.log().Fields(append(fields, "error", err)...).Error("error reading DATA")
				client.resetTransaction()
				break
			}

			if !be.StreamOn() {
				res = be.Process(client.Envelope)
			}

			if res.Code() < 300 {
				client.messagesSent++
				s.log().Fields(append(fields, "length", n)...).Info("received message DATA")
			}
			client.sendResponse(res)
			client.state = ClientCmd
			if s.isShuttingDown() {
				client.state = ClientShutdown
			}
			client.resetTransaction()

		case ClientStartTLS:
			if !client.TLS && sc.TLS.StartTLSOn {
				tlsConfig, ok := s.tlsConfigStore.Load().(*tls.Config)
				if !ok {
					s.mainlog().Fields("iface", s.listenInterface).Error("failed to load *tls.Config")
				} else if err := client.upgradeToTLS(tlsConfig); err == nil {
					advertiseTLS = ""
					client.resetTransaction()
				} else {
					s.log().Fields("error", err, "iface", s.listenInterface, "ip", client.RemoteIP).
						Warn("failed TLS handshake")
					// Don't disconnect, let the client decide if it wants to continue
				}
			}
			// change to command state
			client.state = ClientCmd
		case ClientShutdown:
			// shutdown state
			client.sendResponse(r.ErrorShutdown)
			client.kill()
		}

		if client.bufErr != nil {
			s.log().WithError(client.bufErr).Debug("client could not buffer a response")
			return
		}
		// flush the response buffer
		if client.bufout.Buffered() > 0 {
			if s.log().IsDebug() {
				s.log().Fields("out", client.response.String()).Debug("writing response to client")
			}
			err := s.flushResponse(client)
			if err != nil {
				s.log().WithError(err).Debug("error writing response")
				return
			}
		}

	}
}

func (s *server) log() log.Logger {
	return s.loadLog(&s.logStore)
}

func (s *server) mainlog() log.Logger {
	return s.loadLog(&s.mainlogStore)
}

func (s *server) loadLog(value *atomic.Value) log.Logger {
	if l, ok := value.Load().(log.Logger); ok {
		return l
	}
	out := log.OutputStderr.String()
	level := log.InfoLevel.String()
	if value == &s.logStore {
		if sc, ok := s.configStore.Load().(ServerConfig); ok && sc.LogFile != "" {
			out = sc.LogFile
		}
		level = s.mainlog().GetLevel()
	}

	l, err := log.GetLogger(out, level)
	if err == nil {
		value.Store(l)
	}
	return l
}

// defaultHost ensures that the host attribute is set, if addressed to Postmaster
func (s *server) defaultHost(a *mail.Address) {
	if a.Host == "" && a.IsPostmaster() {
		sc := s.configStore.Load().(ServerConfig)
		a.Host = sc.Hostname
		if !s.allowsHost(a.Host) {
			s.log().Fields("hostname", sc.Hostname).
				Warn("the hostname is not present in the AllowedHosts config setting")
		}
	}
}

func (s *server) copyEnvelope(src *mail.Envelope, dest *mail.Envelope) {
	dest.TLS = src.TLS
	dest.Helo = src.Helo
	dest.ESMTP = src.ESMTP
	dest.RcptTo = src.RcptTo
	dest.MailFrom = src.MailFrom
	dest.MessageID = src.MessageID
	dest.TransportType = src.TransportType
}
