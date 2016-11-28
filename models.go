package guerrilla

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

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

// Backend accepts the received messages, and store/deliver/process them
// type Backend interface {
// 	Initialize(BackendConfig) error
// 	Process(client *Client, from *EmailParts, to []*EmailParts) string
// 	Finalize() error
// }

var InputLimitExceeded = errors.New("Line too long") // 500 Line too long.

// we need to adjust the limit, so we embed io.LimitedReader
type adjustableLimitedReader struct {
	R *io.LimitedReader
}

// bolt this on so we can adjust the limit
func (alr *adjustableLimitedReader) setLimit(n int64) {
	alr.R.N = n
}

// This delegates to the underlying reader in order to satisfy the Reader interface.
// Since the vanilla limited reader returns io.EOF when the limit is reached,
// we need a more specific error so that we can distinguish when a limit is reached.
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
// We 'extend' bufio to have the limited reader feature
type SMTPBufferedReader struct {
	*bufio.Reader
	alr *adjustableLimitedReader
}

// Delegate to the adjustable limited reader
func (sbr *SMTPBufferedReader) SetLimit(n int64) {
	sbr.alr.setLimit(n)
}

// Allocate a new SMTPBufferedReader
func NewSMTPBufferedReader(rd io.Reader) *SMTPBufferedReader {
	alr := newAdjustableLimitedReader(rd, CommandLineMaxLength)
	s := &SMTPBufferedReader{bufio.NewReader(alr), alr}
	return s
}
