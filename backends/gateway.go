package backends

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/flashmob/go-guerrilla/envelope"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/response"
	"strings"
)

// A backend gateway is a proxy that implements the Backend interface.
// It is used to start multiple goroutine workers for saving mail, and then distribute email saving to the workers
// via a channel. Shutting down via Shutdown() will stop all workers.
// The rest of this program always talks to the backend via this gateway.
type BackendGateway struct {
	saveMailChan chan *savePayload
	// waits for backend workers to start/stop
	wg sync.WaitGroup
	w  *Worker
	b  Backend
	// controls access to state
	sync.Mutex
	State    backendState
	config   BackendConfig
	gwConfig *GatewayConfig
}

type GatewayConfig struct {
	WorkersSize   int    `json:"save_workers_size,omitempty"`
	ProcessorLine string `json:"process_line,omitempty"`
}

// savePayload is what get placed on the BackendGateway.saveMailChan channel
type savePayload struct {
	mail *envelope.Envelope
	// savedNotify is used to notify that the save operation completed
	savedNotify chan *saveStatus
}

// possible values for state
const (
	BackendStateRunning = iota
	BackendStateShuttered
	BackendStateError
)

type backendState int

func (s backendState) String() string {
	return strconv.Itoa(int(s))
}

// New retrieve a backend specified by the backendName, and initialize it using
// backendConfig
func New(backendName string, backendConfig BackendConfig, l log.Logger) (Backend, error) {
	Service.StoreMainlog(l)
	gateway := &BackendGateway{config: backendConfig}
	if backend, found := backends[backendName]; found {
		gateway.b = backend
	}
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
		return NewBackendResult(response.Canned.FailBackendNotRunning + gw.State.String())
	}
	// place on the channel so that one of the save mail workers can pick it up
	savedNotify := make(chan *saveStatus)
	gw.saveMailChan <- &savePayload{e, savedNotify}
	// wait for the save to complete
	// or timeout
	select {
	case status := <-savedNotify:
		if status.err != nil {
			return NewBackendResult(response.Canned.FailBackendTransaction + status.err.Error())
		}
		return NewBackendResult(response.Canned.SuccessMessageQueued + status.queuedID)

	case <-time.After(time.Second * 30):
		Log().Infof("Backend has timed out")
		return NewBackendResult(response.Canned.FailBackendTimeout)
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
		Service.Shutdown()
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
// This function uses the config value process_line to figure out which Decorator to use
func (gw *BackendGateway) newProcessorLine() Processor {
	var decorators []Decorator
	if len(gw.gwConfig.ProcessorLine) == 0 {
		return nil
	}
	line := strings.Split(strings.ToLower(gw.gwConfig.ProcessorLine), "|")
	for i := range line {
		name := line[len(line)-1-i] // reverse order, since decorators are stacked
		if makeFunc, ok := Processors[name]; ok {
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
	if _, ok := cfg["process_line"]; !ok {
		cfg["process_line"] = "Debugger"
	}
	if _, ok := cfg["save_workers_size"]; !ok {
		cfg["save_workers_size"] = 1
	}
	bcfg, err := Service.ExtractConfig(cfg, configType)
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
		workersSize := gw.getNumberOfWorkers()
		if workersSize < 1 {
			gw.State = BackendStateError
			return errors.New("Must have at least 1 worker")
		}
		var lines []Processor
		for i := 0; i < workersSize; i++ {
			lines = append(lines, gw.newProcessorLine())
		}
		// initialize processors
		if err := Service.Initialize(cfg); err != nil {
			return err
		}
		gw.saveMailChan = make(chan *savePayload, workersSize)
		// start our savemail workers
		gw.wg.Add(workersSize)
		for i := 0; i < workersSize; i++ {
			go func(workerId int) {
				gw.w.saveMailWorker(gw.saveMailChan, lines[workerId], workerId+1)
				gw.wg.Done()
			}(i)
		}
	} else {
		gw.State = BackendStateError
	}
	return err
}

// getNumberOfWorkers gets the number of workers to use for saving email by reading the save_workers_size config value
// Returns 1 if no config value was set
func (gw *BackendGateway) getNumberOfWorkers() int {
	if gw.gwConfig.WorkersSize == 0 {
		return 1
	}
	return gw.gwConfig.WorkersSize
}
