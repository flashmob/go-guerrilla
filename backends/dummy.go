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

func (b *DummyBackend) Process(client *guerrilla.Client) string {
	if b.config.LogReceivedMails {
		log.Infof("Mail from: %s / to: %v", client.MailFrom.String(), client.RcptTo)
	}
	return fmt.Sprintf("250 OK : queued as %s", client.ID)
}
