package backends

import (
	"fmt"

	log "github.com/Sirupsen/logrus"

	guerrilla "github.com/jordanschalm/guerrilla"
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
		b.config = dummyConfig{willLog && ok}
	}
}

func (b *DummyBackend) Initialize(config map[string]interface{}) {
	b.loadConfig(config)
}

func (b *DummyBackend) Process(client *guerrilla.Client) (string, bool) {
	if b.config.LogReceivedMails {
		log.Infof("Mail from: %s / to: %v", client.MailFrom.String(), client.RcptTo)
	}
	return fmt.Sprintf("250 OK : queued as %s", client.ID), true
}
