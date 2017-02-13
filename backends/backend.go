package backends

import (
	"fmt"
	"github.com/flashmob/go-guerrilla/envelope"
	"github.com/flashmob/go-guerrilla/log"
	"strconv"
	"strings"
)

var mainlog log.Logger

var Service *BackendService

func init() {
	Service = &BackendService{}
}

// Backends process received mail. Depending on the implementation, they can store mail in the database,
// write to a file, check for spam, re-transmit to another server, etc.
// Must return an SMTP message (i.e. "250 OK") and a boolean indicating
// whether the message was processed successfully.
type Backend interface {
	// Public methods
	Process(*envelope.Envelope) BackendResult
	Initialize(BackendConfig) error
	Shutdown() error
}

type Worker interface {
	// start save mail worker(s)
	saveMailWorker(chan *savePayload)
	// get the number of workers that will be stared
	getNumberOfWorkers() int
	// test database settings, permissions, correct paths, etc, before starting workers
	testSettings() error
	// parse the configuration files
	loadConfig(BackendConfig) error

	Shutdown() error
	Process(*envelope.Envelope) BackendResult
	Initialize(BackendConfig) error

	SetProcessors(p ...Decorator)
}

type BackendConfig map[string]interface{}

var backends = map[string]Worker{}

type baseConfig interface{}

type saveStatus struct {
	err  error
	hash string
}

type savePayload struct {
	mail *envelope.Envelope
	//from        *envelope.EmailAddress
	//recipient   *envelope.EmailAddress
	savedNotify chan *saveStatus
}

// BackendResult represents a response to an SMTP client after receiving DATA.
// The String method should return an SMTP message ready to send back to the
// client, for example `250 OK: Message received`.
type BackendResult interface {
	fmt.Stringer
	// Code should return the SMTP code associated with this response, ie. `250`
	Code() int
}

// Internal implementation of BackendResult for use by backend implementations.
type backendResult string

func (br backendResult) String() string {
	return string(br)
}

// Parses the SMTP code from the first 3 characters of the SMTP message.
// Returns 554 if code cannot be parsed.
func (br backendResult) Code() int {
	trimmed := strings.TrimSpace(string(br))
	if len(trimmed) < 3 {
		return 554
	}
	code, err := strconv.Atoi(trimmed[:3])
	if err != nil {
		return 554
	}
	return code
}

func NewBackendResult(message string) BackendResult {
	return backendResult(message)
}

type ProcessorInitializer interface {
	Initialize(backendConfig BackendConfig) error
}

type ProcessorShutdowner interface {
	Shutdown() error
}

type Initialize func(backendConfig BackendConfig) error
type Shutdown func() error

// Satisfy ProcessorInitializer interface
// So we can now pass an anonymous function that implements ProcessorInitializer
func (i Initialize) Initialize(backendConfig BackendConfig) error {
	// delegate to the anonymous function
	return i(backendConfig)
}

// satisfy ProcessorShutdowner interface, same concept as Initialize type
func (s Shutdown) Shutdown() error {
	// delegate
	return s()
}

type BackendService struct {
	ProcessorHandlers
}

type ProcessorHandlers struct {
	Initializers []ProcessorInitializer
	Shutdowners  []ProcessorShutdowner
}

func (b *BackendService) AddInitializer(i ProcessorInitializer) {
	b.Initializers = append(b.Initializers, i)
}

func (b *BackendService) AddShutdowner(i ProcessorShutdowner) {
	b.Shutdowners = append(b.Shutdowners, i)
}

func (b *BackendService) Initialize(backend BackendConfig) {
	for i := range b.Initializers {
		b.Initializers[i].Initialize(backend)
	}
}

func (b *BackendService) Shutdown() {
	for i := range b.Shutdowners {
		b.Shutdowners[i].Shutdown()
	}
}
