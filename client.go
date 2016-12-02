package guerrilla

import (
	"bufio"
	"net"
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
	state ClientState
	// Message sent in HELO command
	helo string
	// Sender
	mailFrom *EmailParts
	// Recipients
	rcptTo []*EmailParts
	// Response from the server to be written to the client
	response    string
	address     string
	data        string
	connectedAt time.Time
	tls         bool
	conn        net.Conn
	bufin       *SMTPBufferedReader
	bufout      *bufio.Writer
	killedAt    time.Time
	errors      int
	id          int64
}

func (c *Client) responseAdd(r string) {
	c.response = c.response + r + "\r\n"
}

func (c *Client) reset() {
	c.mailFrom = &EmailParts{}
	c.rcptTo = []*EmailParts{}
}

func (c *Client) kill() {
	c.killedAt = time.Now()
}
