package guerrilla

import (
	"bufio"
	"errors"
	"io"
)

var (
	LineLimitExceeded   = errors.New("Maximum line length exceeded")
	MessageSizeExceeded = errors.New("Maximum message size exceeded")
)

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

// Set a new reader & use it to reset the underlying reader
func (sbr *smtpBufferedReader) Reset(r io.Reader) {
	sbr.alr = newAdjustableLimitedReader(r, CommandLineMaxLength)
	sbr.Reader.Reset(sbr.alr)
}

// Allocate a new SMTPBufferedReader
func newSMTPBufferedReader(rd io.Reader) *smtpBufferedReader {
	alr := newAdjustableLimitedReader(rd, CommandLineMaxLength)
	s := &smtpBufferedReader{bufio.NewReader(alr), alr}
	return s
}
