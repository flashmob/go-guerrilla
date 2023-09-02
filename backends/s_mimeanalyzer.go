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
		mimeErr  error
		parser   *mimeparse.Parser
	)
	sd.Configure = func(cfg ConfigGroup) error {
		parser = mimeparse.NewMimeParser()
		return nil
	}
	sd.Shutdown = func() error {
		// assumed that parser has been closed, but we can call close again just to make sure
		_ = parser.Close()
		parser = nil
		return nil
	}

	sd.Decorate =
		func(sp StreamProcessor, a ...interface{}) StreamProcessor {

			sd.Open = func(e *mail.Envelope) error {
				parser.Open()
				envelope = e
				mimeErr = nil
				envelope.MimeError = nil
				return nil
			}

			sd.Close = func() error {
				closeErr := parser.Close()
				if mimeErr == nil {
					mimeErr = closeErr
				}

				envelope.MimeError = mimeErr

				if mimeErr != nil {
					Log().WithError(closeErr).Warn("mime parse error in mimeanalyzer on close")
					envelope.MimeError = nil

					if err, ok := mimeErr.(*mimeparse.Error); ok && err.ParseError() {
						// dont propagate parse errors && NotMime error
						return nil
					}
				}
				return mimeErr
			}

			return StreamProcessWith(func(p []byte) (int, error) {
				if envelope.MimeParts == nil {
					envelope.MimeParts = &parser.Parts
				}
				if mimeErr == nil {
					mimeErr = parser.Parse(p)
					if mimeErr != nil {
						Log().WithError(mimeErr).Warn("mime parse error in mimeanalyzer")
					}
				}
				return sp.Write(p)
			})
		}

	return sd

}
