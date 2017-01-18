package guerrilla

import (
	"bufio"
	"crypto/tls"
	log "github.com/Sirupsen/logrus"
	"github.com/flashmob/go-guerrilla/envelope"
	"net"
	"net/textproto"
	"strings"
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
	response   string
	conn       net.Conn
	bufin      *smtpBufferedReader
	bufout     *bufio.Writer
	smtpReader *textproto.Reader
	ar         *adjustableLimitedReader
	// guards access to conn
	connGuard sync.Mutex
}

func NewClient(conn net.Conn, clientID uint64) *client {
	c := &client{
		conn: conn,
		Envelope: &envelope.Envelope{
			RemoteAddress: conn.RemoteAddr().String(),
		},
		ConnectedAt: time.Now(),
		bufin:       newSMTPBufferedReader(conn),
		bufout:      bufio.NewWriter(conn),
		ID:          clientID,
	}

	c.smtpReader = textproto.NewReader(c.bufin.Reader)
	return c
}

func (c *client) responseAdd(r string) {
	c.response = c.response + r + "\r\n"
}

func (c *client) resetTransaction() {
	c.MailFrom = &envelope.EmailAddress{}
	c.RcptTo = []envelope.EmailAddress{}
	c.Data.Reset()
	c.Subject = ""
}

func (c *client) isInTransaction() bool {
	isMailFromEmpty := (c.MailFrom == nil || *c.MailFrom == (envelope.EmailAddress{}))
	if isMailFromEmpty {
		return false
	}
	return true
}

func (c *client) kill() {
	c.KilledAt = time.Now()
}

func (c *client) isAlive() bool {
	return c.KilledAt.IsZero()
}

func (c *client) scanSubject(reply string) {
	if c.Subject == "" && (len(reply) > 8) {
		test := strings.ToUpper(reply[0:9])
		if i := strings.Index(test, "SUBJECT: "); i == 0 {
			// first line with \r\n
			c.Subject = reply[9:]
		}
	} else if strings.HasSuffix(c.Subject, "\r\n") {
		// chop off the \r\n
		c.Subject = c.Subject[0 : len(c.Subject)-2]
		if (strings.HasPrefix(reply, " ")) || (strings.HasPrefix(reply, "\t")) {
			// subject is multi-line
			c.Subject = c.Subject + reply[1:]
		}
	}
}

// setTimeout adjust the timeout on the connection, goroutine safe
func (c *client) setTimeout(t time.Duration) {
	defer c.connGuard.Unlock()
	c.connGuard.Lock()
	if c.conn != nil {
		c.conn.SetDeadline(time.Now().Add(t * time.Second))
	}
}

// Closes a client connection, , goroutine safe
func (c *client) closeConn() {
	defer c.connGuard.Unlock()
	c.connGuard.Lock()
	c.conn.Close()
	c.conn = nil
}

func (c *client) init(conn net.Conn, clientID uint64) {
	c.conn = conn
	// reset our reader & writer
	c.bufout.Reset(conn)
	c.bufin.Reset(conn)
	c.Data.Reset()

	//br := bufio.NewReader(newAdjustableLimitedReader(conn, 267))
	//c.smtpReader = textproto.NewReader(br)
	// reset session data
	c.state = 0
	c.KilledAt = time.Time{}
	c.ConnectedAt = time.Now()
	c.ID = clientID
	c.TLS = false
	c.errors = 0
	c.response = ""
	c.Helo = ""
}

func (c *client) getID() uint64 {
	return c.ID
}

// Upgrades a client connection to TLS
func (client *client) upgradeToTLS(tlsConfig *tls.Config) bool {
	var tlsConn *tls.Conn
	// load the config thread-safely
	tlsConn = tls.Server(client.conn, tlsConfig)
	err := tlsConn.Handshake()
	if err != nil {
		log.WithError(err).Warnf("[%s] Failed TLS handshake", client.RemoteAddress)
		return false
	}
	client.conn = net.Conn(tlsConn)
	client.bufout.Reset(client.conn)
	client.bufin.Reset(client.conn)
	client.TLS = true

	return true
}
