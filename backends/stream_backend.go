package backends

var (
	streamers map[string]StreamProcessorConstructor
)

func init() {

}

type processorCloser interface {
	Close() error
}

type CloseWith func() error

// satisfy processorCloser interface
func (c CloseWith) Close() error {
	// delegate
	return c()
}

type StreamProcessorConstructor func() *StreamDecorator

type streamService struct {
	service
}
