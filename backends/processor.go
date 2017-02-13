package backends

import (
	"github.com/flashmob/go-guerrilla/envelope"
)

// Our processor is defined as something that processes the envelope and returns a result and error
type Processor interface {
	Process(*envelope.Envelope) (BackendResult, error)
}

// Signature of DoFunc
type ProcessorFunc func(*envelope.Envelope) (BackendResult, error)

// Make ProcessorFunc will satisfy the Processor interface
func (f ProcessorFunc) Process(e *envelope.Envelope) (BackendResult, error) {
	return f(e)
}

// DefaultProcessor is a undecorated worker that does nothing
// Notice MockClient has no knowledge of the other decorators that have orthogonal concerns.
type DefaultProcessor struct{}

// do nothing except return the result
func (w DefaultProcessor) Process(e *envelope.Envelope) (BackendResult, error) {
	return NewBackendResult("200 OK"), nil
}
