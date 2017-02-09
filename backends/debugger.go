package backends

import (
	"github.com/flashmob/go-guerrilla/envelope"
)

func Debugger() Decorator {
	return func(c Processor) Processor {
		return ProcessorFunc(func(e *envelope.Envelope) (BackendResult, error) {
			mainlog.Infof("Mail from: %s / to: %v", e.MailFrom.String(), e.RcptTo)
			mainlog.Info("So, Headers are: %s", e.Header)
			return c.Process(e)
		})
	}
}
