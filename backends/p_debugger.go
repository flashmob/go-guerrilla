package backends

import (
	"github.com/flashmob/go-guerrilla/envelope"
)

// ----------------------------------------------------------------------------------
// Processor Name: debugger
// ----------------------------------------------------------------------------------
// Description   : Log received emails
// ----------------------------------------------------------------------------------
// Config Options: log_received_mails bool - log if true
// --------------:-------------------------------------------------------------------
// Input         : e.MailFrom, e.RcptTo, e.Header
// ----------------------------------------------------------------------------------
// Output        : none (only output to the log if enabled)
// ----------------------------------------------------------------------------------
func init() {
	Processors["debugger"] = func() Decorator {
		return Debugger()
	}
}

type debuggerConfig struct {
	LogReceivedMails bool `json:"log_received_mails"`
}

func Debugger() Decorator {
	var config *debuggerConfig
	initFunc := Initialize(func(backendConfig BackendConfig) error {
		configType := baseConfig(&debuggerConfig{})
		bcfg, err := Service.extractConfig(backendConfig, configType)
		if err != nil {
			return err
		}
		config = bcfg.(*debuggerConfig)
		return nil
	})
	Service.AddInitializer(initFunc)
	return func(c Processor) Processor {
		return ProcessorFunc(func(e *envelope.Envelope) (BackendResult, error) {
			if config.LogReceivedMails {
				Log().Infof("Mail from: %s / to: %v", e.MailFrom.String(), e.RcptTo)
				Log().Info("Headers are:", e.Header)
			}
			// continue to the next Processor in the decorator chain
			return c.Process(e)
		})
	}
}
