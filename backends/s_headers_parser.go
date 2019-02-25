package backends

import (
	"bufio"
	"bytes"
	"io"
	"net/textproto"

	"github.com/flashmob/go-guerrilla/mail"
)

func init() {
	streamers["headersparser"] = func() *StreamDecorator {
		return StreamHeadersParser()
	}
}

const stateHeaderScanning = 0
const stateHeaderNotScanning = 1
const headerMaxBytes = 1024 * 4

func StreamHeadersParser() *StreamDecorator {
	sd := &StreamDecorator{}
	sd.p =

		func(sp StreamProcessor) StreamProcessor {

			// buf buffers the header
			var buf bytes.Buffer
			var state byte
			var lastByte byte
			var total int64
			var envelope *mail.Envelope

			parse := func() error {
				var err error
				// use a TeeReader to split the write to both sp and headerReader
				r := bufio.NewReader(io.TeeReader(&buf, sp))
				headerReader := textproto.NewReader(r)
				envelope.Header, err = headerReader.ReadMIMEHeader()

				if err != nil {
					if subject, ok := envelope.Header["Subject"]; ok {
						envelope.Subject = mail.MimeHeaderDecode(subject[0])
					}
				}

				return err
			}
			sd.Open = func(e *mail.Envelope) error {
				buf.Reset()
				state = 0
				lastByte = 0
				total = 0
				envelope = e
				return nil
			}

			sd.Close = func() error {
				// If header wasn't detected
				// pump out whatever is in the buffer to the underlying writer
				if state == stateHeaderScanning {
					_, err := io.Copy(sp, &buf)
					return err
				}
				return nil
			}
			return StreamProcessWith(func(p []byte) (int, error) {

				switch state {
				case stateHeaderScanning:
					// detect end of header \n\n
					headerEnd := bytes.Index(p, []byte{'\n', '\n'})
					if headerEnd == -1 && (lastByte == '\n' && p[0] == '\n') {
						headerEnd = 0
					}
					var remainder []byte // remainder are the non-header bytes after the \n\n
					if headerEnd > -1 {

						if len(p) > headerEnd {
							remainder = p[headerEnd:]
						}
						p = p[:headerEnd]
					}

					// read in the header to a temp buffer
					n, err := io.Copy(&buf, bytes.NewReader(p))
					lastByte = p[n-1] // remember the last byte read
					if headerEnd > -1 {
						// header found, parse it
						parseErr := parse()
						if parseErr != nil {
							Log().WithError(parseErr).Error("cannot parse headers")
						}
						// flush the remainder to the underlying writer
						if remainder != nil {
							n1, _ := sp.Write(remainder)
							n = n + int64(n1)
						}
						state = stateHeaderNotScanning
					} else {
						total += n
						// give up if we didn't detect the header after x bytes
						if total > headerMaxBytes {
							state = stateHeaderNotScanning
							n, err = io.Copy(sp, &buf)
							return int(n), err
						}
					}
					return int(n), err
				case stateHeaderNotScanning:
					// just forward everything to the next writer without buffering
					return sp.Write(p)
				}

				return sp.Write(p)
			})
		}

	return sd
}
