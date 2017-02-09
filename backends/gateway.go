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
)

// A backend gateway is a proxy that implements the Backend interface.
// It is used to start multiple goroutine workers for saving mail, and then distribute email saving to the workers
// via a channel. Shutting down via Shutdown() will stop all workers.
// The rest of this program always talks to the backend via this gateway.
type BackendGateway struct {
	AbstractBackend
	saveMailChan chan *savePayload
	// waits for backend workers to start/stop
	wg sync.WaitGroup
	b  Worker
	// controls access to state
	stateGuard sync.Mutex
	State      backendState
	config     BackendConfig
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
		return NewBackendResult(response.Canned.SuccessMessageQueued + status.hash)

	case <-time.After(time.Second * 30):
		mainlog.Infof("Backend has timed out")
		return NewBackendResult(response.Canned.FailBackendTimeout)
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
