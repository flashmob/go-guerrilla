package guerrilla

import (
	"bufio"
	"net"
	"strings"
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
)

type client struct {
	*MailData
	ID          int64
	ConnectedAt time.Time
	KilledAt    time.Time
	// Number of errors encountered during session with this client
	errors       int
	state        ClientState
	messagesSent int
	// Response to be written to the client
	response string
	conn     net.Conn
	bufin    *smtpBufferedReader
	bufout   *bufio.Writer
}

type MailData struct {
	Address string
	// Message sent in EHLO command
	Helo string
	// Sender
	MailFrom *EmailParts
	// Recipients
	RcptTo  []*EmailParts
	Data    string
	Subject string
	TLS     bool
}

func (c *client) responseAdd(r string) {
	c.response = c.response + r + "\r\n"
}

func (c *client) reset() {
	c.MailFrom = &EmailParts{}
	c.RcptTo = []*EmailParts{}
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
