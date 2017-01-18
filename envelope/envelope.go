package envelope

import (
	"bytes"
	"fmt"
)

// EmailAddress encodes an email address of the form `<user@host>`
type EmailAddress struct {
	User string
	Host string
}

func (ep *EmailAddress) String() string {
	return fmt.Sprintf("%s@%s", ep.User, ep.Host)
}

func (ep *EmailAddress) IsEmpty() bool {
	return ep.User == "" && ep.Host == ""
}

// Email represents a single SMTP message.
type Envelope struct {
	// Remote IP address
	RemoteAddress string
	// Message sent in EHLO command
	Helo string
	// Sender
	MailFrom *EmailAddress
	// Recipients
	RcptTo  []EmailAddress
	Data    bytes.Buffer
	Subject string
	TLS     bool
}
