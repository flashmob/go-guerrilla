package backends

import (
	"compress/zlib"
	"github.com/flashmob/go-guerrilla/mail"
	"io"
)

func init() {
	streamers["compress"] = func() *StreamDecorator {
		return StreamCompress()
	}
}

func StreamCompress() *StreamDecorator {
	sd := &StreamDecorator{}
	sd.p =
		func(sp StreamProcessor) StreamProcessor {
			var zw io.WriteCloser
			_ = zw
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
