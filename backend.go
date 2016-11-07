package guerrilla

import (
	"bufio"
	"errors"
	"io"
	"net"
)

// Backend accepts the recieved messages, and store/deliver/process them
type Backend interface {
	Initialize(BackendConfig) error
	Process(client *Client, user, host string) string
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
	ClientID    int64
	SavedNotify chan int
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

// allocate a new smtpBufferedReader
func NewSMTPBufferedReader(rd io.Reader) *SMTPBufferedReader {
	alr := newAdjustableLimitedReader(rd, CommandMaxLength)
	s := &SMTPBufferedReader{bufio.NewReader(alr), alr}
	return s
}
