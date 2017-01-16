package backends

import (
	log "github.com/Sirupsen/logrus"

	"errors"
	"fmt"
	"github.com/flashmob/go-guerrilla/envelope"
	"reflect"
	"strings"
)

type AbstractBackend struct {
	config abstractConfig
	extend Backend
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
	bcfg, err := b.extractConfig(backendConfig, configType)
	if err != nil {
		return err
	}
	m := bcfg.(*abstractConfig)
	b.config = *m
	return nil
}

func (b *AbstractBackend) Initialize(config BackendConfig) error {
	if b.extend != nil {
		return b.extend.loadConfig(config)
	}
	err := b.loadConfig(config)
	if err != nil {
		return err
	}
	return nil
}

func (b *AbstractBackend) Shutdown() error {
	if b.extend != nil {
		return b.extend.Shutdown()
	}
	return nil
}

func (b *AbstractBackend) Process(mail *envelope.Envelope) BackendResult {
	if b.extend != nil {
		return b.extend.Process(mail)
	}
	if b.config.LogReceivedMails {
		log.Infof("Mail from: %s / to: %v", mail.MailFrom.String(), mail.RcptTo)
	}
	return NewBackendResult("250 OK")
}

func (b *AbstractBackend) saveMailWorker(saveMailChan chan *savePayload) {
	if b.extend != nil {
		b.extend.saveMailWorker(saveMailChan)
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
	if b.extend != nil {
		return b.extend.getNumberOfWorkers()
	}
	return 1
}

func (b *AbstractBackend) testSettings() error {
	if b.extend != nil {
		return b.extend.testSettings()
	}
	return nil
}

// Load the backend config for the backend. It has already been unmarshalled
// from the main config file 'backend' config "backend_config"
// Now we need to convert each type and copy into the guerrillaDBAndRedisConfig struct
// The reason why using reflection is because we'll get a nice error message if the field is missing
// the alternative solution would be to json.Marshal() and json.Unmarshal() however that will not give us any
// error messages
func (h *AbstractBackend) extractConfig(configData BackendConfig, configType baseConfig) (interface{}, error) {
	// Use reflection so that we can provide a nice error message
	s := reflect.ValueOf(configType).Elem() // so that we can set the values
	m := reflect.ValueOf(configType).Elem()
	t := reflect.TypeOf(configType).Elem()
	typeOfT := s.Type()

	for i := 0; i < m.NumField(); i++ {
		f := s.Field(i)
		// read the tags of the config struct
		field_name := t.Field(i).Tag.Get("json")
		if len(field_name) > 0 {
			// parse the tag to
			// get the field name from struct tag
			split := strings.Split(field_name, ",")
			field_name = split[0]
		} else {
			// could have no tag
			// so use the reflected field name
			field_name = typeOfT.Field(i).Name
		}
		if f.Type().Name() == "int" {
			if intVal, converted := configData[field_name].(float64); converted {
				s.Field(i).SetInt(int64(intVal))
			} else {
				return configType, convertError("property missing/invalid: '" + field_name + "' of expected type: " + f.Type().Name())
			}
		}
		if f.Type().Name() == "string" {
			if stringVal, converted := configData[field_name].(string); converted {
				s.Field(i).SetString(stringVal)
			} else {
				return configType, convertError("missing/invalid: '" + field_name + "' of type: " + f.Type().Name())
			}
		}
	}
	return configType, nil
}
