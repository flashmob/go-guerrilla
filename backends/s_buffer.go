package backends

import (
	"bytes"
	"github.com/flashmob/go-guerrilla/mail"
	"io"
)

// ----------------------------------------------------------------------------------
// Processor Name: buffer
// ----------------------------------------------------------------------------------
// Description   : Buffers the message data to envelope.Data
// ----------------------------------------------------------------------------------
// Config Options:
// --------------:-------------------------------------------------------------------
// Input         :
// ----------------------------------------------------------------------------------
// Output        : envelope.Data
// ----------------------------------------------------------------------------------

func init() {
	Streamers["buffer"] = func() *StreamDecorator {
		return StreamProcess()
	}
}

// Buffers to envelope.Data so that processors can be called on it at the end
func StreamProcess() *StreamDecorator {
	sd := &StreamDecorator{}
	sd.Decorate =

		func(sp StreamProcessor, a ...interface{}) StreamProcessor {
			var envelope *mail.Envelope
			sd.Open = func(e *mail.Envelope) error {
				envelope = e
				return nil
			}

			return StreamProcessWith(func(p []byte) (int, error) {
				tr := io.TeeReader(bytes.NewReader(p), sp)
				n, err := envelope.Data.ReadFrom(tr)
				return int(n), err
			})
		}

	return sd
}
