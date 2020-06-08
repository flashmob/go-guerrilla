package backends

import (
	"github.com/flashmob/go-guerrilla/mail"
	"io"
	"strings"
	"time"
)

// ----------------------------------------------------------------------------------
// Processor Name: header
// ----------------------------------------------------------------------------------
// Description   : Adds delivery information headers to e.DeliveryHeader
// ----------------------------------------------------------------------------------
// Config Options: primary_mail_host - string of the primary mail hostname
// --------------:-------------------------------------------------------------------
// Input         : e.Helo
//               : e.RemoteAddress
//               : e.RcptTo
//               : e.Hashes
// ----------------------------------------------------------------------------------
// Output        : Sets e.DeliveryHeader with additional delivery info
// ----------------------------------------------------------------------------------

func init() {
	Streamers["header"] = func() *StreamDecorator {
		return StreamHeader()
	}
}

type streamHeader struct {
	addHead []byte
	w       io.Writer
	i       int
}

func newStreamHeader(w io.Writer) *streamHeader {
	sc := new(streamHeader)
	sc.w = w
	return sc
}

func (sh *streamHeader) addHeader(e *mail.Envelope, config *HeaderConfig) {
	to := strings.TrimSpace(e.RcptTo[0].User) + "@" + config.PrimaryHost
	hash := "unknown"
	if len(e.Hashes) > 0 {
		hash = e.Hashes[0]
	}
	var addHead string
	addHead += "Delivered-To: " + to + "\n"
	addHead += "Received: from " + e.Helo + " (" + e.Helo + "  [" + e.RemoteIP + "])\n"
	if len(e.RcptTo) > 0 {
		addHead += "	by " + e.RcptTo[0].Host + " with SMTP id " + hash + "@" + e.RcptTo[0].Host + ";\n"
	}
	addHead += "	" + time.Now().Format(time.RFC1123Z) + "\n"

	sh.addHead = []byte(addHead)
}

func StreamHeader() *StreamDecorator {

	var hc *HeaderConfig

	Svc.AddInitializer(InitializeWith(func(backendConfig BackendConfig) error {
		configType := BaseConfig(&HeaderConfig{})
		bcfg, err := Svc.ExtractConfig(
			ConfigStreamProcessors, "header", backendConfig, configType)
		if err != nil {
			return err
		}
		hc = bcfg.(*HeaderConfig)
		return nil
	}))

	sd := &StreamDecorator{}
	sd.Decorate =

		func(sp StreamProcessor, a ...interface{}) StreamProcessor {
			var sh *streamHeader

			sd.Open = func(e *mail.Envelope) error {
				sh = newStreamHeader(sp)
				sh.addHeader(e, hc)
				return nil
			}

			return StreamProcessWith(func(p []byte) (int, error) {
				if sh.i < len(sh.addHead) {
					for {
						if N, err := sh.w.Write(sh.addHead[sh.i:]); err != nil {
							return N, err
						} else {
							sh.i += N
							if sh.i >= len(sh.addHead) {
								break
							}
						}
					}
				}
				return sp.Write(p)
			})
		}

	return sd
}
