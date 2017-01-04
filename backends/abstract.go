package backends

import (
	log "github.com/Sirupsen/logrus"

	"errors"
	"fmt"
	"github.com/flashmob/go-guerrilla/envelope"
)

type AbstractBackend struct {
	helper
	config          abstractConfig
	extendedBackend Backend
}

type abstractConfig struct {
	LogReceivedMails bool `json:"log_received_mails"`
}

// Your backend should implement this method and set b.config field with a custom config struct
// Therefore, your implementation would have your own custom config type instead of dummyConfig
func (b *AbstractBackend) loadConfig(backendConfig BackendConfig) (err error) {
	// Load the backend config for the backend. It has already been unmarshalled
	// from the main config file 'backend' config "backend_config"
	// Now we need to convert each type and copy into the dummyConfig struct
	configType := baseConfig(&abstractConfig{})
	bcfg, err := b.helper.extractConfig(backendConfig, configType)
	if err != nil {
		return err
	}
	m := bcfg.(*abstractConfig)
	b.config = *m
	return nil
}

func (b *AbstractBackend) Initialize(config BackendConfig) error {
	if b.extendedBackend != nil {
		return b.extendedBackend.loadConfig(config)
	}
	err := b.loadConfig(config)
	if err != nil {
		return err
	}
	return nil
}

func (b *AbstractBackend) Shutdown() error {
	if b.extendedBackend != nil {
		return b.extendedBackend.Shutdown()
	}
	return nil
}

func (b *AbstractBackend) Process(mail *envelope.Envelope) BackendResult {
	if b.extendedBackend != nil {
		return b.extendedBackend.Process(mail)
	}
	if b.config.LogReceivedMails {
		log.Infof("Mail from: %s / to: %v", mail.MailFrom.String(), mail.RcptTo)
	}
	return NewBackendResult("250 OK")
}

func (b *AbstractBackend) saveMailWorker() {
	if b.extendedBackend != nil {
		b.extendedBackend.saveMailWorker()
		return
	}
	defer func() {
		if r := recover(); r != nil {
			// recover form closed channel
			fmt.Println("Recovered in f", r)
		}
		// close any connections / files
		// ...

		// singnal our wait group
		b.wg.Done()
	}()
	for {
		payload := <-b.saveMailChan
		if payload == nil {
			log.Debug("No more saveMailChan payload")
			return
		}
		// process the email here
		result := b.Process(payload.mail)
		// if all good
		if result.Code() < 300 {
			payload.savedNotify <- &saveStatus{nil, "s0m3l337Ha5hva1u3LOL"}
		} else {
			payload.savedNotify <- &saveStatus{errors.New(result.String()), "s0m3l337Ha5hva1u3LOL"}
		}

	}
}

func (b *AbstractBackend) getNumberOfWorkers() int {
	if b.extendedBackend != nil {
		return b.extendedBackend.getNumberOfWorkers()
	}
	return 1
}

func (b *AbstractBackend) testSettings() error {
	if b.extendedBackend != nil {
		return b.extendedBackend.testSettings()
	}
	return nil
}
