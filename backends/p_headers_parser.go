package backends

import (
	"github.com/flashmob/go-guerrilla/envelope"
)

// ----------------------------------------------------------------------------------
// Processor Name: headersparser
// ----------------------------------------------------------------------------------
// Description   : Parses the header using e.ParseHeaders()
// ----------------------------------------------------------------------------------
// Config Options: none
// --------------:-------------------------------------------------------------------
// Input         : envelope
// ----------------------------------------------------------------------------------
// Output        : Headers will be populated in e.Header
// ----------------------------------------------------------------------------------
func init() {
	processors["headersparser"] = func() Decorator {
		return HeadersParser()
	}
}

func HeadersParser() Decorator {
	return func(c Processor) Processor {
		return ProcessWith(func(e *envelope.Envelope, task SelectTask) (Result, error) {
			if task == TaskSaveMail {
				e.ParseHeaders()
				// next processor
				return c.Process(e, task)
			} else {
				// next processor
				return c.Process(e, task)
			}
		})
	}
}
