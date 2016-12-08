package guerrilla

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
	"sync"
)

type EmailParts struct {
	User string
	Host string
}

func (ep *EmailParts) String() string {
	return fmt.Sprintf("%s@%s", ep.User, ep.Host)
}

// Backend accepts the received messages, and store/deliver/process them
type Backend interface {
	Initialize(BackendConfig) error
	Process(client *Client, from *EmailParts, to []*EmailParts) string
	Finalize() error
}

const CommandMaxLength = 1024

// TODO: cleanup
type Client struct {
	State       int
	Helo        string
	MailFrom    string
	RcptTo      string
	Response    string
	Address     string
	Data        string
	Subject     string
	Hash        string
	Time        int64
	TLS         bool
	Conn        net.Conn
	Bufin       *SMTPBufferedReader
	Bufout      *bufio.Writer
	KillTime    int64
	Errors      int
	ClientID    uint64
	SavedNotify chan int
	mu          sync.Mutex
}

func NewClient(conn net.Conn, clientID uint64) *Client {
	return &Client{
		Conn:        conn,
		Address:     conn.RemoteAddr().String(),
		Time:        time.Now().Unix(),
		Bufin:       NewSMTPBufferedReader(conn),
		Bufout:      bufio.NewWriter(conn),
		ClientID:    clientID,
		SavedNotify: make(chan int),
	}
}

func (c *Client) Reset(conn net.Conn, clientID uint64) {
	c.Conn = conn
	// reset our reader & writer
	c.Bufout.Reset(conn)
	c.Bufin.Reset(conn)
	// reset session data
	c.State = 0
	c.KillTime = 0
	c.Time = time.Now().Unix()
	c.ClientID = clientID
	c.TLS = false
	c.Errors = 0
	c.Response = ""
	c.Helo = ""
}

func (c *Client) ClearEmailData() {
	// todo - maybe these could be implemented as buffers? then reset the buffers here
	c.Data = ""
	c.Hash = ""
	c.Address = ""
	c.Subject = ""
	c.RcptTo = ""
	c.MailFrom = ""
}

func (c *Client) SetTimeout(t time.Duration) {
	defer c.mu.Unlock()
	c.mu.Lock()
	c.Conn.SetDeadline(time.Now().Add(t * time.Second))
}

var InputLimitExceeded = errors.New("Line too long") // 500 Line too long.

// we need to adjust the limit, so we embed io.LimitedReader
type adjustableLimitedReader struct {
	R *io.LimitedReader
}

// bolt this on so we can adjust the limit
func (alr *adjustableLimitedReader) setLimit(n int64) {
	alr.R.N = n
}

// this just delegates to the underlying reader in order to satisfy the Reader interface
// Since the vanilla limited reader returns io.EOF when the limit is reached, we need a more specific
// error so that we can distinguish when a limit is reached
func (alr *adjustableLimitedReader) Read(p []byte) (n int, err error) {
	n, err = alr.R.Read(p)
	if err == io.EOF && alr.R.N <= 0 {
		// return our custom error since std lib returns EOF
		err = InputLimitExceeded
	}
	return
}

// allocate a new adjustableLimitedReader
func newAdjustableLimitedReader(r io.Reader, n int64) *adjustableLimitedReader {
	lr := &io.LimitedReader{R: r, N: n}
	return &adjustableLimitedReader{lr}
}

// This is a bufio.Reader what will use our adjustable limit reader
// We 'extend' buffio to have the limited reader feature
type SMTPBufferedReader struct {
	*bufio.Reader
	alr *adjustableLimitedReader
}

// delegate to the adjustable limited reader
func (sbr *SMTPBufferedReader) SetLimit(n int64) {
	sbr.alr.setLimit(n)
}

// Set a new reader & use it to reset the underlying reader
func (sbr *SMTPBufferedReader) Reset(r io.Reader) {
	sbr.alr = newAdjustableLimitedReader(r, CommandMaxLength)
	sbr.Reader.Reset(sbr.alr)
}

// allocate a new smtpBufferedReader
func NewSMTPBufferedReader(rd io.Reader) *SMTPBufferedReader {
	alr := newAdjustableLimitedReader(rd, CommandMaxLength)
	s := &SMTPBufferedReader{bufio.NewReader(alr), alr}
	return s
}
