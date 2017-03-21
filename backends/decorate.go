package backends

// We define what a decorator to our processor will look like
type Decorator func(Processor) Processor

// Decorate will decorate a processor with a slice of passed decorators
func Decorate(c Processor, ds ...Decorator) Processor {
	decorated := c
	for _, decorate := range ds {
		decorated = decorate(decorated)
	}
	return decorated
}
