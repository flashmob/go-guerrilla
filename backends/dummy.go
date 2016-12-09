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

func (b *DummyBackend) loadConfig(backendConfig guerrilla.BackendConfig) error {
	var converted bool
	b.config.LogReceivedMails, converted = backendConfig["log_received_mails"].(bool)
	if !converted {
		return fmt.Errorf("failed to load backend config (%v)", backendConfig)
	}
	return nil
}

func (b *DummyBackend) Initialize(backendConfig guerrilla.BackendConfig) error {
	return b.loadConfig(backendConfig)
}

func (b *DummyBackend) Finalize() error {
	return nil
}

func (b *DummyBackend) Process(client *guerrilla.Client, from *guerrilla.EmailParts, to []*guerrilla.EmailParts) string {
	if b.config.LogReceivedMails {
		log.Infof("Mail from: %s / to: %v data:[%s]", from, to, client.Data)
	}
	return fmt.Sprintf("250 OK : queued as %s", client.Hash)
}
