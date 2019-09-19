package backends

import (
	"compress/zlib"
	"io"

	"github.com/flashmob/go-guerrilla/mail"
)

func init() {
	Streamers["compress"] = func() *StreamDecorator {
		return StreamCompress()
	}
}

func StreamCompress() *StreamDecorator {
	sd := &StreamDecorator{}
	sd.Decorate =
		func(sp StreamProcessor, a ...interface{}) StreamProcessor {
			var zw io.WriteCloser
			sd.Open = func(e *mail.Envelope) error {
				var err error
				zw, err = zlib.NewWriterLevel(sp, zlib.BestSpeed)
				return err
			}

			sd.Close = func() error {
				return zw.Close()
			}

			return StreamProcessWith(func(p []byte) (int, error) {
				return zw.Write(p)
			})

		}
	return sd
}
