package mail

import (
	"bufio"
	"io"
	"net/textproto"

	"github.com/flashmob/go-guerrilla/mail/mime"
)

// MimeDotReader parses the mime structure while reading using the underlying reader
type MimeDotReader struct {
	R       io.Reader
	p       *mime.Parser
	mimeErr error
}

// Read parses the mime structure wile reading. Results are immediately available in
// the data-structure returned from Parts() after each read.
func (r *MimeDotReader) Read(p []byte) (n int, err error) {
	n, err = r.R.Read(p)
	if n > 0 {
		if r.mimeErr == nil {
			r.mimeErr = r.p.Parse(p)
		}
	}
	if err != nil {
		if r.mimeErr == nil {
			r.mimeErr = r.p.Close()
		}
		return
	}
	return
}

// Close closes the underlying reader if it's a ReadCloser and closes the mime parser
func (r MimeDotReader) Close() (err error) {
	if rc, t := r.R.(io.ReadCloser); t {
		err = rc.Close()
	}
	// parser already closed?
	if r.mimeErr != nil {
		return r.mimeErr
	}
	// close the parser, only care about parse errors
	if pErr := r.p.Close(); r.p.ParseError(pErr) {
		err = pErr
	}
	return
}

// Parts returns the mime-header parts built by the parser
func (r *MimeDotReader) Parts() mime.Parts {
	return r.p.Parts
}

// Returns the underlying io.Reader (which is a dotReader from textproto)
// useful for reading from directly if mime parsing is not desirable.
func (r *MimeDotReader) DotReader() io.Reader {
	return r.R
}

// NewMimeDotReader returns a pointer to a new MimeDotReader
// br is the underlying reader it will read from
// maxNodes limits the number of nodes can be added to the mime tree before the mime-parser aborts
func NewMimeDotReader(br *bufio.Reader, maxNodes int) *MimeDotReader {
	r := new(MimeDotReader)
	r.R = textproto.NewReader(br).DotReader()
	if maxNodes > 0 {
		r.p = mime.NewMimeParserLimited(maxNodes)
	} else {
		r.p = mime.NewMimeParser()
	}
	r.p.Open()
	return r
}
