package backends

import (
	"errors"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/flashmob/go-guerrilla/envelope"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Backends process received mail. Depending on the implementation, they can store mail in the database,
// write to a file, check for spam, re-transmit to another server, etc.
// Must return an SMTP message (i.e. "250 OK") and a boolean indicating
// whether the message was processed successfully.
type Backend interface {
	// Public methods
	Process(*envelope.Envelope) BackendResult
	Initialize(BackendConfig) error
	Shutdown() error

	// Private

	// start save mail worker
	saveMailWorker()
	// get the number of workers that will be stared
	getNumberOfWorkers() int
	// test database settings, permissions, correct paths, etc, before starting workers
	testSettings() error
	// parse the configuration files
	loadConfig(BackendConfig) error
}

type configLoader interface {
	loadConfig(backendConfig BackendConfig) (err error)
}

type BackendConfig map[string]interface{}

var backends = map[string]Backend{}

type baseConfig interface{}

type saveStatus struct {
	err  error
	hash string
}

type savePayload struct {
	mail        *envelope.Envelope
	from        *envelope.EmailAddress
	recipient   *envelope.EmailAddress
	savedNotify chan *saveStatus
}
type helper struct {
	saveMailChan chan *savePayload
	wg           sync.WaitGroup
}

// Load the backend config for the backend. It has already been unmarshalled
// from the main config file 'backend' config "backend_config"
// Now we need to convert each type and copy into the guerrillaDBAndRedisConfig struct
// The reason why using reflection is because we'll get a nice error message if the field is missing
// the alternative solution would be to json.Marshal() and json.Unmarshal() however that will not give us any
// error messages
func (h *helper) extractConfig(configData BackendConfig, configType baseConfig) (interface{}, error) {
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
	}
	return configType, nil
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

// New retrieve a backend specified by the backendName, and initialize it using
// backendConfig
func New(backendName string, backendConfig BackendConfig) (Backend, error) {
	backend, found := backends[backendName]
	if !found {
		return nil, fmt.Errorf("backend %q not found", backendName)
	}
	p := &backendProxy{b: backend}
	err := p.Initialize(backendConfig)
	if err != nil {
		return nil, fmt.Errorf("error while initializing the backend: %s", err)
	}
	return p, nil
}

type backendProxy struct {
	helper
	AbstractBackend
	b Backend
}

// Distributes an envelope to one of the backend workers
func (p *backendProxy) Process(e *envelope.Envelope) BackendResult {
	to := e.RcptTo
	from := e.MailFrom

	// place on the channel so that one of the save mail workers can pick it up
	// TODO: support multiple recipients
	savedNotify := make(chan *saveStatus)
	p.helper.saveMailChan <- &savePayload{e, from, &to[0], savedNotify}
	// wait for the save to complete
	// or timeout
	select {
	case status := <-savedNotify:
		if status.err != nil {
			return NewBackendResult("554 Error: " + status.err.Error())
		}
		return NewBackendResult(fmt.Sprintf("250 OK : queued as %s", status.hash))
	case <-time.After(time.Second * 30):
		log.Infof("Backend has timed out")
		return NewBackendResult("554 Error: transaction timeout")
	}
}
func (p *backendProxy) Shutdown() error {
	err := p.b.Shutdown()
	if err == nil {
		close(p.helper.saveMailChan) // workers will stop
		p.helper.wg.Wait()
	}
	return err

}

func (p *backendProxy) Initialize(cfg BackendConfig) error {
	err := p.b.Initialize(cfg)
	if err == nil {
		workersSize := p.b.getNumberOfWorkers()
		if workersSize < 1 {
			return errors.New("Must have at least 1 worker")
		}
		if err := p.b.testSettings(); err != nil {
			return err
		}

		p.helper.saveMailChan = make(chan *savePayload, workersSize)
		// start our savemail workers
		p.helper.wg.Add(workersSize)
		for i := 0; i < workersSize; i++ {
			go func() {
				p.b.saveMailWorker()
				p.helper.wg.Done()
			}()
		}
	}
	return err
}
