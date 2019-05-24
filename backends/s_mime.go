package backends

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/mime"
	"io"
	"net/textproto"
	"strconv"
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
// Output        :
// ----------------------------------------------------------------------------------

func init() {
	streamers["mimeanalyzer"] = func() *StreamDecorator {
		return StreamMimeAnalyzer()
	}
}

func StreamMimeAnalyzer() *StreamDecorator {

	sd := &StreamDecorator{}
	sd.p =

		func(sp StreamProcessor) StreamProcessor {

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
				_ = parser.Close()
				return nil
			}))

			sd.Open = func(e *mail.Envelope) error {
				envelope = e
				return parser.Open()
			}

			sd.Close = func() error {
				if parts, ok := envelope.Values["MimeParts"].(*[]*mime.MimeHeader); ok {
					for _, v := range *parts {
						fmt.Println(v.part + " " + strconv.Itoa(int(v.startingPos)) + " " + strconv.Itoa(int(v.startingPosBody)) + " " + strconv.Itoa(int(v.endingPosBody)))
					}
				}

				if parseErr == nil {
					err := parser.Close()
					return err
				} else {
					return parseErr
				}
			}

			return StreamProcessWith(func(p []byte) (int, error) {
				_ = envelope
				if len(envelope.Header) > 0 {

				}
				if _, ok := envelope.Values["MimeParts"]; !ok {
					envelope.Values["MimeParts"] = &parser.Parts
				}
				if parseErr == nil {
					_, parseErr = parser.Read(p)
					if parseErr != nil {
						Log().WithError(parseErr).Error("mime parse error")
					}
				}

				return sp.Write(p)
			})
		}

	return sd
}
