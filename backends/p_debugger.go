package backends

import (
	"github.com/flashmob/go-guerrilla/mail"
	"strings"
	"time"
)

// ----------------------------------------------------------------------------------
// Processor Name: debugger
// ----------------------------------------------------------------------------------
// Description   : Log received emails
// ----------------------------------------------------------------------------------
// Config Options: log_received_mails bool - log if true
//               : sleep_seconds - how many seconds to pause for, useful to force a
//               : timeout. If sleep_seconds is 1 then a panic will be induced
// --------------:-------------------------------------------------------------------
// Input         : e.MailFrom, e.RcptTo, e.Header
// ----------------------------------------------------------------------------------
// Output        : none (only output to the log if enabled)
// ----------------------------------------------------------------------------------
func init() {
	processors[strings.ToLower(defaultProcessor)] = func() Decorator {
		return Debugger()
	}
}

type debuggerConfig struct {
	LogReceivedMails bool `json:"log_received_mails"`
	SleepSec         int  `json:"sleep_seconds,omitempty"`
}

func Debugger() Decorator {
	var config *debuggerConfig
	initFunc := InitializeWith(func(backendConfig BackendConfig) error {
		configType := BaseConfig(&debuggerConfig{})
		bcfg, err := Svc.ExtractConfig(
			ConfigProcessors, defaultProcessor, backendConfig, configType)
		if err != nil {
			return err
		}
		config = bcfg.(*debuggerConfig)
		return nil
	})
	Svc.AddInitializer(initFunc)
	return func(p Processor) Processor {
		return ProcessWith(func(e *mail.Envelope, task SelectTask) (Result, error) {
			if task == TaskSaveMail {
				if config.LogReceivedMails {
					Log().Fields("queuedID", e.QueuedId, "from", e.MailFrom.String(), "to", e.RcptTo).Info("save mail")
					Log().Fields("queuedID", e.QueuedId, "headers", e.Header).Info("headers dump")
					Log().Fields("queuedID", e.QueuedId, "body", e.Data.String()).Info("body dump")
				}
				if config.SleepSec > 0 {
					Log().Fields("queuedID", e.QueuedId, "sleep", config.SleepSec).Info("sleeping")
					time.Sleep(time.Second * time.Duration(config.SleepSec))
					Log().Fields("queuedID", e.QueuedId).Info("woke up")

					if config.SleepSec == 1 {
						panic("panic on purpose")
					}
				}
				// continue to the next Processor in the decorator stack
				return p.Process(e, task)
			} else {
				return p.Process(e, task)
			}
		})
	}
}
