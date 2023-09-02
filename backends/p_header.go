package backends

import (
	"github.com/flashmob/go-guerrilla/mail"
	"strings"
	"time"
)

type headerConfig struct {
	PrimaryHost string `json:"primary_mail_host"`
}

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
	processors["header"] = func() Decorator {
		return Header()
	}
}

// Generate the MTA delivery header
// Sets e.DeliveryHeader part of the envelope with the generated header
func Header() Decorator {

	var config *headerConfig

	Svc.AddInitializer(InitializeWith(func(backendConfig BackendConfig) error {
		configType := BaseConfig(&headerConfig{})
		bcfg, err := Svc.ExtractConfig(ConfigProcessors, "header", backendConfig, configType)
		if err != nil {
			return err
		}
		config = bcfg.(*headerConfig)
		return nil
	}))

	return func(p Processor) Processor {
		return ProcessWith(func(e *mail.Envelope, task SelectTask) (Result, error) {
			if task == TaskSaveMail {
				to := strings.TrimSpace(e.RcptTo[0].User) + "@" + config.PrimaryHost
				hash := "unknown"
				if len(e.Hashes) > 0 {
					hash = e.Hashes[0]
				}
				var addHead string
				addHead += "Delivered-To: " + to + "\n"
				addHead += "Received: from " + e.RemoteIP + " ([" + e.RemoteIP + "])\n"
				if len(e.RcptTo) > 0 {
					addHead += "	by " + e.RcptTo[0].Host + " with " + e.Protocol().String() + " id " + hash + "@" + e.RcptTo[0].Host + ";\n"
				}
				addHead += "	" + time.Now().Format(time.RFC1123Z) + "\n"
				// save the result
				e.DeliveryHeader = addHead
				// next processor
				return p.Process(e, task)

			} else {
				return p.Process(e, task)
			}
		})
	}
}
