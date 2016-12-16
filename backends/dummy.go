package backends

import (
	"fmt"

	log "github.com/Sirupsen/logrus"

	guerrilla "github.com/flashmob/go-guerrilla"
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

func (b *DummyBackend) Process(client *guerrilla.Client) guerrilla.BackendResult {
	if b.config.LogReceivedMails {
		log.Infof("Mail from: %s / to: %v", client.MailFrom.String(), client.RcptTo)
	}
	return guerrilla.NewBackendResult(fmt.Sprintf("250 OK : queued as %d", client.ID))
}
