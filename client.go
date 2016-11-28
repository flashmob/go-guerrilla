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
)

type Client struct {
	state    ClientState
	helo     string
	mailFrom *EmailParts
	rcptTo   []*EmailParts
	response string
	address  string
	data     string
	subject  string
	// Hash        string
	connectedAt time.Time
	tls         bool
	conn        net.Conn
	bufin       *SMTPBufferedReader
	bufout      *bufio.Writer
	killedAt    time.Time
	errors      int
	id          int64
	// SavedNotify chan int
}

func (c *Client) responseAdd(r string) {
	c.response = c.response + "\r\n" + r
}
