package backends

import (
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
)

func init() {
	streamers["debug"] = func() *StreamDecorator {
		return StreamDebug()
	}
}

func StreamDebug() *StreamDecorator {
	sd := &StreamDecorator{}
	sd.p =

		func(sp StreamProcessor) StreamProcessor {

			sd.Open = func(e *mail.Envelope) error {
				return nil
			}
			return StreamProcessWith(func(p []byte) (int, error) {
				fmt.Println(string(p))
				Log().WithField("p", string(p)).Info("Debug stream")
				return sp.Write(p)
			})
		}

	return sd
}
