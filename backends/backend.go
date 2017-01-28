package backends

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/envelope"
	"github.com/flashmob/go-guerrilla/response"
)

var mainlog log.Logger

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
	State      int
	config     BackendConfig
}

// possible values for state
const (
	BackendStateRunning = iota
	BackendStateShuttered
	BackendStateError
)

// New retrieve a backend specified by the backendName, and initialize it using
// backendConfig
func New(backendName string, backendConfig BackendConfig, l log.Logger) (Backend, error) {
	backend, found := backends[backendName]
	mainlog = l
	if !found {
		return nil, fmt.Errorf("backend %q not found", backendName)
	}
	gateway := &BackendGateway{b: backend, config: backendConfig}
	err := gateway.Initialize(backendConfig)
	if err != nil {
		return nil, fmt.Errorf("error while initializing the backend: %s", err)
	}
	gateway.State = BackendStateRunning
	return gateway, nil
}

// Process distributes an envelope to one of the backend workers
func (gw *BackendGateway) Process(e *envelope.Envelope) BackendResult {
	if gw.State != BackendStateRunning {
		resp := &response.Response{
			EnhancedCode: response.OtherOrUndefinedProtocolStatus,
			BasicCode:    554,
			Class:        response.ClassPermanentFailure,
			Comment:      "Transaction failed - backend not running " + strconv.Itoa(gw.State),
		}
		return NewBackendResult(resp.String())
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
			resp := &response.Response{
				EnhancedCode: response.OtherOrUndefinedProtocolStatus,
				BasicCode:    554,
				Class:        response.ClassPermanentFailure,
				Comment:      "Error: " + status.err.Error(),
			}
			return NewBackendResult(resp.String())
		}
		resp := &response.Response{
			EnhancedCode: response.OtherStatus,
			BasicCode:    250,
			Class:        response.ClassSuccess,
			Comment:      fmt.Sprintf("OK : queued as %s", status.hash),
		}
		return NewBackendResult(resp.String())

	case <-time.After(time.Second * 30):
		mainlog.Infof("Backend has timed out")
		resp := &response.Response{
			EnhancedCode: response.OtherOrUndefinedProtocolStatus,
			BasicCode:    554,
			Class:        response.ClassPermanentFailure,
			Comment:      "Error: transaction timeout",
		}
		return NewBackendResult(resp.String())
	}
}
func (gw *BackendGateway) Shutdown() error {
	gw.stateGuard.Lock()
	defer gw.stateGuard.Unlock()
	if gw.State != BackendStateShuttered {
		err := gw.b.Shutdown()
		if err == nil {
			close(gw.saveMailChan) // workers will stop
			gw.wg.Wait()
			gw.State = BackendStateShuttered
		}
		return err
	}
	return nil
}

// Reinitialize starts up a backend gateway that was shutdown before
func (gw *BackendGateway) Reinitialize() error {
	if gw.State != BackendStateShuttered {
		return errors.New("backend must be in BackendStateshuttered state to Reinitialize")
	}
	err := gw.Initialize(gw.config)
	if err != nil {
		return fmt.Errorf("error while initializing the backend: %s", err)
	}
	gw.State = BackendStateRunning
	return err
}

func (gw *BackendGateway) Initialize(cfg BackendConfig) error {
	err := gw.b.Initialize(cfg)
	if err == nil {
		workersSize := gw.b.getNumberOfWorkers()
		if workersSize < 1 {
			gw.State = BackendStateError
			return errors.New("Must have at least 1 worker")
		}
		if err := gw.b.testSettings(); err != nil {
			gw.State = BackendStateError
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
		gw.State = BackendStateError
	}
	return err
}
