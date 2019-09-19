package chunk

import (
	"bytes"
	"io"
)

type transportEncoding int

const (
	encodingTypeBase64 transportEncoding = iota
	encodingTypeQP
)

// decoder decodes base64 and q-printable, then converting charset to UTF-8
type decoder struct {
	buf     []byte
	state   int
	charset string

	r io.Reader
}

// db Storage, email *Email, part int)
/*

r, err := NewChunkedReader(db, email, part)
	if err != nil {
		return nil, err
	}

*/

// NewDecoder reads from an underlying reader r and decodes base64, quoted-printable and decodes
func NewDecoder(r io.Reader, encoding transportEncoding, charset string) (*decoder, error) {
	decoder := new(decoder)
	decoder.r = r
	return decoder, nil
}

const chunkSaverNL = '\n'

const (
	decoderStateFindHeader int = iota
	decoderStateMatchNL
	decoderStateDecode
)

func (r *decoder) Read(p []byte) (n int, err error) {

	r.buf = make([]byte, len(p), cap(p))
	var start, buffered int

	buffered, err = r.r.Read(r.buf)
	if buffered == 0 {
		return
	}
	for {
		switch r.state {
		case decoderStateFindHeader:
			// finding the start of the header
			if start = bytes.Index(r.buf, []byte{chunkSaverNL, chunkSaverNL}); start != -1 {
				start += 2                   // skip the \n\n
				r.state = decoderStateDecode // found the header
				continue                     // continue scanning
			} else if r.buf[len(r.buf)-1] == chunkSaverNL {
				// the last char is a \n so next call to Read will check if it starts with a matching \n
				r.state = decoderStateMatchNL
			}
		case decoderStateMatchNL:
			if r.buf[0] == '\n' {
				// found the header
				start = 1
				r.state = decoderStateDecode
				continue
			} else {
				r.state = decoderStateFindHeader
				continue
			}

		case decoderStateDecode:
			if start < len(r.buf) {
				// todo decode here (q-printable, base64, charset)
				n += copy(p[:], r.buf[start:buffered])
			}
			return
		}

		buffered, err = r.r.Read(r.buf)
		if buffered == 0 {
			return
		}
	}

}
