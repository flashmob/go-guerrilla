package guerrilla

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/flashmob/go-guerrilla/envelope"
	"github.com/flashmob/go-guerrilla/log"
	"net"
	"net/textproto"
	"sync"
	"time"
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
	*envelope.Envelope
	ID          uint64
	ConnectedAt time.Time
	KilledAt    time.Time
	// Number of errors encountered during session with this client
	errors       int
	state        ClientState
	messagesSent int
	// Response to be written to the client
	response   bytes.Buffer
	conn       net.Conn
	bufin      *smtpBufferedReader
	bufout     *bufio.Writer
	smtpReader *textproto.Reader
	ar         *adjustableLimitedReader
	// guards access to conn
	connGuard sync.Mutex
	log       log.Logger
}

// Allocate a new client
func NewClient(conn net.Conn, clientID uint64, logger log.Logger) *client {
	c := &client{
		conn: conn,
		Envelope: &envelope.Envelope{
			RemoteAddress: conn.RemoteAddr().String(),
		},
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

// setResponse adds a response to be written on the next turn
func (c *client) setResponse(r ...interface{}) {
	c.bufout.Reset(c.conn)
	if c.log.IsDebug() {
		// us additional buffer so that we can log the response in debug mode only
		c.response.Reset()
	}
	for _, item := range r {
		switch v := item.(type) {
		case string:
			c.bufout.WriteString(v)
			if c.log.IsDebug() {
				c.response.WriteString(v)
			}
		case error:
			c.bufout.WriteString(v.Error())
			if c.log.IsDebug() {
				c.response.WriteString(v.Error())
			}
		case fmt.Stringer:
			c.bufout.WriteString(v.String())
			if c.log.IsDebug() {
				c.response.WriteString(v.String())
			}
		}
	}
	c.bufout.WriteString("\r\n")
	if c.log.IsDebug() {
		c.response.WriteString("\r\n")
	}
}

// resetTransaction resets the SMTP transaction, ready for the next email (doesn't disconnect)
// Transaction ends on:
// -HELO/EHLO/REST command
// -End of DATA command
// TLS handhsake
func (c *client) resetTransaction() {
	c.MailFrom = &envelope.EmailAddress{}
	c.RcptTo = []envelope.EmailAddress{}
	c.Data.Reset()
	c.Subject = ""
	c.Header = nil
}

// isInTransaction returns true if the connection is inside a transaction.
// A transaction starts after a MAIL command gets issued by the client.
// Call resetTransaction to end the transaction
func (c *client) isInTransaction() bool {
	isMailFromEmpty := (c.MailFrom == nil || *c.MailFrom == (envelope.EmailAddress{}))
	if isMailFromEmpty {
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
func (c *client) setTimeout(t time.Duration) {
	defer c.connGuard.Unlock()
	c.connGuard.Lock()
	if c.conn != nil {
		c.conn.SetDeadline(time.Now().Add(t * time.Second))
	}
}

// closeConn closes a client connection, , goroutine safe
func (c *client) closeConn() {
	defer c.connGuard.Unlock()
	c.connGuard.Lock()
	c.conn.Close()
	c.conn = nil
}

// init is called after the client is borrowed from the pool, to get it ready for the connection
func (c *client) init(conn net.Conn, clientID uint64) {
	c.conn = conn
	// reset our reader & writer
	c.bufout.Reset(conn)
	c.bufin.Reset(conn)
	// reset the data buffer, keep it allocated
	c.Data.Reset()
	// reset session data
	c.state = 0
	c.KilledAt = time.Time{}
	c.ConnectedAt = time.Now()
	c.ID = clientID
	c.TLS = false
	c.errors = 0
	//c.response.Reset()
	c.Helo = ""
	c.Header = nil
	c.RemoteAddress = conn.RemoteAddr().String()
}

// getID returns the client's unique ID
func (c *client) getID() uint64 {
	return c.ID
}

// UpgradeToTLS upgrades a client connection to TLS
func (client *client) upgradeToTLS(tlsConfig *tls.Config) error {
	var tlsConn *tls.Conn
	// load the config thread-safely
	tlsConn = tls.Server(client.conn, tlsConfig)
	// Call handshake here to get any handshake error before reading starts
	err := tlsConn.Handshake()
	if err != nil {
		return err
	}
	// convert tlsConn to net.Conn
	client.conn = net.Conn(tlsConn)
	client.bufout.Reset(client.conn)
	client.bufin.Reset(client.conn)
	client.TLS = true
	return err
}
