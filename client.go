package guerrilla

import (
	"bufio"
	"errors"
	"fmt"
	"io"
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

// EmailParts encodes an email address of the form `<user@host>`
type EmailParts struct {
	User string
	Host string
}

func (ep *EmailParts) String() string {
	return fmt.Sprintf("%s@%s", ep.User, ep.Host)
}

func (ep *EmailParts) isEmpty() bool {
	return ep.User == "" && ep.Host == ""
}

var InputLimitExceeded = errors.New("Line too long")

// we need to adjust the limit, so we embed io.LimitedReader
type adjustableLimitedReader struct {
	R *io.LimitedReader
}

// bolt this on so we can adjust the limit
func (alr *adjustableLimitedReader) setLimit(n int64) {
	alr.R.N = n
}

// Returns a specific error when a limit is reached, that can be differentiated
// from an EOF error from the standard io.Reader.
func (alr *adjustableLimitedReader) Read(p []byte) (n int, err error) {
	n, err = alr.R.Read(p)
	if err == io.EOF && alr.R.N <= 0 {
		// return our custom error since io.Reader returns EOF
		err = InputLimitExceeded
	}
	return
}

func newAdjustableLimitedReader(r io.Reader, n int64) *adjustableLimitedReader {
	lr := &io.LimitedReader{R: r, N: n}
	return &adjustableLimitedReader{lr}
}

type SMTPBufferedReader struct {
	*bufio.Reader
	alr *adjustableLimitedReader
}

// Delegate to the adjustable limited reader
func (sbr *SMTPBufferedReader) setLimit(n int64) {
	sbr.alr.setLimit(n)
}

// Allocate a new SMTPBufferedReader
func NewSMTPBufferedReader(rd io.Reader) *SMTPBufferedReader {
	alr := newAdjustableLimitedReader(rd, CommandLineMaxLength)
	s := &SMTPBufferedReader{bufio.NewReader(alr), alr}
	return s
}
