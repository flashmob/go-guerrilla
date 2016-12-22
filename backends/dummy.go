package backends

import (
	log "github.com/Sirupsen/logrus"

	"github.com/flashmob/go-guerrilla"
)

type DummyBackend struct {
	config dummyConfig
}

type dummyConfig struct {
	LogReceivedMails bool `json:"log_received_mails"`
}

func (b *DummyBackend) loadConfig(config map[string]interface{}) {
	willLog, ok := config["log_received_mails"].(bool)
	if !ok {
		b.config = dummyConfig{false}
	} else {
		b.config = dummyConfig{willLog}
	}
}

func (b *DummyBackend) Initialize(config map[string]interface{}) {
	b.loadConfig(config)
}

func (b *DummyBackend) Process(mail *guerrilla.Envelope) guerrilla.BackendResult {
	if b.config.LogReceivedMails {
		log.Infof("Mail from: %s / to: %v", mail.MailFrom.String(), mail.RcptTo)
	}
	return guerrilla.NewBackendResult("250 OK")
}
