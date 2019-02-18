package backends

import (
	"compress/zlib"
	"github.com/flashmob/go-guerrilla/mail"
	"io"
)

func init() {
	streamers["compress"] = func() StreamDecorator {
		return StreamCompress()
	}
}

func StreamCompress() StreamDecorator {
	sd := StreamDecorator{}
	sd.p =
		func(sp StreamProcessor) StreamProcessor {
			var zw io.WriteCloser
			sd.Open = func(e *mail.Envelope) error {
				var err error
				zw, err = zlib.NewWriterLevel(sp, zlib.BestSpeed)
				return err
			}

			sd.Close = func() error {
				return zw.Close()
			}

			return StreamProcessWith(zw.Write)
			/*
				return StreamProcessWith(func(p []byte) (n int, err error) {
					var buf bytes.Buffer
					if n, err := io.Copy(w, bytes.NewReader(p)); err != nil {
						return int(n), err
					}
					return sp.Write(buf.Bytes())
				})
			*/

		}
	return sd
}
