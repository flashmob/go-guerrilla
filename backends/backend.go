package backends

import (
	"fmt"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	Svc *service

	// Store the constructor for making an new processor decorator.
	processors map[string]ProcessorConstructor

	b Backend
)

func init() {
	Svc = &service{}
	processors = make(map[string]ProcessorConstructor)
}

type ProcessorConstructor func() Decorator

// Backends process received mail. Depending on the implementation, they can store mail in the database,
// write to a file, check for spam, re-transmit to another server, etc.
// Must return an SMTP message (i.e. "250 OK") and a boolean indicating
// whether the message was processed successfully.
type Backend interface {
	// Process processes then saves the mail envelope
	Process(*mail.Envelope) Result
	// ValidateRcpt validates the last recipient that was pushed to the mail envelope
	ValidateRcpt(e *mail.Envelope) RcptError
	// Initializes the backend, eg. creates folders, sets-up database connections
	Initialize(BackendConfig) error
	// Initializes the backend after it was Shutdown()
	Reinitialize() error
	// Shutdown frees / closes anything created during initializations
	Shutdown() error
	// Start Starts a backend that has been initialized
	Start() error
}

type BackendConfig map[string]interface{}

// All config structs extend from this
type BaseConfig interface{}

type notifyMsg struct {
	err      error
	queuedID string
}

// Result represents a response to an SMTP client after receiving DATA.
// The String method should return an SMTP message ready to send back to the
// client, for example `250 OK: Message received`.
type Result interface {
	fmt.Stringer
	// Code should return the SMTP code associated with this response, ie. `250`
	Code() int
}

// Internal implementation of BackendResult for use by backend implementations.
type result string

func (br result) String() string {
	return string(br)
}

