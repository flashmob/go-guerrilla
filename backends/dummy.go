package backends

import (
	"fmt"

	log "github.com/Sirupsen/logrus"

	guerrilla "github.com/flashmob/go-guerrilla"
)

func init() {
	backends["dummy"] = &DummyBackend{}
}

type DummyBackend struct {
	config dummyConfig
}

type dummyConfig struct {
	LogReceivedMails bool `json:"log_received_mails"`
}

func (b *DummyBackend) Initialize(backendConfig guerrilla.BackendConfig) error {
	var converted bool
	b.config.LogReceivedMails, converted = backendConfig["log_received_mails"].(bool)
	if !converted {
		return fmt.Errorf("failed to load backend config (%v)", backendConfig)
	}
	return nil
}

func (b *DummyBackend) Process(client *guerrilla.Client, user, host string) string {
	if b.config.LogReceivedMails {
		log.Infof("Mail from: %s@%s", user, host)
	}
	return fmt.Sprintf("250 OK : queued as %s", client.Hash)
}
