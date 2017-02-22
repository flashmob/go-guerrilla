package backends

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/flashmob/go-guerrilla/envelope"
	"github.com/flashmob/go-guerrilla/response"
	"strings"
)

// A backend gateway is a proxy that implements the Backend interface.
// It is used to start multiple goroutine workers for saving mail, and then distribute email saving to the workers
// via a channel. Shutting down via Shutdown() will stop all workers.
// The rest of this program always talks to the backend via this gateway.
type BackendGateway struct {
	// channel for sending envelopes to process and save
	saveMailChan chan *workerMsg
	// channel for validating the last recipient added to the envelope
	validateRcptChan chan *workerMsg
	// waits for backend workers to start/stop
	wg sync.WaitGroup
	w  *Worker

	// controls access to state
	sync.Mutex
	State    backendState
	config   BackendConfig
	gwConfig *GatewayConfig
}

type GatewayConfig struct {
	WorkersSize   int    `json:"save_workers_size,omitempty"`
	ProcessorLine string `json:"process_stack,omitempty"`
}

// savePayload is what get placed on the BackendGateway.saveMailChan channel
type workerMsg struct {
	mail *envelope.Envelope
	// savedNotify is used to notify that the save operation completed
	notifyMe chan *notifyMsg
}

// possible values for state
const (
	BackendStateRunning = iota
	BackendStateShuttered
	BackendStateError
)

const ProcessTimeout = time.Second * 30

type backendState int

func (s backendState) String() string {
	return strconv.Itoa(int(s))
}

// Process distributes an envelope to one of the backend workers
func (gw *BackendGateway) Process(e *envelope.Envelope) Result {
	if gw.State != BackendStateRunning {
		return NewResult(response.Canned.FailBackendNotRunning + gw.State.String())
	}
	// place on the channel so that one of the save mail workers can pick it up
	savedNotify := make(chan *notifyMsg)
	gw.saveMailChan <- &workerMsg{e, savedNotify}
	// wait for the save to complete
	// or timeout
	select {
	case status := <-savedNotify:
		if status.err != nil {
			return NewResult(response.Canned.FailBackendTransaction + status.err.Error())
		}
		return NewResult(response.Canned.SuccessMessageQueued + status.queuedID)

	case <-time.After(ProcessTimeout):
		Log().Infof("Backend has timed out")
		return NewResult(response.Canned.FailBackendTimeout)
	}

}

// ValidateRcpt asks one of the workers to validate the recipient
// Only the last recipient appended to e.RcptTo will be validated.
func (gw *BackendGateway) ValidateRcpt(e *envelope.Envelope) RcptError {
	if gw.State != BackendStateRunning {
		return StorageNotAvailable
	}
	// place on the channel so that one of the save mail workers can pick it up
	notify := make(chan *notifyMsg)
	gw.validateRcptChan <- &workerMsg{e, notify}
	// wait for the validation to complete
	// or timeout
	select {
	case status := <-notify:
		if status.err != nil {
			return status.err
		}
		return nil

	case <-time.After(time.Second):
		Log().Infof("Backend has timed out")
		return StorageTimeout
	}
}

// Shutdown shuts down the backend and leaves it in BackendStateShuttered state
func (gw *BackendGateway) Shutdown() error {
	gw.Lock()
	defer gw.Unlock()
	if gw.State != BackendStateShuttered {
		close(gw.saveMailChan) // workers will stop
		// wait for workers to stop
		gw.wg.Wait()
		Svc.shutdown()
		gw.State = BackendStateShuttered
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

// newProcessorLine creates a new call-stack of decorators and returns as a single Processor
// Decorators are functions of Decorator type, source files prefixed with p_*
// Each decorator does a specific task during the processing stage.
// This function uses the config value process_stack to figure out which Decorator to use
func (gw *BackendGateway) newProcessorLine() Processor {
	var decorators []Decorator
	if len(gw.gwConfig.ProcessorLine) == 0 {
		return nil
	}
	line := strings.Split(strings.ToLower(gw.gwConfig.ProcessorLine), "|")
	for i := range line {
		name := line[len(line)-1-i] // reverse order, since decorators are stacked
		if makeFunc, ok := processors[name]; ok {
			decorators = append(decorators, makeFunc())
		}
	}
	// build the call-stack of decorators
	p := Decorate(DefaultProcessor{}, decorators...)
	return p
}

// loadConfig loads the config for the GatewayConfig
func (gw *BackendGateway) loadConfig(cfg BackendConfig) error {
	configType := BaseConfig(&GatewayConfig{})
	if _, ok := cfg["process_stack"]; !ok {
		cfg["process_stack"] = "Debugger"
	}
	if _, ok := cfg["save_workers_size"]; !ok {
		cfg["save_workers_size"] = 1
	}
	bcfg, err := Svc.ExtractConfig(cfg, configType)
	if err != nil {
		return err
	}
	gw.gwConfig = bcfg.(*GatewayConfig)
	return nil
}

// Initialize builds the workers and starts each worker in a goroutine
func (gw *BackendGateway) Initialize(cfg BackendConfig) error {
	gw.Lock()
	defer gw.Unlock()
	err := gw.loadConfig(cfg)
	if err == nil {
		workersSize := gw.workersSize()
		if workersSize < 1 {
			gw.State = BackendStateError
			return errors.New("Must have at least 1 worker")
		}
		var lines []Processor
		for i := 0; i < workersSize; i++ {
			lines = append(lines, gw.newProcessorLine())
		}
		// initialize processors
		if err := Svc.initialize(cfg); err != nil {
			return err
		}
		gw.saveMailChan = make(chan *workerMsg, workersSize)
		gw.validateRcptChan = make(chan *workerMsg, workersSize)
		// start our workers
		gw.wg.Add(workersSize)
		for i := 0; i < workersSize; i++ {
			go func(workerId int) {
				gw.w.workDispatcher(gw.saveMailChan, gw.validateRcptChan, lines[workerId], workerId+1)
				gw.wg.Done()
			}(i)
		}
	} else {
		gw.State = BackendStateError
	}
	return err
}

// workersSize gets the number of workers to use for saving email by reading the save_workers_size config value
// Returns 1 if no config value was set
func (gw *BackendGateway) workersSize() int {
	if gw.gwConfig.WorkersSize == 0 {
		return 1
	}
	return gw.gwConfig.WorkersSize
}