// Parses the SMTP code from the first 3 characters of the SMTP message.
// Returns 554 if code cannot be parsed.
func (br result) Code() int {
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

func NewResult(message string) Result {
	return result(message)
}

type processorInitializer interface {
	Initialize(backendConfig BackendConfig) error
}

type processorShutdowner interface {
	Shutdown() error
}

type InitializeWith func(backendConfig BackendConfig) error
type ShutdownWith func() error

// Satisfy ProcessorInitializer interface
// So we can now pass an anonymous function that implements ProcessorInitializer
func (i InitializeWith) Initialize(backendConfig BackendConfig) error {
	// delegate to the anonymous function
	return i(backendConfig)
}

// satisfy ProcessorShutdowner interface, same concept as InitializeWith type
func (s ShutdownWith) Shutdown() error {
	// delegate
	return s()
}

type Errors []error

// implement the Error interface
func (e Errors) Error() string {
	if len(e) == 1 {
		return e[0].Error()
	}
	// multiple errors
	msg := ""
	for _, err := range e {
		msg += "\n" + err.Error()
	}
	return msg
}

func convertError(name string) error {
	return fmt.Errorf("failed to load backend config (%s)", name)
}

type service struct {
	initializers []processorInitializer
	shutdowners  []processorShutdowner
	sync.Mutex
	mainlog atomic.Value
}

// Get loads the log.logger in an atomic operation. Returns a stderr logger if not able to load
func Log() log.Logger {
	if v, ok := Svc.mainlog.Load().(log.Logger); ok {
		return v
	}
	l, _ := log.GetLogger(log.OutputStderr.String(), log.InfoLevel.String())
	return l
}

func (s *service) SetMainlog(l log.Logger) {
	s.mainlog.Store(l)
}

// AddInitializer adds a function that implements ProcessorShutdowner to be called when initializing
func (s *service) AddInitializer(i processorInitializer) {
	s.Lock()
	defer s.Unlock()
	s.initializers = append(s.initializers, i)
}

// AddShutdowner adds a function that implements ProcessorShutdowner to be called when shutting down
func (s *service) AddShutdowner(sh processorShutdowner) {
	s.Lock()
	defer s.Unlock()
	s.shutdowners = append(s.shutdowners, sh)
}

// reset clears the initializers and Shutdowners
func (s *service) reset() {
	s.shutdowners = make([]processorShutdowner, 0)
	s.initializers = make([]processorInitializer, 0)
}

// Initialize initializes all the processors one-by-one and returns any errors.
// Subsequent calls to Initialize will not call the initializer again unless it failed on the previous call
// so Initialize may be called again to retry after getting errors
func (s *service) initialize(backend BackendConfig) Errors {
	s.Lock()
	defer s.Unlock()
	var errors Errors
	failed := make([]processorInitializer, 0)
	for i := range s.initializers {
		if err := s.initializers[i].Initialize(backend); err != nil {
			errors = append(errors, err)
			failed = append(failed, s.initializers[i])
		}
	}
	// keep only the failed initializers
	s.initializers = failed
	return errors
}

// Shutdown shuts down all the processors by calling their shutdowners (if any)
// Subsequent calls to Shutdown will not call the shutdowners again unless it failed on the previous call
// so Shutdown may be called again to retry after getting errors
func (s *service) shutdown() Errors {
	s.Lock()
	defer s.Unlock()
	var errors Errors
	failed := make([]processorShutdowner, 0)
	for i := range s.shutdowners {
		if err := s.shutdowners[i].Shutdown(); err != nil {
			errors = append(errors, err)
			failed = append(failed, s.shutdowners[i])
		}
	}
	s.shutdowners = failed
	return errors
}

// AddProcessor adds a new processor, which becomes available to the backend_config.save_process option
// and also the backend_config.validate_process option
// Use to add your own custom processor when using backends as a package, or after importing an external
// processor.
func (s *service) AddProcessor(name string, p ProcessorConstructor) {
	// wrap in a constructor since we want to defer calling it
	var c ProcessorConstructor
	c = func() Decorator {
		return p()
	}
	// add to our processors list
	processors[strings.ToLower(name)] = c
}

// extractConfig loads the backend config. It has already been unmarshalled
// configData contains data from the main config file's "backend_config" value
// configType is a Processor's specific config value.
// The reason why using reflection is because we'll get a nice error message if the field is missing
// the alternative solution would be to json.Marshal() and json.Unmarshal() however that will not give us any
// error messages
func (s *service) ExtractConfig(configData BackendConfig, configType BaseConfig) (interface{}, error) {
	// Use reflection so that we can provide a nice error message
	v := reflect.ValueOf(configType).Elem() // so that we can set the values
	//m := reflect.ValueOf(configType).Elem()
	t := reflect.TypeOf(configType).Elem()
	typeOfT := v.Type()

	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		// read the tags of the config struct
		field_name := t.Field(i).Tag.Get("json")
		omitempty := false
		if len(field_name) > 0 {
			// parse the tag to
			// get the field name from struct tag
			split := strings.Split(field_name, ",")
			field_name = split[0]
			if len(split) > 1 {
				if split[1] == "omitempty" {
					omitempty = true
				}
			}
		} else {
			// could have no tag
			// so use the reflected field name
			field_name = typeOfT.Field(i).Name
		}
		if f.Type().Name() == "int" {
			// in json, there is no int, only floats...
			if intVal, converted := configData[field_name].(float64); converted {
				v.Field(i).SetInt(int64(intVal))
			} else if intVal, converted := configData[field_name].(int); converted {
				v.Field(i).SetInt(int64(intVal))
			} else if !omitempty {
				return configType, convertError("property missing/invalid: '" + field_name + "' of expected type: " + f.Type().Name())
			}
		}
		if f.Type().Name() == "string" {
			if stringVal, converted := configData[field_name].(string); converted {
				v.Field(i).SetString(stringVal)
			} else if !omitempty {
				return configType, convertError("missing/invalid: '" + field_name + "' of type: " + f.Type().Name())
			}
		}
		if f.Type().Name() == "bool" {
			if boolVal, converted := configData[field_name].(bool); converted {
				v.Field(i).SetBool(boolVal)
			} else if !omitempty {
				return configType, convertError("missing/invalid: '" + field_name + "' of type: " + f.Type().Name())
			}
		}
	}
	return configType, nil
}
