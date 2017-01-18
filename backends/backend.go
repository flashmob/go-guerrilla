package backends

import (
	"errors"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/flashmob/go-guerrilla/envelope"
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

	// start save mail worker(s)
	saveMailWorker(chan *savePayload)
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

// A backend gateway is a proxy that implements the Backend interface.
// It is used to start multiple goroutine workers for saving mail, and then distribute email saving to the workers
// via a channel. Shutting down via Shutdown() will stop all workers.
// The rest of this program always talks to the backend via this gateway.
type BackendGateway struct {
	AbstractBackend
	saveMailChan chan *savePayload
	// waits for backend workers to start/stop
	wg sync.WaitGroup
	b  Backend
	// controls access to state
	stateGuard sync.Mutex
	state      int
}

// possible values for state
const (
	BackendStateRunning = iota
	BackendStateShutdown
	BackendStateError
)

// New retrieve a backend specified by the backendName, and initialize it using
// backendConfig
func New(backendName string, backendConfig BackendConfig) (Backend, error) {
	backend, found := backends[backendName]
	if !found {
		return nil, fmt.Errorf("backend %q not found", backendName)
	}
	p := &BackendGateway{b: backend}
	err := p.Initialize(backendConfig)
	if err != nil {
		return nil, fmt.Errorf("error while initializing the backend: %s", err)
	}
	p.state = BackendStateRunning
	return p, nil
}

// Distributes an envelope to one of the backend workers
func (gw *BackendGateway) Process(e *envelope.Envelope) BackendResult {
	if gw.state != BackendStateRunning {
		return NewBackendResult("554 Transaction failed - backend not running")
	}

	to := e.RcptTo
	from := e.MailFrom

	// place on the channel so that one of the save mail workers can pick it up
	// TODO: support multiple recipients
	savedNotify := make(chan *saveStatus)
	gw.saveMailChan <- &savePayload{e, from, &to[0], savedNotify}
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
func (gw *BackendGateway) Shutdown() error {
	gw.stateGuard.Lock()
	defer gw.stateGuard.Unlock()
	if gw.state != BackendStateShutdown {
		err := gw.b.Shutdown()
		if err == nil {
			close(gw.saveMailChan) // workers will stop
			gw.wg.Wait()
			gw.state = BackendStateShutdown
		}
		return err
	}
	return nil
}

func (gw *BackendGateway) Initialize(cfg BackendConfig) error {
	err := gw.b.Initialize(cfg)
	if err == nil {
		workersSize := gw.b.getNumberOfWorkers()
		if workersSize < 1 {
			gw.state = BackendStateError
			return errors.New("Must have at least 1 worker")
		}
		if err := gw.b.testSettings(); err != nil {
			gw.state = BackendStateError
			return err
		}
		gw.saveMailChan = make(chan *savePayload, workersSize)
		// start our savemail workers
		gw.wg.Add(workersSize)
		for i := 0; i < workersSize; i++ {
			go func() {
				gw.b.saveMailWorker(gw.saveMailChan)
				gw.wg.Done()
			}()
		}
	} else {
		gw.state = BackendStateError
	}
	return err
}
