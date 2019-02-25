package backends

import (
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
	"strings"
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
				str := string(p)
				str = strings.Replace(str, "\n", "<LF>\n", -1)
				fmt.Println(str)
				Log().WithField("p", string(p)).Info("Debug stream")
				return sp.Write(p)
			})
		}

	return sd
}
