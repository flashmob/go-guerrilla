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
	processors["debugger"] = func() Decorator {
		return Debugger()
	}
}

type debuggerConfig struct {
	LogReceivedMails bool `json:"log_received_mails"`
}

func Debugger() Decorator {
	var config *debuggerConfig
	initFunc := Initialize(func(backendConfig BackendConfig) error {
		configType := BaseConfig(&debuggerConfig{})
		bcfg, err := Svc.ExtractConfig(backendConfig, configType)
		if err != nil {
			return err
		}
		config = bcfg.(*debuggerConfig)
		return nil
	})
	Svc.AddInitializer(initFunc)
	return func(c Processor) Processor {
		return ProcessWith(func(e *envelope.Envelope, task SelectTask) (Result, error) {
			if task == TaskSaveMail {
				if config.LogReceivedMails {
					Log().Infof("Mail from: %s / to: %v", e.MailFrom.String(), e.RcptTo)
					Log().Info("Headers are:", e.Header)
				}
				// continue to the next Processor in the decorator stack
				return c.Process(e, task)
			} else {
				return c.Process(e, task)
			}
		})
	}
}
