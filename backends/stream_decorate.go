package backends

import (
	"github.com/flashmob/go-guerrilla/mail"
)

type streamOpenWith func(e *mail.Envelope) error

type streamCloseWith func() error

// We define what a decorator to our processor will look like
// StreamProcessor argument is the underlying processor that we're decorating
// the additional ...interface argument is not needed, but can be useful for dependency injection
type StreamDecorator struct {
	Decorate func(StreamProcessor, ...interface{}) StreamProcessor
	e        *mail.Envelope
	Close    streamCloseWith
	Open     streamOpenWith
}

// DecorateStream will decorate a StreamProcessor with a slice of passed decorators
func DecorateStream(c StreamProcessor, ds []*StreamDecorator) (StreamProcessor, []*StreamDecorator) {
	for i := range ds {
		c = ds[i].Decorate(c)
	}
	return c, ds
}
