package backends

import (
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/mime"
)

// ----------------------------------------------------------------------------------
// Name          : Mime Analyzer
// ----------------------------------------------------------------------------------
// Description   : analyse the MIME structure of a stream
// ----------------------------------------------------------------------------------
// Config Options:
// --------------:-------------------------------------------------------------------
// Input         :
// ----------------------------------------------------------------------------------
// Output        : MimeParts (of type *[]*mime.Part) stored in the envelope.Values map
// ----------------------------------------------------------------------------------

func init() {
	Streamers["mimeanalyzer"] = func() *StreamDecorator {
		return StreamMimeAnalyzer()
	}
}

func StreamMimeAnalyzer() *StreamDecorator {

	sd := &StreamDecorator{}
	sd.Decorate =

		func(sp StreamProcessor, a ...interface{}) StreamProcessor {

			var (
				envelope *mail.Envelope
				parseErr error
				parser   *mime.Parser
			)
			Svc.AddInitializer(InitializeWith(func(backendConfig BackendConfig) error {
				parser = mime.NewMimeParser()
				return nil
			}))

			Svc.AddShutdowner(ShutdownWith(func() error {
				fmt.Println("shutdownewr")
				//_ = parser.Close()
				return nil
			}))

			sd.Open = func(e *mail.Envelope) error {
				envelope = e
				return nil
			}

			sd.Close = func() error {
				/*
					defer func() {
						// todo remove, for debugging only
						if parts, ok := envelope.Values["MimeParts"].(*[]*mime.Part); ok {
							for _, v := range *parts {
								fmt.Println(v.Node + " " + strconv.Itoa(int(v.StartingPos)) + " " + strconv.Itoa(int(v.StartingPosBody)) + " " + strconv.Itoa(int(v.EndingPosBody)))
							}
						}
					}()

				*/

				if parseErr == nil {
					_ = parser.Close()
					return nil
				} else {
					return parseErr
				}
			}

			return StreamProcessWith(func(p []byte) (int, error) {
				_ = envelope

				if _, ok := envelope.Values["MimeParts"]; !ok {
					envelope.Values["MimeParts"] = &parser.Parts
				}

				if parseErr == nil {
					parseErr = parser.Parse(p)
					if parseErr != nil {
						Log().WithError(parseErr).Error("mime parse error")
					}
				}

				return sp.Write(p)
			})
		}

	return sd
}
