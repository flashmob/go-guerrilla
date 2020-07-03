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

type streamCompressConfig struct {
	CompressLevel int `json:"compress_level,omitempty"`
}

func StreamCompress() *StreamDecorator {
	sd := &StreamDecorator{}
	var config streamCompressConfig
	sd.Configure = func(cfg ConfigGroup) error {
		if _, ok := cfg["compress_level"]; !ok {
			cfg["compress_level"] = zlib.BestSpeed
		}
		return sd.ExtractConfig(cfg, &config)
	}
	sd.Decorate =
		func(sp StreamProcessor, a ...interface{}) StreamProcessor {
			var zw io.WriteCloser
			sd.Open = func(e *mail.Envelope) error {
				var err error
				zw, err = zlib.NewWriterLevel(sp, config.CompressLevel)
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
