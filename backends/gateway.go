package backends

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/response"
	"runtime/debug"
	"strings"
)

var ErrProcessorNotFound error

// A backend gateway is a proxy that implements the Backend interface.
// It is used to start multiple goroutine workers for saving mail, and then distribute email saving to the workers
// via a channel. Shutting down via Shutdown() will stop all workers.
// The rest of this program always talks to the backend via this gateway.
type BackendGateway struct {
	// channel for distributing envelopes to workers
	conveyor chan *workerMsg

	// waits for backend workers to start/stop
	wg           sync.WaitGroup
	workStoppers []chan bool
	chains       []Processor

	// controls access to state
	sync.Mutex
	State    backendState
	config   BackendConfig
	gwConfig *GatewayConfig
}

type GatewayConfig struct {
	// WorkersSize controls how many concurrent workers to start. Defaults to 1
	WorkersSize int `json:"save_workers_size,omitempty"`
	// ProcessorStack controls which processors to chain in a stack.
	ProcessorStack string `json:"process_stack,omitempty"`
	// TimeoutSave is the number of seconds before timeout when saving an email
	TimeoutSave int `json:"gw_save_timeout,omitempty"`
	// TimeoutValidateRcpt is how many seconds before timeout when validating a recipient
	TimeoutValidateRcpt int `json:"gw_val_rcpt_timeout,omitempty"`
}

// workerMsg is what get placed on the BackendGateway.saveMailChan channel
type workerMsg struct {
	// The email data
	e *mail.Envelope
	// notifyMe is used to notify the gateway of workers finishing their processing
	notifyMe chan *notifyMsg
	// select the task type
	task SelectTask
}

type backendState int

// possible values for state
const (
	BackendStateNew backendState = iota
	BackendStateRunning
	BackendStateShuttered
	BackendStateError
	BackendStateInitialized

	// default timeout for saving email, if 'gw_save_timeout' not present in config
	saveTimeout = time.Second * 30
	// default timeout for validating rcpt to, if 'gw_val_rcpt_timeout' not present in config
	validateRcptTimeout = time.Second * 5
	defaultProcessor    = "Debugger"
)

func (s backendState) String() string {
	switch s {
	case BackendStateNew:
		return "NewState"
	case BackendStateRunning:
		return "RunningState"
	case BackendStateShuttered:
		return "ShutteredState"
	case BackendStateError:
		return "ErrorSate"
	case BackendStateInitialized:
		return "InitializedState"
	}
	return strconv.Itoa(int(s))
}

// New makes a new default BackendGateway backend, and initializes it using
// backendConfig and stores the logger
func New(backendConfig BackendConfig, l log.Logger) (Backend, error) {
	Svc.SetMainlog(l)
	gateway := &BackendGateway{}
	err := gateway.Initialize(backendConfig)
	if err != nil {
		return nil, fmt.Errorf("error while initializing the backend: %s", err)
	}
	// keep the config known to be good.
	gateway.config = backendConfig

	b = Backend(gateway)
	return b, nil
}

// Process distributes an envelope to one of the backend workers
func (gw *BackendGateway) Process(e *mail.Envelope) Result {
	if gw.State != BackendStateRunning {
		return NewResult(response.Canned.FailBackendNotRunning + gw.State.String())
	}
	// place on the channel so that one of the save mail workers can pick it up
	savedNotify := make(chan *notifyMsg)
	gw.conveyor <- &workerMsg{e, savedNotify, TaskSaveMail}
	// wait for the save to complete
	// or timeout
	select {
	case status := <-savedNotify:
		if status.err != nil {
			return NewResult(response.Canned.FailBackendTransaction + status.err.Error())
		}
		return NewResult(response.Canned.SuccessMessageQueued + status.queuedID)

	case <-time.After(gw.saveTimeout()):
		Log().Error("Backend has timed out while saving eamil")
		return NewResult(response.Canned.FailBackendTimeout)
	}
}

