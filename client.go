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
	ClientHandshake = iota
	// We have responded to the client's connection and are awaiting a command
	ClientCmd
	// We have recieved the sender and recipient information
	ClientData
	// We have agreed with the client to secure the connection over TLS
	ClientStartTLS
)

type Client struct {
	ID int64
	// Message sent in HELO command
	Helo string
	// Sender
	MailFrom *EmailParts
	// Recipients
	RcptTo       []*EmailParts
	Address      string
	Data         string
	Subject      string
	Hash         string
	ConnectedAt  time.Time
	KilledAt     time.Time
	TLS          bool
	Errors       int
	state        ClientState
	messagesSent int
	// Response to be written to the client
	response string
	conn     net.Conn
	bufin    *smtpBufferedReader
	bufout   *bufio.Writer
}

func (c *Client) responseAdd(r string) {
	c.response = c.response + r + "\r\n"
}

func (c *Client) reset() {
	c.MailFrom = &EmailParts{}
	c.RcptTo = []*EmailParts{}
}

func (c *Client) kill() {
	c.KilledAt = time.Now()
}

func (c *Client) isAlive() bool {
	return c.KilledAt.IsZero()
}

func (c *Client) scanSubject(reply string) {
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
