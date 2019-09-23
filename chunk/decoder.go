package chunk

import (
	"bytes"
	"encoding/base64"
	"github.com/flashmob/go-guerrilla/mail"
	"io"
	"mime/quotedprintable"
	"strings"

	_ "github.com/flashmob/go-guerrilla/mail/encoding"
)

type transportEncoding int

const (
	transportBase64 transportEncoding = iota
	transportQuotedPrintable
	transport7bit // default, 1-127, 13 & 10 at line endings
	transport8bit // 998 octets per line,  13 & 10 at line endings

)

// decoder decodes base64 and q-printable, then converting charset to UTF-8
type decoder struct {
	buf       []byte
	state     int
	charset   string
	transport transportEncoding
	r         io.Reader
}

// NewDecoder reads from an underlying reader r and decodes base64, quoted-printable and decodes
func NewDecoder(r io.Reader, transport transportEncoding, charset string) (*decoder, error) {
	decoder := new(decoder)
	decoder.transport = transport
	decoder.charset = strings.ToLower(charset)
	decoder.r = r
	return decoder, nil
}

const chunkSaverNL = '\n'

const (
	decoderStateFindHeader int = iota
	decoderStateMatchNL
	decoderStateDecodeSetup
	decoderStateDecode
)

func (r *decoder) Read(p []byte) (n int, err error) {
	r.buf = make([]byte, len(p), cap(p))
	var start, buffered int
	if r.state != decoderStateDecode {
		buffered, err = r.r.Read(r.buf)
		if buffered == 0 {
			return
		}
	}
	for {
		switch r.state {
		case decoderStateFindHeader:
			// finding the start of the header
			if start = bytes.Index(r.buf, []byte{chunkSaverNL, chunkSaverNL}); start != -1 {
				start += 2                        // skip the \n\n
				r.state = decoderStateDecodeSetup // found the header
				continue
			} else if r.buf[len(r.buf)-1] == chunkSaverNL {
				// the last char is a \n so next call to Read will check if it starts with a matching \n
				r.state = decoderStateMatchNL
			}
		case decoderStateMatchNL:
			// check the first char if it is a '\n' because last time we matched a '\n'
			if r.buf[0] == '\n' {
				// found the header
				start = 1
				r.state = decoderStateDecodeSetup
				continue
			} else {
				r.state = decoderStateFindHeader
				continue
			}
		case decoderStateDecodeSetup:
			if start != buffered {
				// include any bytes that have already been read
				r.r = io.MultiReader(bytes.NewBuffer(r.buf[start:buffered]), r.r)
			}
			switch r.transport {
			case transportQuotedPrintable:
				r.r = quotedprintable.NewReader(r.r)
			case transportBase64:
				r.r = base64.NewDecoder(base64.StdEncoding, r.r)
			default:

			}
			// conversion to utf-8
			if r.charset != "utf-8" {
				r.r, err = mail.Dec.CharsetReader(r.charset, r.r)
				if err != nil {
					return
				}
			}
			r.state = decoderStateDecode
			continue
		case decoderStateDecode:
			return r.r.Read(p)
		}
		start = 0
		buffered, err = r.r.Read(r.buf)
		if buffered == 0 {
			return
		}
	}
}
