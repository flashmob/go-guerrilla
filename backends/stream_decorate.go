package backends

import "github.com/flashmob/go-guerrilla/mail"

type streamOpenWith func(e *mail.Envelope) error

type streamCloseWith func() error

// We define what a decorator to our processor will look like
type StreamDecorator struct {
	p     func(StreamProcessor) StreamProcessor
	e     *mail.Envelope
	Close streamCloseWith
	Open  streamOpenWith
}

// DecorateStream will decorate a StreamProcessor with a slice of passed decorators
func DecorateStream(c StreamProcessor, ds ...StreamDecorator) StreamProcessor {
	decorated := c
	for _, decorate := range ds {
		decorated = decorate.p(decorated)
	}
	return decorated
}

func (sd *StreamDecorator) OpenX(e *mail.Envelope) error {
	sd.e = e
	return nil
}

func (sd *StreamDecorator) Closex() error {
	return nil
}
