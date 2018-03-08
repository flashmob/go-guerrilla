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
	processors   []Processor
	validators   []Processor

	// controls access to state
	sync.Mutex
	State    backendState
	config   BackendConfig
	gwConfig *GatewayConfig
}

type GatewayConfig struct {
	// WorkersSize controls how many concurrent workers to start. Defaults to 1
	WorkersSize int `json:"save_workers_size,omitempty"`
	// SaveProcess controls which processors to chain in a stack for saving email tasks
	SaveProcess string `json:"save_process,omitempty"`
	// ValidateProcess is like ProcessorStack, but for recipient validation tasks
	ValidateProcess string `json:"validate_process,omitempty"`
	// TimeoutSave is duration before timeout when saving an email, eg "29s"
	TimeoutSave string `json:"gw_save_timeout,omitempty"`
	// TimeoutValidateRcpt duration before timeout when validating a recipient, eg "1s"
	TimeoutValidateRcpt string `json:"gw_val_rcpt_timeout,omitempty"`
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

var workerMsgPool = sync.Pool{
	// if not available, then create a new one
	New: func() interface{} {
		return &workerMsg{}
	},
}

// reset resets a workerMsg that has been borrowed from the pool
func (w *workerMsg) reset(e *mail.Envelope, task SelectTask) {
	if w.notifyMe == nil {
		w.notifyMe = make(chan *notifyMsg)
	}
	w.e = e
	w.task = task
}

// Process distributes an envelope to one of the backend workers with a TaskSaveMail task
func (gw *BackendGateway) Process(e *mail.Envelope) Result {
	if gw.State != BackendStateRunning {
		return NewResult(response.Canned.FailBackendNotRunning + gw.State.String())
	}
	// borrow a workerMsg from the pool
	workerMsg := workerMsgPool.Get().(*workerMsg)
	workerMsg.reset(e, TaskSaveMail)
	// place on the channel so that one of the save mail workers can pick it up
	gw.conveyor <- workerMsg
	// wait for the save to complete
	// or timeout
	select {
	case status := <-workerMsg.notifyMe:
		workerMsgPool.Put(workerMsg) // can be recycled since we used the notifyMe channel
		if status.err != nil {
			return NewResult(response.Canned.FailBackendTransaction + status.err.Error())
		}
		return NewResult(response.Canned.SuccessMessageQueued + status.queuedID)
	case <-time.After(gw.saveTimeout()):
		Log().Error("Backend has timed out while saving email")
		e.Lock() // lock the envelope - it's still processing here, we don't want the server to recycle it
		go func() {
			// keep waiting for the backend to finish processing
			<-workerMsg.notifyMe
			e.Unlock()
			workerMsgPool.Put(workerMsg)
		}()
		return NewResult(response.Canned.FailBackendTimeout)
	}
}

// ValidateRcpt asks one of the workers to validate the recipient
// Only the last recipient appended to e.RcptTo will be validated.
func (gw *BackendGateway) ValidateRcpt(e *mail.Envelope) RcptError {
	if gw.State != BackendStateRunning {
		return StorageNotAvailable
	}
	if _, ok := gw.validators[0].(NoopProcessor); ok {
		// no validator processors configured
		return nil
	}
	// place on the channel so that one of the save mail workers can pick it up
	workerMsg := workerMsgPool.Get().(*workerMsg)
	workerMsg.reset(e, TaskValidateRcpt)
	gw.conveyor <- workerMsg
	// wait for the validation to complete
	// or timeout
	select {
	case status := <-workerMsg.notifyMe:
		workerMsgPool.Put(workerMsg)
		if status.err != nil {
			return status.err
		}
		return nil

	case <-time.After(gw.validateRcptTimeout()):
		e.Lock()
		go func() {
			<-workerMsg.notifyMe
			e.Unlock()
			workerMsgPool.Put(workerMsg)
			Log().Error("Backend has timed out while validating rcpt")
		}()
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
	// clear the Initializers and Shutdowners
	Svc.reset()

	err := gw.Initialize(gw.config)
	if err != nil {
		fmt.Println("reinitialize to ", gw.config, err)
		return fmt.Errorf("error while initializing the backend: %s", err)
	}

	return err
}

// newStack creates a new Processor by chaining multiple Processors in a call stack
// Decorators are functions of Decorator type, source files prefixed with p_*
// Each decorator does a specific task during the processing stage.
// This function uses the config value save_process or validate_process to figure out which Decorator to use
func (gw *BackendGateway) newStack(stackConfig string) (Processor, error) {
	var decorators []Decorator
	cfg := strings.ToLower(strings.TrimSpace(stackConfig))
	if len(cfg) == 0 {
		//cfg = strings.ToLower(defaultProcessor)
		return NoopProcessor{}, nil
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
		return errors.New("can only Initialize in BackendStateNew or BackendStateShuttered state")
	}
	err := gw.loadConfig(cfg)
	if err != nil {
		gw.State = BackendStateError
		return err
	}
	workersSize := gw.workersSize()
	if workersSize < 1 {
		gw.State = BackendStateError
		return errors.New("must have at least 1 worker")
	}
	gw.processors = make([]Processor, 0)
	gw.validators = make([]Processor, 0)
	for i := 0; i < workersSize; i++ {
		p, err := gw.newStack(gw.gwConfig.SaveProcess)
		if err != nil {
			gw.State = BackendStateError
			return err
		}
		gw.processors = append(gw.processors, p)

		v, err := gw.newStack(gw.gwConfig.ValidateProcess)
		if err != nil {
			gw.State = BackendStateError
			return err
		}
		gw.validators = append(gw.validators, v)
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
				for {
					state := gw.workDispatcher(
						gw.conveyor,
						gw.processors[workerId],
						gw.validators[workerId],
						workerId+1,
						stop)
					// keep running after panic
					if state != dispatcherStatePanic {
						break
					}
				}
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
	if gw.gwConfig.WorkersSize <= 0 {
		return 1
	}
	return gw.gwConfig.WorkersSize
}

// saveTimeout returns the maximum amount of seconds to wait before timing out a save processing task
func (gw *BackendGateway) saveTimeout() time.Duration {
	if gw.gwConfig.TimeoutSave == "" {
		return saveTimeout
	}
	t, err := time.ParseDuration(gw.gwConfig.TimeoutSave)
	if err != nil {
		return saveTimeout
	}
	return t
}

// validateRcptTimeout returns the maximum amount of seconds to wait before timing out a recipient validation  task
func (gw *BackendGateway) validateRcptTimeout() time.Duration {
	if gw.gwConfig.TimeoutValidateRcpt == "" {
		return validateRcptTimeout
	}
	t, err := time.ParseDuration(gw.gwConfig.TimeoutValidateRcpt)
	if err != nil {
		return validateRcptTimeout
	}
	return t
}

type dispatcherState int

const (
	dispatcherStateStopped dispatcherState = iota
	dispatcherStateIdle
	dispatcherStateWorking
	dispatcherStateNotify
	dispatcherStatePanic
)

func (gw *BackendGateway) workDispatcher(
	workIn chan *workerMsg,
	save Processor,
	validate Processor,
	workerId int,
	stop chan bool) (state dispatcherState) {

	var msg *workerMsg

	defer func() {

		// panic recovery mechanism: it may panic when processing
		// since processors may call arbitrary code, some may be 3rd party / unstable
		// we need to detect the panic, and notify the backend that it failed & unlock the envelope
		if r := recover(); r != nil {
			Log().Error("worker recovered from panic:", r, string(debug.Stack()))

			if state == dispatcherStateWorking {
				msg.notifyMe <- &notifyMsg{err: errors.New("storage failed")}
			}
			state = dispatcherStatePanic
			return
		}
		// state is dispatcherStateStopped if it reached here
		return

	}()
	state = dispatcherStateIdle
	Log().Infof("processing worker started (#%d)", workerId)
	for {
		select {
		case <-stop:
			state = dispatcherStateStopped
			Log().Infof("stop signal for worker (#%d)", workerId)
			return
		case msg = <-workIn:
			state = dispatcherStateWorking // recovers from panic if in this state
			if msg.task == TaskSaveMail {
				// process the email here
				result, _ := save.Process(msg.e, TaskSaveMail)
				state = dispatcherStateNotify
				if result.Code() < 300 {
					// if all good, let the gateway know that it was saved
					msg.notifyMe <- &notifyMsg{nil, msg.e.QueuedId}
				} else {
					// notify the gateway about the error
					msg.notifyMe <- &notifyMsg{err: errors.New(result.String())}
				}
			} else if msg.task == TaskValidateRcpt {
				_, err := validate.Process(msg.e, TaskValidateRcpt)
				state = dispatcherStateNotify
				if err != nil {
					// validation failed
					msg.notifyMe <- &notifyMsg{err: err}
				} else {
					// all good.
					msg.notifyMe <- &notifyMsg{err: nil}
				}
			}
		}
		state = dispatcherStateIdle
	}
}

// stopWorkers sends a signal to all workers to stop
func (gw *BackendGateway) stopWorkers() {
	for i := range gw.workStoppers {
		gw.workStoppers[i] <- true
	}
}
