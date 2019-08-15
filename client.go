package guerrilla

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"sync"
	"time"

	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/rfc5321"
	"github.com/flashmob/go-guerrilla/response"
)

// ClientState indicates which part of the SMTP transaction a given client is in.
type ClientState int

const (
	// The client has connected, and is awaiting our first response
	ClientGreeting = iota
	// We have responded to the client's connection and are awaiting a command
	ClientCmd
	// We have received the sender and recipient information
	ClientData
	// We have agreed with the client to secure the connection over TLS
	ClientStartTLS
	// Server will shutdown, client to shutdown on next command turn
	ClientShutdown
)

type client struct {
	*mail.Envelope
	ID          uint64
	ConnectedAt time.Time
	KilledAt    time.Time
	// Number of errors encountered during session with this client
	errors       int
	state        ClientState
	messagesSent int
	// Response to be written to the client (for debugging)
	response   bytes.Buffer
	bufErr     error
	conn       net.Conn
	bufin      *smtpBufferedReader
	bufout     *bufio.Writer
	smtpReader *textproto.Reader
	ar         *adjustableLimitedReader
	// guards access to conn
	connGuard sync.Mutex
	log       log.Logger
	parser    rfc5321.Parser
}

// NewClient allocates a new client.
func NewClient(conn net.Conn, clientID uint64, logger log.Logger, envelope *mail.Pool) *client {
	c := &client{
		conn: conn,
		// Envelope will be borrowed from the envelope pool
		// the envelope could be 'detached' from the client later when processing
		Envelope:    envelope.Borrow(getRemoteAddr(conn), clientID),
		ConnectedAt: time.Now(),
		bufin:       newSMTPBufferedReader(conn),
		bufout:      bufio.NewWriter(conn),
		ID:          clientID,
		log:         logger,
	}

	// used for reading the DATA state
	c.smtpReader = textproto.NewReader(c.bufin.Reader)
	return c
}

// sendResponse adds a response to be written on the next turn
// the response gets buffered
func (c *client) sendResponse(r ...interface{}) {
	c.bufout.Reset(c.conn)
	if c.log.IsDebug() {
		// an additional buffer so that we can log the response in debug mode only
		c.response.Reset()
	}
	var out string
	if c.bufErr != nil {
		c.bufErr = nil
	}
	for _, item := range r {
		switch v := item.(type) {
		case error:
			out = v.Error()
		case fmt.Stringer:
			out = v.String()
		case string:
			out = v
		}
		if _, c.bufErr = c.bufout.WriteString(out); c.bufErr != nil {
			c.log.WithError(c.bufErr).Error("could not write to c.bufout")
		}
		if c.log.IsDebug() {
			c.response.WriteString(out)
		}
		if c.bufErr != nil {
			return
		}
	}
	_, c.bufErr = c.bufout.WriteString("\r\n")
	if c.log.IsDebug() {
		c.response.WriteString("\r\n")
	}
}

// resetTransaction resets the SMTP transaction, ready for the next email (doesn't disconnect)
// Transaction ends on:
// -HELO/EHLO/REST command
// -End of DATA command
// TLS handshake
func (c *client) resetTransaction() {
	c.Envelope.ResetTransaction()
}

// isInTransaction returns true if the connection is inside a transaction.
// A transaction starts after a MAIL command gets issued by the client.
// Call resetTransaction to end the transaction
func (c *client) isInTransaction() bool {
	if len(c.MailFrom.User) == 0 && !c.MailFrom.NullPath {
		return false
	}
	return true
}

// kill flags the connection to close on the next turn
func (c *client) kill() {
	c.KilledAt = time.Now()
}

// isAlive returns true if the client is to close on the next turn
func (c *client) isAlive() bool {
	return c.KilledAt.IsZero()
}

// setTimeout adjust the timeout on the connection, goroutine safe
func (c *client) setTimeout(t time.Duration) (err error) {
	defer c.connGuard.Unlock()
	c.connGuard.Lock()
	if c.conn != nil {
		err = c.conn.SetDeadline(time.Now().Add(t * time.Second))
	}
	return
}

// closeConn closes a client connection, , goroutine safe
func (c *client) closeConn() {
	defer c.connGuard.Unlock()
	c.connGuard.Lock()
	_ = c.conn.Close()
	c.conn = nil
}

// init is called after the client is borrowed from the pool, to get it ready for the connection
func (c *client) init(conn net.Conn, clientID uint64, ep *mail.Pool) {
	c.conn = conn
	// reset our reader & writer
	c.bufout.Reset(conn)
	c.bufin.Reset(conn)
	// reset session data
	c.state = 0
	c.KilledAt = time.Time{}
	c.ConnectedAt = time.Now()
	c.ID = clientID
	c.errors = 0
	// borrow an envelope from the envelope pool
	c.Envelope = ep.Borrow(getRemoteAddr(conn), clientID)
}

// getID returns the client's unique ID
func (c *client) getID() uint64 {
	return c.ID
}

// UpgradeToTLS upgrades a client connection to TLS
func (c *client) upgradeToTLS(tlsConfig *tls.Config) error {
	// wrap c.conn in a new TLS server side connection
	tlsConn := tls.Server(c.conn, tlsConfig)
	// Call handshake here to get any handshake error before reading starts
	err := tlsConn.Handshake()
	if err != nil {
		return err
	}
	// convert tlsConn to net.Conn
	c.conn = net.Conn(tlsConn)
	c.bufout.Reset(c.conn)
	c.bufin.Reset(c.conn)
	c.TLS = true
	return err
}

func getRemoteAddr(conn net.Conn) string {
	if addr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		// we just want the IP (not the port)
		return addr.IP.String()
	} else {
		return conn.RemoteAddr().Network()
	}
}

type pathParser func([]byte) error

func (c *client) parsePath(in []byte, p pathParser) (mail.Address, error) {
	address := mail.Address{}
	var err error
	if len(in) > rfc5321.LimitPath {
		return address, errors.New(response.Canned.FailPathTooLong.String())
	}
	if err = p(in); err != nil {
		return address, errors.New(response.Canned.FailInvalidAddress.String())
	} else if c.parser.NullPath {
		// bounce has empty from address
		address = mail.Address{}
	} else if len(c.parser.LocalPart) > rfc5321.LimitLocalPart {
		err = errors.New(response.Canned.FailLocalPartTooLong.String())
	} else if len(c.parser.Domain) > rfc5321.LimitDomain {
		err = errors.New(response.Canned.FailDomainTooLong.String())
	} else {
		address = mail.Address{
			User:       c.parser.LocalPart,
			Host:       c.parser.Domain,
			ADL:        c.parser.ADL,
			PathParams: c.parser.PathParams,
			NullPath:   c.parser.NullPath,
		}
	}
	return address, err
}

func (s *server) rcptTo() (address mail.Address, err error) {
	return address, err
}
