package backends

import (
	"encoding/json"
	"github.com/flashmob/go-guerrilla/mail"
)

type streamOpenWith func(e *mail.Envelope) error
type streamCloseWith func() error
type streamConfigureWith func(cfg ConfigGroup) error
type streamShutdownWith func() error

// We define what a decorator to our processor will look like
// StreamProcessor argument is the underlying processor that we're decorating
// the additional ...interface argument is not needed, but can be useful for dependency injection
type StreamDecorator struct {
	// Decorate is called first. The StreamProcessor will be the next processor called
	// after this one finished.
	Decorate func(StreamProcessor, ...interface{}) StreamProcessor
	e        *mail.Envelope
	// Open is called at the start of each email
	Open streamOpenWith
	// Close is called when the email finished writing
	Close streamCloseWith
	// Configure is always called after Decorate, only once for the entire lifetime
	// it can open database connections, test file permissions, etc
	Configure streamConfigureWith
	// Shutdown is called to release any resources before StreamDecorator is destroyed
	// typically to close any database connections, cleanup any files, etc
	Shutdown streamShutdownWith
	// GetEmail returns a reader for reading the data of ab email,
	// it may return nil if no email is available
	GetEmail func(emailID uint64) (SeekPartReader, error)
}

func (s StreamDecorator) ExtractConfig(cfg ConfigGroup, i interface{}) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, i)
	if err != nil {
		return err
	}
	return nil
}

// DecorateStream will decorate a StreamProcessor with a slice of passed decorators
func DecorateStream(c StreamProcessor, ds []*StreamDecorator) (StreamProcessor, []*StreamDecorator) {
	for i := range ds {
		c = ds[i].Decorate(c)
	}
	return c, ds
}
