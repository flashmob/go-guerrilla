package backends

import (
	"bytes"
	"compress/zlib"
	"github.com/flashmob/go-guerrilla/mail"
	"io"
)

func init() {
	streamers["decompress"] = func() *StreamDecorator {
		return StreamDecompress()
	}
}

// StreamDecompress is a PoC demonstrating how we can connect an io.Reader to our Writer
// We use an io.Pipe to connect the two, writing to one end of the pipe, while
// consuming the output on the other end of the pipe.

func StreamDecompress() *StreamDecorator {
	sd := &StreamDecorator{}
	sd.Decorate =
		func(sp StreamProcessor, a ...interface{}) StreamProcessor {
			var (
				zr io.ReadCloser
				pr *io.PipeReader
				pw *io.PipeWriter
			)

			// consumer runs as a gorouitne.
			// It connects the zlib reader with the read-end of the pipe
			// then copies the output down to the next stream processor
			// consumer will exit of the pipe gets closed or on error
			consumer := func() {
				var err error
				for {
					if zr == nil {
						zr, err = zlib.NewReader(pr)
						if err != nil {
							_ = pr.CloseWithError(err)
							return
						}
					}

					_, err := io.Copy(sp, zr)
					if err != nil {
						_ = pr.CloseWithError(err)
						return
					}
				}
			}

			// start our consumer goroutine
			sd.Open = func(e *mail.Envelope) error {
				pr, pw = io.Pipe()
				go consumer()
				return nil
			}

			// close both ends of the pipes when finished
			sd.Close = func() error {
				errR := pr.Close()
				errW := pw.Close()
				if err := zr.Close(); err != nil {
					return err
				}
				if errR != nil {
					return errR
				}
				if errW != nil {
					return errW
				}
				return nil
			}

			return StreamProcessWith(func(p []byte) (n int, err error) {
				// take the output and copy on the pipe, for the consumer to pick up
				N, err := io.Copy(pw, bytes.NewReader(p))
				if N > 0 {
					n = int(N)
				}
				return
			})

		}
	return sd
}
