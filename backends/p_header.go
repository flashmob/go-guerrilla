package backends

import (
	"github.com/flashmob/go-guerrilla/envelope"
	"strings"
	"time"
)

type HeaderConfig struct {
	PrimaryHost string `json:"primary_mail_host"`
}

// Generate the MTA delivery header
// Sets e.DeliveryHeader with the result
func Header(dc *DecoratorCallbacks) Decorator {

	var config *HeaderConfig
	dc.loader = func(backendConfig BackendConfig) error {
		configType := baseConfig(&HeaderConfig{})
		bcfg, err := ab.extractConfig(backendConfig, configType)
		if err != nil {
			return err
		}
		config = bcfg.(*HeaderConfig)

		return nil
	}

	return func(c Processor) Processor {
		return ProcessorFunc(func(e *envelope.Envelope) (BackendResult, error) {
			to := strings.TrimSpace(e.RcptTo[0].User) + "@" + config.PrimaryHost
			hash := "unknown"
			if len(e.Hashes) > 0 {
				hash = e.Hashes[0]
			}
			var addHead string
			addHead += "Delivered-To: " + to + "\r\n"
			addHead += "Received: from " + e.Helo + " (" + e.Helo + "  [" + e.RemoteAddress + "])\r\n"
			if len(e.RcptTo) > 0 {
				addHead += "	by " + e.RcptTo[0].Host + " with SMTP id " + hash + "@" + e.RcptTo[0].Host + ";\r\n"
			}
			addHead += "	" + time.Now().Format(time.RFC1123Z) + "\r\n"
			// save the result
			e.DeliveryHeader = addHead
			// next processor
			return c.Process(e)
		})
	}
}