// ValidateRcpt asks one of the workers to validate the recipient
// Only the last recipient appended to e.RcptTo will be validated.
func (gw *BackendGateway) ValidateRcpt(e *mail.Envelope) RcptError {
	if gw.State != BackendStateRunning {
		return StorageNotAvailable
	}
	// place on the channel so that one of the save mail workers can pick it up
	notify := make(chan *notifyMsg)
	gw.conveyor <- &workerMsg{e, notify, TaskValidateRcpt}
	// wait for the validation to complete
	// or timeout
	select {
	case status := <-notify:
		if status.err != nil {
			return status.err
		}
		return nil

	case <-time.After(gw.validateRcptTimeout()):
		Log().Error("Backend has timed out while validating rcpt")
		return StorageTimeout
	}
}

// Shutdown shuts down the backend and leaves it in BackendStateShuttered state
func (gw *BackendGateway) Shutdown() error {
	gw.Lock()
	defer gw.Unlock()
	if gw.State != BackendStateShuttered {
		// send a signal to all workers
		gw.stopWorkers()
		// wait for workers to stop
		gw.wg.Wait()
		// call shutdown on all processor shutdowners
		if err := Svc.shutdown(); err != nil {
			return err
		}
		gw.State = BackendStateShuttered
	}
	return nil
}

// Reinitialize initializes the gateway with the existing config after it was shutdown
func (gw *BackendGateway) Reinitialize() error {
	if gw.State != BackendStateShuttered {
		return errors.New("backend must be in BackendStateshuttered state to Reinitialize")
	}
	//
	Svc.reset()

	err := gw.Initialize(gw.config)
	if err != nil {
		fmt.Println("reinitialize to ", gw.config, err)
		return fmt.Errorf("error while initializing the backend: %s", err)
	}

	return err
}

// newChain creates a new Processor by chaining multiple Processors in a call stack
// Decorators are functions of Decorator type, source files prefixed with p_*
// Each decorator does a specific task during the processing stage.
// This function uses the config value process_stack to figure out which Decorator to use
func (gw *BackendGateway) newChain() (Processor, error) {
	var decorators []Decorator
	cfg := strings.ToLower(strings.TrimSpace(gw.gwConfig.ProcessorStack))
	if len(cfg) == 0 {
		cfg = strings.ToLower(defaultProcessor)
	}
	items := strings.Split(cfg, "|")
	for i := range items {
		name := items[len(items)-1-i] // reverse order, since decorators are stacked
		if makeFunc, ok := processors[name]; ok {
			decorators = append(decorators, makeFunc())
		} else {
			ErrProcessorNotFound = errors.New(fmt.Sprintf("processor [%s] not found", name))
			return nil, ErrProcessorNotFound
		}
	}
	// build the call-stack of decorators
	p := Decorate(DefaultProcessor{}, decorators...)
	return p, nil
}

// loadConfig loads the config for the GatewayConfig
func (gw *BackendGateway) loadConfig(cfg BackendConfig) error {
	configType := BaseConfig(&GatewayConfig{})
	// Note: treat config values as immutable
	// if you need to change a config value, change in the file then
	// send a SIGHUP
	bcfg, err := Svc.ExtractConfig(cfg, configType)
	if err != nil {
		return err
	}
	gw.gwConfig = bcfg.(*GatewayConfig)
	return nil
}

// Initialize builds the workers and initializes each one
func (gw *BackendGateway) Initialize(cfg BackendConfig) error {
	gw.Lock()
	defer gw.Unlock()
	if gw.State != BackendStateNew && gw.State != BackendStateShuttered {
		return errors.New("Can only Initialize in BackendStateNew or BackendStateShuttered state")
	}
	err := gw.loadConfig(cfg)
	if err == nil {
		workersSize := gw.workersSize()
		if workersSize < 1 {
			gw.State = BackendStateError
			return errors.New("Must have at least 1 worker")
		}
		gw.chains = make([]Processor, 0)
		for i := 0; i < workersSize; i++ {
			p, err := gw.newChain()
			if err != nil {
				gw.State = BackendStateError
				return err
			}
			gw.chains = append(gw.chains, p)
		}
		// initialize processors
		if err := Svc.initialize(cfg); err != nil {
			gw.State = BackendStateError
			return err
		}
		if gw.conveyor == nil {
			gw.conveyor = make(chan *workerMsg, workersSize)
		}
		// ready to start
		gw.State = BackendStateInitialized
		return nil
	}
	gw.State = BackendStateError
	return err
}

