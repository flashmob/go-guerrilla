package backends

import (
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/mimeparse"
)

// ----------------------------------------------------------------------------------
// Name          : Mime Analyzer
// ----------------------------------------------------------------------------------
// Description   : Analyse the MIME structure of a stream.
//               : Headers of each part are unfolded and saved in a *mime.Parts struct.
//               : No decoding or any other processing.
// ----------------------------------------------------------------------------------
// Config Options:
// --------------:-------------------------------------------------------------------
// Input         :
// ----------------------------------------------------------------------------------
// Output        : MimeParts (of type *mime.Parts) stored in the envelope.MimeParts field
// ----------------------------------------------------------------------------------

func init() {
	Streamers["mimeanalyzer"] = func() *StreamDecorator {
		return StreamMimeAnalyzer()
	}
}

func StreamMimeAnalyzer() *StreamDecorator {

	sd := &StreamDecorator{}
	var (
		envelope *mail.Envelope
		parseErr error
		parser   *mimeparse.Parser
	)
	sd.Configure = func(cfg ConfigGroup) error {
		parser = mimeparse.NewMimeParser()
		return nil
	}
	sd.Shutdown = func() error {
		var err error
		defer func() {
			parser = nil

		}()
		if err = parser.Close(); err != nil {
			Log().WithError(err).Error("error when closing parser in mimeanalyzer")
			return err
		}
		return nil
	}

	sd.Decorate =
		func(sp StreamProcessor, a ...interface{}) StreamProcessor {

			sd.Open = func(e *mail.Envelope) error {
				parser.Open()
				envelope = e
				return nil
			}

			sd.Close = func() error {
				if parseErr == nil {
					_ = parser.Close()
					return nil
				} else {
					return parseErr
				}
			}

			return StreamProcessWith(func(p []byte) (int, error) {
				if envelope.MimeParts == nil {
					envelope.MimeParts = &parser.Parts
				}
				if parseErr == nil {
					parseErr = parser.Parse(p)
					if parseErr != nil {
						Log().WithError(parseErr).Error("mime parse error in mimeanalyzer")
					}
				}
				return sp.Write(p)
			})
		}

	return sd

}
