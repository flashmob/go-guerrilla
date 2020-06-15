package backends

import (
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
)

func init() {
	Streamers["debug"] = func() *StreamDecorator {
		return StreamDebug()
	}
}

func StreamDebug() *StreamDecorator {
	sd := &StreamDecorator{}
	sd.Decorate =

		func(sp StreamProcessor, a ...interface{}) StreamProcessor {

			sd.Open = func(e *mail.Envelope) error {
				return nil
			}
			return StreamProcessWith(func(p []byte) (int, error) {
				str := string(p)
				fmt.Print(str)
				Log().WithField("p", string(p)).Info("Debug stream")
				return sp.Write(p)
			})
		}

	return sd
}
