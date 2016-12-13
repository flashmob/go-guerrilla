package guerrilla

import (
	"bufio"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

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
	Headers      map[string]string
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
	bufin    *SMTPBufferedReader
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

// First capturing group is header name, second is header value.
// Accounts for folding headers.
var headerRegex, _ = regexp.Compile(`^([\S ]+):([\S ]+(?:\r\n\s[\S ]+)?)`)

func (c *Client) parseHeaders() {
	var headerSectionEnds int
	for i, char := range c.Data[:len(c.Data)-4] {
		if char == '\r' {
			if c.Data[i+1] == '\n' && c.Data[i+2] == '\r' && c.Data[i+3] == '\n' {
				headerSectionEnds = i + 2
			}
		}
	}
	c.Headers = make(map[string]string, 5)
	// TODO header comments
	matches := headerRegex.FindAllStringSubmatch(c.Data[:headerSectionEnds], -1)
	for _, h := range matches {
		name := http.CanonicalHeaderKey(strings.TrimSpace(strings.Replace(h[1], "\r\n", "", -1)))
		val := strings.TrimSpace(strings.Replace(h[2], "\r\n", "", -1))
		c.Headers[name] = val
	}
}
