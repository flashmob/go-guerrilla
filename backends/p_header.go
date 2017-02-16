package backends

import (
	"github.com/flashmob/go-guerrilla/envelope"
	"strings"
	"time"
)

type HeaderConfig struct {
	PrimaryHost string `json:"primary_mail_host"`
}

// ----------------------------------------------------------------------------------
// Processor Name: header
// ----------------------------------------------------------------------------------
// Description   : Adds delivery information headers to e.DeliveryHeader
// ----------------------------------------------------------------------------------
// Config Options: none
// --------------:-------------------------------------------------------------------
// Input         : e.Helo
//               : e.RemoteAddress
//               : e.RcptTo
//               : e.Hashes
// ----------------------------------------------------------------------------------
// Output        : Sets e.DeliveryHeader with additional delivery info
// ----------------------------------------------------------------------------------
func init() {
	Processors["header"] = func() Decorator {
		return Header()
	}
}

// Generate the MTA delivery header
// Sets e.DeliveryHeader part of the envelope with the generated header
func Header() Decorator {

	var config *HeaderConfig

	Service.AddInitializer(Initialize(func(backendConfig BackendConfig) error {
		configType := BaseConfig(&HeaderConfig{})
		bcfg, err := Service.ExtractConfig(backendConfig, configType)
		if err != nil {
			return err
		}
		config = bcfg.(*HeaderConfig)
		return nil
	}))

	return func(c Processor) Processor {
		return ProcessorFunc(func(e *envelope.Envelope) (BackendResult, error) {
			to := strings.TrimSpace(e.RcptTo[0].User) + "@" + config.PrimaryHost
			hash := "unknown"
			if len(e.Hashes) > 0 {
				hash = e.Hashes[0]
			}
			var addHead string
			addHead += "Delivered-To: " + to + "\n"
			addHead += "Received: from " + e.Helo + " (" + e.Helo + "  [" + e.RemoteAddress + "])\n"
			if len(e.RcptTo) > 0 {
				addHead += "	by " + e.RcptTo[0].Host + " with SMTP id " + hash + "@" + e.RcptTo[0].Host + ";\n"
			}
			addHead += "	" + time.Now().Format(time.RFC1123Z) + "\n"
			// save the result
			e.DeliveryHeader = addHead
			// next processor
			return c.Process(e)
		})
	}
}
