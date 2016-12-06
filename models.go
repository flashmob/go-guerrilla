package guerrilla

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

var (
	LineLimitExceeded   = errors.New("Maximum line length exceeded")
	MessageSizeExceeded = errors.New("Maximum message size exceeded")
)

// Backends process received mail. Depending on the implementation, that can
// be storing in a database, retransmitting to another server, etc.
// Must return an SMTP message to send back to the client.
type Backend interface {
	Process(*Client) string
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
		err = LineLimitExceeded
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
