package backends

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/response"
	"runtime/debug"
)

// A backend gateway is a proxy that implements the Backend interface.
// It is used to start multiple goroutine workers for saving mail, and then distribute email saving to the workers
// via a channel. Shutting down via Shutdown() will stop all workers.
// The rest of this program always talks to the backend via this gateway.
type BackendGateway struct {
	// name is the name of the gateway given in the config
	name string
	// channel for distributing envelopes to workers
	conveyor           chan *workerMsg
	conveyorValidation chan *workerMsg
	conveyorStream     chan *workerMsg
	conveyorStreamBg   chan *workerMsg

	// waits for backend workers to start/stop
	wg           sync.WaitGroup
	workStoppers []chan bool
	processors   []Processor
	validators   []ValidatingProcessor
	streamers    []streamer
	background   []streamer

	producer *StreamDecorator

	decoratorLookup map[ConfigSection]map[string]*StreamDecorator

	workerID int

	// controls access to state
	sync.Mutex
	State    backendState
	config   BackendConfig
	gwConfig *GatewayConfig

	//buffers []byte // stream output buffer
	buffers map[int][]byte
}

// workerMsg is what get placed on the BackendGateway.saveMailChan channel
type workerMsg struct {
	// The email data
	e *mail.Envelope
	// notifyMe is used to notify the gateway of workers finishing their processing
	notifyMe chan *notifyMsg
	// select the task type
	task SelectTask
	// io.Reader for streamed processor
	r io.Reader
}

type streamer struct {
	// StreamProcessor is a chain of StreamProcessor
	sp StreamProcessor
	// so that we can call Open and Close
	d []*StreamDecorator
}

func (s streamer) Write(p []byte) (n int, err error) {
	return s.sp.Write(p)
}

func (s *streamer) open(e *mail.Envelope) error {
	var err Errors
	for i := range s.d {
		if s.d[i].Open != nil {
			if e := s.d[i].Open(e); e != nil {
				err = append(err, e)
			}
		}
	}
	if len(err) == 0 {
		return nil
	}
	return err
}

func (s *streamer) close() error {
	var err Errors
	// close in reverse order
	for i := len(s.d) - 1; i >= 0; i-- {
		if s.d[i].Close != nil {
			if e := s.d[i].Close(); e != nil {
				err = append(err, e)
			}
		}
	}
	if len(err) == 0 {
		return nil
	}
	return err
}

func (s *streamer) shutdown() error {
	var err Errors
	// shutdown in reverse order
	for i := len(s.d) - 1; i >= 0; i-- {
		if s.d[i].Shutdown != nil {
			if e := s.d[i].Shutdown(); e != nil {
				err = append(err, e)
			}
		}
	}
	if len(err) == 0 {
		return nil
	}
	return err
}

type backendState int

