package backends

import (
	"errors"
	"fmt"
	"github.com/flashmob/go-guerrilla/envelope"
	"github.com/flashmob/go-guerrilla/log"
	"sync"
)

// Deprecated: ProxyBackend makes it possible to use the old backend system
type ProxyBackend struct {
	config       proxyConfig
	extend       proxy
	saveMailChan chan *savePayload
	State        backendState
	// waits for backend workers to start/stop
	wg sync.WaitGroup
}

// Deprecated: Use workerMsg instead
type savePayload struct {
	mail        *envelope.Envelope
	from        *envelope.EmailAddress
	recipient   *envelope.EmailAddress
	savedNotify chan *saveStatus
}

// Deprecated: Use notifyMsg instead
type saveStatus struct {
	err  error
	hash string
}

// Deprecated: Kept for compatibility, use BackendGateway instead
// AbstractBackend is an alias ProxyBackend for back-compatibility.
type AbstractBackend struct {
	extend proxy
	ProxyBackend
}

// extractConfig for compatibility only. Forward to Svc.ExtractConfig
func (ac *AbstractBackend) extractConfig(configData BackendConfig, configType BaseConfig) (interface{}, error) {
	return Svc.ExtractConfig(configData, configType)
}

// copy the extend field down to the Proxy
func (ac *AbstractBackend) Initialize(config BackendConfig) error {
	if ac.extend != nil {
		ac.ProxyBackend.extend = ac.extend
	}
	return ac.ProxyBackend.Initialize(config)
}

// Deprecated: backeConfig is an alias to BaseConfig, use BaseConfig instead
type baseConfig BaseConfig

// Deprecated: Use Log() instead to get a hold of a logger
var mainlog log.Logger

// Deprecated: proxy may implement backend interface or any of the interfaces below
type proxy interface{}

//
type saveMailWorker interface {
	// start save mail worker(s)
	saveMailWorker(chan *savePayload)
}

type numberOfWorkersGetter interface {
	// get the number of workers that will be stared
	getNumberOfWorkers() int
}

type settingsTester interface {
	// test database settings, permissions, correct paths, etc, before starting workers
	testSettings() error
}

type configLoader interface {
	// parse the configuration files
	loadConfig(BackendConfig) error
}

type proxyConfig struct {
	LogReceivedMails bool `json:"log_received_mails"`
}

// Your backend should implement this method and set b.config field with a custom config struct
// Therefore, your implementation would have your own custom config type instead of dummyConfig
func (pb *ProxyBackend) loadConfig(backendConfig BackendConfig) (err error) {
	// Load the backend config for the backend. It has already been unmarshalled
	// from the main config file 'backend' config "backend_config"
	// Now we need to convert each type and copy into the dummyConfig struct
	configType := BaseConfig(&proxyConfig{})
	bcfg, err := Svc.ExtractConfig(backendConfig, configType)
	if err != nil {
		return err
	}
	m := bcfg.(*proxyConfig)
	pb.config = *m
	return nil
}

func (pb *ProxyBackend) initialize(config BackendConfig) error {
	if cl, ok := pb.extend.(configLoader); ok {
		cl.loadConfig(config)
	}
	err := pb.loadConfig(config)
	if err != nil {
		return err
	}
	return nil
}

func (pb *ProxyBackend) Initialize(cfg BackendConfig) error {
	err := pb.initialize(cfg)
	if err == nil {
		workersSize := pb.getNumberOfWorkers()
		if workersSize < 1 {
			pb.State = BackendStateError
			return errors.New("Must have at least 1 worker")
		}
		if err := pb.testSettings(); err != nil {
			pb.State = BackendStateError
			return err
		}
		pb.saveMailChan = make(chan *savePayload, workersSize)
		// start our savemail workers
		pb.wg.Add(workersSize)
		for i := 0; i < workersSize; i++ {
			go func() {
				pb.saveMailWorker(pb.saveMailChan)
				pb.wg.Done()
			}()
		}
	} else {
		pb.State = BackendStateError
	}
	return err
}

func (pb *ProxyBackend) Shutdown() error {
	if b, ok := pb.extend.(Backend); ok {
		return b.Shutdown()
	}
	return nil
}

func (pb *ProxyBackend) ValidateRcpt(mail *envelope.Envelope) RcptError {
	if b, ok := pb.extend.(Backend); ok {
		return b.ValidateRcpt(mail)
	}
	return nil
}

func (pb *ProxyBackend) Process(mail *envelope.Envelope) Result {
	if b, ok := pb.extend.(Backend); ok {
		return b.Process(mail)
	}
	mail.ParseHeaders()

	if pb.config.LogReceivedMails {
		Log().Infof("Mail from: %s / to: %v", mail.MailFrom.String(), mail.RcptTo)
		Log().Info("Headers are: %s", mail.Header)

	}
	return NewResult("250 OK")
}

func (pb *ProxyBackend) saveMailWorker(saveMailChan chan *savePayload) {
	if s, ok := pb.extend.(saveMailWorker); ok {
		s.saveMailWorker(saveMailChan)
		return
	}

	defer func() {
		if r := recover(); r != nil {
			// recover form closed channel
			fmt.Println("Recovered in f", r)
		}
		// close any connections / files
		// ...

	}()
	for {
		payload := <-saveMailChan
		if payload == nil {
			Log().Debug("No more saveMailChan payload")
			return
		}
		// process the email here
		result := pb.Process(payload.mail)
		// if all good
		if result.Code() < 300 {
			payload.savedNotify <- &saveStatus{nil, "s0m3l337Ha5hva1u3LOL"}
		} else {
			payload.savedNotify <- &saveStatus{errors.New(result.String()), "s0m3l337Ha5hva1u3LOL"}
		}

	}
}

func (pb *ProxyBackend) getNumberOfWorkers() int {
	if n, ok := pb.extend.(numberOfWorkersGetter); ok {
		return n.getNumberOfWorkers()
	}
	return 1
}

func (b *ProxyBackend) testSettings() error {
	if t, ok := b.extend.(settingsTester); ok {
		return t.testSettings()
	}
	return nil
}
