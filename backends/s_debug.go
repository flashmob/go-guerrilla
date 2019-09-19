package backends

import (
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
	"strings"
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
				str = strings.Replace(str, "\n", "<LF>\n", -1)
				fmt.Println(str)
				Log().WithField("p", string(p)).Info("Debug stream")
				return sp.Write(p)
			})
		}

	return sd
}