// Start starts the worker goroutines, assuming it has been initialized or shuttered before
func (gw *BackendGateway) Start() error {
	gw.Lock()
	defer gw.Unlock()
	if gw.State == BackendStateInitialized || gw.State == BackendStateShuttered {
		// we start our workers
		workersSize := gw.workersSize()
		// make our slice of channels for stopping
		gw.workStoppers = make([]chan bool, 0)
		// set the wait group
		gw.wg.Add(workersSize)

		for i := 0; i < workersSize; i++ {
			stop := make(chan bool)
			go func(workerId int, stop chan bool) {
				// blocks here until the worker exits
				gw.workDispatcher(gw.conveyor, gw.chains[workerId], workerId+1, stop)
				gw.wg.Done()
			}(i, stop)
			gw.workStoppers = append(gw.workStoppers, stop)
		}
		gw.State = BackendStateRunning
		return nil
	} else {
		return errors.New(fmt.Sprintf("cannot start backend because it's in %s state", gw.State))
	}
}

// workersSize gets the number of workers to use for saving email by reading the save_workers_size config value
// Returns 1 if no config value was set
func (gw *BackendGateway) workersSize() int {
	if gw.gwConfig.WorkersSize == 0 {
		return 1
	}
	return gw.gwConfig.WorkersSize
}

// saveTimeout returns the maximum amount of seconds to wait before timing out a save processing task
func (gw *BackendGateway) saveTimeout() time.Duration {
	if gw.gwConfig.TimeoutSave == 0 {
		return saveTimeout
	}
	return time.Duration(gw.gwConfig.TimeoutSave)
}

// validateRcptTimeout returns the maximum amount of seconds to wait before timing out a recipient validation  task
func (gw *BackendGateway) validateRcptTimeout() time.Duration {
	if gw.gwConfig.TimeoutValidateRcpt == 0 {
		return validateRcptTimeout
	}
	return time.Duration(gw.gwConfig.TimeoutValidateRcpt)
}

func (gw *BackendGateway) workDispatcher(workIn chan *workerMsg, p Processor, workerId int, stop chan bool) {

	defer func() {
		if r := recover(); r != nil {
			// recover form closed channel
			Log().Error("worker recovered form panic:", r, string(debug.Stack()))
		}
		// close any connections / files
		Svc.shutdown()

	}()
	Log().Infof("processing worker started (#%d)", workerId)
	for {
		select {
		case <-stop:
			Log().Infof("stop signal for worker (#%d)", workerId)
			return
		case msg := <-workIn:
			if msg == nil {
				Log().Debugf("worker stopped (#%d)", workerId)
				return
			}
			msg.e.Lock()
			if msg.task == TaskSaveMail {
				// process the email here
				// TODO we should check the err
				result, _ := p.Process(msg.e, TaskSaveMail)
				if result.Code() < 300 {
					// if all good, let the gateway know that it was queued
					msg.notifyMe <- &notifyMsg{nil, msg.e.QueuedId}
				} else {
					// notify the gateway about the error
					msg.notifyMe <- &notifyMsg{err: errors.New(result.String())}
				}
			} else if msg.task == TaskValidateRcpt {
				_, err := p.Process(msg.e, TaskValidateRcpt)
				if err != nil {
					// validation failed
					msg.notifyMe <- &notifyMsg{err: err}
				} else {
					// all good.
					msg.notifyMe <- &notifyMsg{err: nil}
				}
			}
			msg.e.Unlock()
		}
	}
}

// stopWorkers sends a signal to all workers to stop
func (gw *BackendGateway) stopWorkers() {
	for i := range gw.workStoppers {
		gw.workStoppers[i] <- true
	}
}
