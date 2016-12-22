package guerrilla

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var (
	LineLimitExceeded   = errors.New("Maximum line length exceeded")
	MessageSizeExceeded = errors.New("Maximum message size exceeded")
)

// Backends process received mail. Depending on the implementation, that can
// be storing in a database, retransmitting to another server, etc.
// Must return an SMTP message (i.e. "250 OK") and a boolean indicating
// whether the message was processed successfully.
type Backend interface {
	Process(*Envelope) BackendResult
}

// BackendResult represents a response to an SMTP client after receiving DATA.
// The String method should return an SMTP message ready to send back to the
// client, for example `250 OK: Message received`.
type BackendResult interface {
	fmt.Stringer
	// Code should return the SMTP code associated with this response, ie. `250`
	Code() int
}

// Internal implementation of BackendResult for use by backend implementations.
type backendResult string

func (br backendResult) String() string {
	return string(br)
}

// Parses the SMTP code from the first 3 characters of the SMTP message.
// Returns 554 if code cannot be parsed.
func (br backendResult) Code() int {
	trimmed := strings.TrimSpace(string(br))
	if len(trimmed) < 3 {
		return 554
	}
	code, err := strconv.Atoi(trimmed[:3])
	if err != nil {
		return 554
	}
	return code
}

func NewBackendResult(message string) BackendResult {
	return backendResult(message)
}

// EmailAddress encodes an email address of the form `<user@host>`
type EmailAddress struct {
	User string
	Host string
}

func (ep *EmailAddress) String() string {
	return fmt.Sprintf("%s@%s", ep.User, ep.Host)
}

func (ep *EmailAddress) isEmpty() bool {
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

// allocate a new adjustableLimitedReader
func newAdjustableLimitedReader(r io.Reader, n int64) *adjustableLimitedReader {
	lr := &io.LimitedReader{R: r, N: n}
	return &adjustableLimitedReader{lr}
}

// This is a bufio.Reader what will use our adjustable limit reader
// We 'extend' buffio to have the limited reader feature
type smtpBufferedReader struct {
	*bufio.Reader
	alr *adjustableLimitedReader
}

// Delegate to the adjustable limited reader
func (sbr *smtpBufferedReader) setLimit(n int64) {
	sbr.alr.setLimit(n)
}

// Allocate a new SMTPBufferedReader
func newSMTPBufferedReader(rd io.Reader) *smtpBufferedReader {
	alr := newAdjustableLimitedReader(rd, CommandLineMaxLength)
	s := &smtpBufferedReader{bufio.NewReader(alr), alr}
	return s
}
