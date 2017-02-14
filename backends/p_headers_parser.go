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
	Processors["headersparser"] = func() Decorator {
		return HeadersParser()
	}
}

func HeadersParser() Decorator {
	return func(c Processor) Processor {
		return ProcessorFunc(func(e *envelope.Envelope) (BackendResult, error) {
			e.ParseHeaders()
			return c.Process(e)
		})
	}
}
