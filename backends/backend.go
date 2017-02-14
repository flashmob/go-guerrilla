package backends

import (
	"fmt"
	"github.com/flashmob/go-guerrilla/envelope"
	"github.com/flashmob/go-guerrilla/log"
	"reflect"
	"strconv"
	"strings"
)

var (
	mainlog log.Logger
	Service *BackendService
	// deprecated backends system
	backends = map[string]Backend{}
	// new backends system
	Processors map[string]ProcessorConstructor
)

func init() {
	Service = &BackendService{}
	Processors = make(map[string]ProcessorConstructor)
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

/*
type Worker interface {
	// start save mail worker(s)
	saveMailWorker(chan *savePayload)
	// get the number of workers that will be stared
	getNumberOfWorkers() int
	// test database settings, permissions, correct paths, etc, before starting workers
	// parse the configuration files
	loadConfig(BackendConfig) error

	Shutdown() error
	Process(*envelope.Envelope) BackendResult
	Initialize(BackendConfig) error

	SetProcessors(p ...Decorator)
}
*/
type BackendConfig map[string]interface{}

type ProcessorConstructor func() Decorator

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

// extractConfig loads the backend config. It has already been unmarshalled
// configData contains data from the main config file's "backend_config" value
// configType is a Processor's specific config value.
// The reason why using reflection is because we'll get a nice error message if the field is missing
// the alternative solution would be to json.Marshal() and json.Unmarshal() however that will not give us any
// error messages
func (b *BackendService) extractConfig(configData BackendConfig, configType baseConfig) (interface{}, error) {
	// Use reflection so that we can provide a nice error message
	s := reflect.ValueOf(configType).Elem() // so that we can set the values
	m := reflect.ValueOf(configType).Elem()
	t := reflect.TypeOf(configType).Elem()
	typeOfT := s.Type()

	for i := 0; i < m.NumField(); i++ {
		f := s.Field(i)
		// read the tags of the config struct
		field_name := t.Field(i).Tag.Get("json")
		if len(field_name) > 0 {
			// parse the tag to
			// get the field name from struct tag
			split := strings.Split(field_name, ",")
			field_name = split[0]
		} else {
			// could have no tag
			// so use the reflected field name
			field_name = typeOfT.Field(i).Name
		}
		if f.Type().Name() == "int" {
			if intVal, converted := configData[field_name].(float64); converted {
				s.Field(i).SetInt(int64(intVal))
			} else {
				return configType, convertError("property missing/invalid: '" + field_name + "' of expected type: " + f.Type().Name())
			}
		}
		if f.Type().Name() == "string" {
			if stringVal, converted := configData[field_name].(string); converted {
				s.Field(i).SetString(stringVal)
			} else {
				return configType, convertError("missing/invalid: '" + field_name + "' of type: " + f.Type().Name())
			}
		}
		if f.Type().Name() == "bool" {
			if boolVal, converted := configData[field_name].(bool); converted {
				s.Field(i).SetBool(boolVal)
			} else {
				return configType, convertError("missing/invalid: '" + field_name + "' of type: " + f.Type().Name())
			}
		}
	}
	return configType, nil
}
