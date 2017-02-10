package backends

import (
	"crypto/md5"
	"fmt"
	"github.com/flashmob/go-guerrilla/envelope"
	"io"
	"strings"
	"time"
)

// The hasher decorator computes a hash of the email for each recipient
// It appends the hashes to envelope's Hashes slice.
func Hasher() Decorator {
	return func(c Processor) Processor {
		return ProcessorFunc(func(e *envelope.Envelope) (BackendResult, error) {

			// base hash
			h := md5.New()
			ts := fmt.Sprintf("%d", time.Now().UnixNano())
			io.Copy(h, strings.NewReader(e.MailFrom.String()))
			io.Copy(h, strings.NewReader(e.Subject))
			io.Copy(h, strings.NewReader(ts))

			// using the base hash, calculate a unique hash for each recipient
			for i := range e.RcptTo {
				h2 := h // copy
				io.Copy(h2, strings.NewReader(e.RcptTo[i].String()))
				sum := h2.Sum([]byte{})
				e.Hashes = append(e.Hashes, fmt.Sprintf("%x", sum))
			}

			return c.Process(e)
		})
	}
}