// possible values for state
const (
	BackendStateNew backendState = iota
	BackendStateRunning
	BackendStateShuttered
	BackendStateError
	BackendStateInitialized
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
func New(name string, backendConfig BackendConfig, l log.Logger) (Backend, error) {
	Svc.SetMainlog(l)
	gateway := &BackendGateway{name: name}
	backendConfig.toLower()
	// keep the a copy of the config
	gateway.config = backendConfig
	err := gateway.Initialize(backendConfig)
	if err != nil {
		return nil, fmt.Errorf("error while initializing the backend: %s", err)
	}

	return gateway, nil
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

func (gw *BackendGateway) Name() string {
	return gw.name
}

// Process distributes an envelope to one of the backend workers with a TaskSaveMail task
func (gw *BackendGateway) Process(e *mail.Envelope) Result {
	if gw.State != BackendStateRunning {
		return NewResult(response.Canned.FailBackendNotRunning, response.SP, gw.State)
	}
	// borrow a workerMsg from the pool
	workerMsg := workerMsgPool.Get().(*workerMsg)
	defer workerMsgPool.Put(workerMsg)
	workerMsg.reset(e, TaskSaveMail)
	// place on the channel so that one of the save mail workers can pick it up
	gw.conveyor <- workerMsg
	// wait for the save to complete
	// or timeout
	select {
	case status := <-workerMsg.notifyMe:
		// email saving transaction completed
		if status.result == BackendResultOK && status.queuedID != "" {
			return NewResult(response.Canned.SuccessMessageQueued, response.SP, status.queuedID)
		}

		// A custom result, there was probably an error, if so, log it
		if status.result != nil {
			if status.err != nil {
				Log().Error(status.err)
			}
			return status.result
		}

		// if there was no result, but there's an error, then make a new result from the error
		if status.err != nil {
			if _, err := strconv.Atoi(status.err.Error()[:3]); err != nil {
				return NewResult(response.Canned.FailBackendTransaction, response.SP, status.err)
			}
			return NewResult(status.err)
		}

		// both result & error are nil (should not happen)
		err := errors.New("no response from backend - processor did not return a result or an error")
		Log().Error(err)
		return NewResult(response.Canned.FailBackendTransaction, response.SP, err)

	case <-time.After(gw.saveTimeout()):
		Log().Fields("queuedId", e.QueuedId).Error("backend has timed out while saving email")
		e.Add(1) // lock the envelope - it's still processing here, we don't want the server to recycle it
		go func() {
			// keep waiting for the backend to finish processing
			<-workerMsg.notifyMe
			Log().Fields("queuedId", e.QueuedId).Error("finished processing mail after timeout")
			e.Done()
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
	defer workerMsgPool.Put(workerMsg)
	workerMsg.reset(e, TaskValidateRcpt)
	gw.conveyorValidation <- workerMsg
	// wait for the validation to complete
	// or timeout
	select {
	case status := <-workerMsg.notifyMe:
		if status.err != nil {
			return status.err
		}
		return nil

	case <-time.After(gw.validateRcptTimeout()):
		Log().Fields("queuedId", e.QueuedId).Error("backend has timed out while validating rcpt")
		e.Add(1) // lock the envelope - it's still processing here, we don't want the server to recycle it
		go func() {
			<-workerMsg.notifyMe
			Log().Fields("queuedId", e.QueuedId).Error("finished validating rcpt after timeout")
			e.Done()
		}()
		return StorageTimeout
	}
}

func (gw *BackendGateway) StreamOn() bool {
	return len(gw.gwConfig.SaveStream) != 0
}

// newStreamDecorator creates a new StreamDecorator and calls Configure with its corresponding configuration
// cs - the item of 'list' property, result from newStackStreamProcessorConfig()
// section - which section of the config
func (gw *BackendGateway) newStreamDecorator(cs stackConfigExpression, section ConfigSection) *StreamDecorator {
	if makeFunc, ok := Streamers[cs.name]; !ok {
		return nil
	} else {
		d := makeFunc()
		config := gw.config.lookupGroup(section, cs.String())
		if config == nil {
			config = ConfigGroup{}
		}
		if d.Configure != nil {
			if err := d.Configure(config); err != nil {
				return nil
			}
		}
		return d
	}
}

func (gw *BackendGateway) ProcessBackground(e *mail.Envelope) {

	if d := gw.producer; d == nil {
		Log().Error("gateway has failed creating a post_process_producer, check config")
		return
	} else {
		r, err := d.GetEmail(e.MessageID)
		if err != nil {
			Log().Fields("queuedID", e.QueuedId, "messageID", e.MessageID).
				Error("gateway background process aborted: email with messageID not found")
			return
		}

		// borrow a workerMsg from the pool
		workerMsg := workerMsgPool.Get().(*workerMsg)
		defer workerMsgPool.Put(workerMsg)

		workerMsg.reset(e, TaskSaveMailStream)
		workerMsg.r = r

		// place on the channel so that one of the save mail workers can pick it up
		// buffered channel will block if full
		select {
		case gw.conveyorStreamBg <- workerMsg:
			break
		case <-time.After(gw.saveTimeout()):
			Log().Fields("queuedID", e.QueuedId).Error("post-processing timeout - queue full, aborting")
			return
		}
		// process in the background
		for {
			select {
			case status := <-workerMsg.notifyMe:
				// email saving transaction completed
				var fields []interface{}
				var code int
				if status.result != nil {
					code = status.result.Code()
					fields = append(fields, "queuedID", e.QueuedId, "code", code)

				}
				if code > 200 && code < 300 {
					fields = append(fields, "messageID", e.MessageID)
					Log().Fields(fields...).Info("background process done")
					return
				}
				if status.err != nil {
					fields = append(fields, "error", status.err)
				}
				if len(fields) > 0 {
					Log().Fields(fields...).Error("post-process completed with an error")
					return
				}
				// both result & error are nil (should not happen)
				Log().Fields("queuedID", e.QueuedId).Error("no response from backend - post-process did not return a result or an error")
				return
			case <-time.After(gw.saveTimeout()):
				Log().Fields("queuedID", e.QueuedId).Error("background post-processing timed-out, will keep waiting")
				// don't return here, keep waiting for workerMsg.notifyMe
			}
		}
	}
}

func (gw *BackendGateway) ProcessStream(r io.Reader, e *mail.Envelope) (Result, int64, error) {
	res := response.Canned
	if gw.State != BackendStateRunning {
		return NewResult(res.FailBackendNotRunning, response.SP, gw.State), 0, errors.New(res.FailBackendNotRunning.String())
	}
	// borrow a workerMsg from the pool
	workerMsg := workerMsgPool.Get().(*workerMsg)
	workerMsgPool.Put(workerMsg)
	workerMsg.reset(e, TaskSaveMailStream)
	workerMsg.r = r
	// place on the channel so that one of the save mail workers can pick it up
	gw.conveyorStream <- workerMsg
	// wait for the save to complete
	// or timeout
	select {
	case status := <-workerMsg.notifyMe:
		// email saving transaction completed
		if status.result == BackendResultOK && status.queuedID != "" {
			return NewResult(res.SuccessMessageQueued, response.SP, status.queuedID), e.Size, status.err
		}
		// A custom result, there was probably an error, if so, log it
		if status.result != nil {
			return status.result, e.Size, status.err
		}
		// if there was no result, but there's an error, then make a new result from the error
		if status.err != nil {
			if _, err := strconv.Atoi(status.err.Error()[:3]); err != nil {
				return NewResult(res.FailBackendTransaction, response.SP, status.err), e.Size, status.err
			}
			return NewResult(status.err), e.Size, status.err
		}
		// both result & error are nil (should not happen)
		err := errors.New("no response from backend - processor did not return a result or an error")
		Log().Error(err)
		return NewResult(res.FailBackendTransaction, response.SP, err), e.Size, err

	case <-time.After(gw.saveTimeout()):
		Log().Fields("queuedID", e.QueuedId).Error("backend has timed out while saving email stream")
		e.Add(1) // lock the envelope - it's still processing here, we don't want the server to recycle it
		go func() {
			// keep waiting for the backend to finish processing
			<-workerMsg.notifyMe
			e.Done()
			Log().Fields("queuedID", e.QueuedId).Info("backend has finished saving email stream after timeout")
		}()
		return NewResult(res.FailBackendTimeout), -1, errors.New("gateway timeout")
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
		for stream := range gw.streamers {
			err := gw.streamers[stream].shutdown()
			if err != nil {
				Log().Fields("error", err, "gateway", gw.name).Error("failed shutting down stream")
			}
		}
		for stream := range gw.background {
			err := gw.background[stream].shutdown()
			if err != nil {
				Log().Fields("error", err, "gateway", gw.name).Error("failed shutting down background stream")
			}
		}
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
	c := newStackProcessorConfig(stackConfig, newAliasMap(gw.config[ConfigProcessors]))
	if len(c.list) == 0 {
		return NoopProcessor{}, nil
	}
	for i := range c.list {
		if makeFunc, ok := processors[c.list[i].name]; ok {
			decorators = append(decorators, makeFunc())
		} else {
			return nil, c.notFound(c.list[i].name)
		}
	}
	// build the call-stack of decorators
	p := Decorate(DefaultProcessor{}, decorators...)
	return p, nil
}

func (gw *BackendGateway) newStreamStack(stackConfig string) (streamer, error) {
	var decorators []*StreamDecorator

	noop := streamer{NoopStreamProcessor{}, decorators}
	groupName := ConfigStreamProcessors
	c := newStackStreamProcessorConfig(stackConfig, newAliasMap(gw.config[groupName]))
	if len(c.list) == 0 {
		return noop, nil
	}
	for i := range c.list {
		if d := gw.newStreamDecorator(c.list[i], groupName); d != nil {
			if gw.decoratorLookup[groupName] == nil {
				gw.decoratorLookup[groupName] = make(map[string]*StreamDecorator)
			}
			gw.decoratorLookup[groupName][c.list[i].String()] = d
			decorators = append(decorators, d)
		} else {
			return streamer{nil, decorators}, c.notFound(c.list[i].name)
		}
	}
	// build the call-stack of decorators
	sp, decorators := DecorateStream(&DefaultStreamProcessor{}, decorators)
	return streamer{sp, decorators}, nil
}

// loadConfig loads the config for the GatewayConfig
func (gw *BackendGateway) loadConfig(cfg BackendConfig) error {
	configType := BaseConfig(&GatewayConfig{})
	// Note: treat config values as immutable
	// if you need to change a config value, change in the file then
	// send a SIGHUP
	if gw.name == "" {
		gw.name = DefaultGateway
	}
	if _, ok := cfg[ConfigGateways][gw.name]; !ok {
		return errors.New("no such gateway configured: " + gw.name)
	}
	bcfg, err := Svc.ExtractConfig(ConfigGateways, gw.name, cfg, configType)
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
	gw.buffers = make(map[int][]byte) // individual buffers are made later
	gw.decoratorLookup = make(map[ConfigSection]map[string]*StreamDecorator)
	gw.processors = make([]Processor, 0)
	gw.validators = make([]ValidatingProcessor, 0)
	gw.streamers = make([]streamer, 0)
	gw.background = make([]streamer, 0)
	for i := 0; i < gw.gwConfig.saveWorkersCount(); i++ {
		p, err := gw.newStack(gw.gwConfig.SaveProcess)
		if err != nil {
			gw.State = BackendStateError
			return err
		}
		gw.processors = append(gw.processors, p)
	}
	for i := 0; i < gw.gwConfig.validateWorkersCount(); i++ {
		v, err := gw.newStack(gw.gwConfig.ValidateProcess)
		if err != nil {
			gw.State = BackendStateError
			return err
		}
		gw.validators = append(gw.validators, v)
	}
	for i := 0; i < gw.gwConfig.streamWorkersCount(); i++ {
		s, err := gw.newStreamStack(gw.gwConfig.SaveStream)
		if err != nil {
			gw.State = BackendStateError
			return err
		}
		gw.streamers = append(gw.streamers, s)
	}
	for i := 0; i < gw.gwConfig.backgroundWorkersCount(); i++ {
		c, err := gw.newStreamStack(gw.gwConfig.PostProcessConsumer)
		if err != nil {
			gw.State = BackendStateError
			return err
		}
		gw.background = append(gw.background, c)
	}
	if err = gw.initProducer(); err != nil {
		return err
	}

	// Initialize processors & stream processors
	if err := Svc.Initialize(cfg); err != nil {
		gw.State = BackendStateError
		return err
	}
	gw.conveyor = make(chan *workerMsg, gw.gwConfig.saveProcessSize())
	gw.conveyorValidation = make(chan *workerMsg, gw.gwConfig.validateProcessSize())
	gw.conveyorStream = make(chan *workerMsg, gw.gwConfig.saveStreamSize())
	gw.conveyorStreamBg = make(chan *workerMsg, gw.gwConfig.postProcessSize())
	// ready to start
	gw.State = BackendStateInitialized
	return nil
}

// Start starts the worker goroutines, assuming it has been initialized or shuttered before
func (gw *BackendGateway) Start() error {
	gw.Lock()
	defer gw.Unlock()
	if gw.State == BackendStateInitialized || gw.State == BackendStateShuttered {
		// make our slice of channels for stopping
		gw.workStoppers = make([]chan bool, 0)
		gw.startWorkers(gw.conveyor, gw.processors)
		gw.startWorkers(gw.conveyorValidation, gw.validators)
		gw.startWorkers(gw.conveyorStream, gw.streamers)
		gw.startWorkers(gw.conveyorStreamBg, gw.background)
		gw.State = BackendStateRunning
		return nil
	} else {
		return fmt.Errorf("cannot start backend because it's in %s state", gw.State)
	}
}

func (gw *BackendGateway) startWorkers(conveyor chan *workerMsg, processors interface{}) {
	p := reflect.ValueOf(processors)
	if reflect.TypeOf(processors).Kind() != reflect.Slice {
		panic("processors must be a slice")
	}
	// set the wait group (when stopping, it will block for all goroutines to exit)
	gw.wg.Add(p.Len())
	for i := 0; i < p.Len(); i++ {
		// set the buffer
		gw.buffers[gw.workerID] = gw.makeBuffer()
		// stop is a channel used for stopping the worker
		stop := make(chan bool)
		// start the worker and keep it running
		go func(workerId int, stop chan bool, i int) {
			// blocks here until the worker exits
			// for-loop used so that if workDispatcher panics, re-enter gw.workDispatcher
			for {
				state := gw.workDispatcher(
					conveyor,
					p.Index(i).Interface(),
					workerId,
					stop)
				// keep running after panic
				if state != dispatcherStatePanic {
					break
				}
			}
			gw.wg.Done()
		}(gw.workerID, stop, i)
		gw.workStoppers = append(gw.workStoppers, stop)
		gw.workerID++
	}
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
	processor interface{},
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

	}()
	state = dispatcherStateIdle
	Log().Fields("id", workerId+1, "gateway", gw.name).
		Info("processing worker started")
	for {
		select {
		case <-stop:
			state = dispatcherStateStopped
			Log().Fields("id", workerId+1, "gateway", gw.name).
				Info("stop signal for worker")
			return
		case msg = <-workIn:
			state = dispatcherStateWorking // recovers from panic if in this state
			switch v := processor.(type) {
			case Processor:
				result, err := v.Process(msg.e, msg.task)
				state = dispatcherStateNotify
				msg.notifyMe <- &notifyMsg{err: err, result: result, queuedID: msg.e.QueuedId.String()}
			case ValidatingProcessor:
				result, err := v.Process(msg.e, msg.task)
				state = dispatcherStateNotify
				msg.notifyMe <- &notifyMsg{err: err, result: result}
			case streamer:
				err := v.open(msg.e)
				if err == nil {
					if msg.e.Size, err = io.CopyBuffer(v, msg.r, gw.buffers[workerId]); err != nil {
						Log().Fields("error", err, "workerID", workerId+1).Error("stream writing failed")
					}
					if err = v.close(); err != nil {
						Log().Fields("error", err, "workerID", workerId+1).Error("stream closing failed")
					}
				}
				state = dispatcherStateNotify
				var result Result
				if err != nil {
					result = NewResult(response.Canned.FailBackendTransaction, err)
				} else {
					result = NewResult(response.Canned.SuccessMessageQueued, response.SP, msg.e.QueuedId)
				}
				msg.notifyMe <- &notifyMsg{err: err, result: result, queuedID: msg.e.QueuedId.String()}

			}
		}
		state = dispatcherStateIdle
	}
}

func (gw *BackendGateway) makeBuffer() []byte {
	if gw.buffers == nil {
		gw.buffers = make(map[int][]byte)
	}
	size := configStreamBufferSize
	if gw.gwConfig.StreamBufferSize > 0 {
		size = gw.gwConfig.StreamBufferSize
	}
	return make([]byte, size)
}

// stopWorkers sends a signal to all workers to stop
func (gw *BackendGateway) stopWorkers() {
	for i := range gw.workStoppers {
		gw.workStoppers[i] <- true
	}
	gw.workerID = 0
}

func (gw *BackendGateway) initProducer() error {
	notValid := errors.New("gateway has no valid [post_process_producer] configured")
	if gw.gwConfig.PostProcessConsumer == "" {
		// consumer not configured, so not active
		return nil
	}
	if gw.gwConfig.PostProcessProducer == "" {
		return notValid
	}
	section := ConfigStreamProcessors // which section of the config (stream_processors)
	m := newAliasMap(gw.config[section])
	c := newStackStreamProcessorConfig(gw.gwConfig.PostProcessProducer, m)
	if len(c.list) == 0 {
		return notValid
	}
	// check it there's already an instance of it
	if gw.decoratorLookup[section] != nil {
		if v, ok := gw.decoratorLookup[section][c.list[0].String()]; ok {
			gw.producer = v
			return nil
		}
	}
	if d := gw.newStreamDecorator(c.list[0], section); d != nil {
		// use a new instance
		gw.producer = d
		return nil
	} else {
		return errors.New("please check gateway config [post_process_producer]")
	}
}
