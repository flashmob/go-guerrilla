package chunk

import (
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/mail/smtp"
	"io"
	"net"
	"time"
)

// Storage defines an interface to the storage layer (the database)
type Storage interface {
	// OpenMessage is used to begin saving an email. An email id is returned and used to call CloseMessage later
	OpenMessage(from string, helo string, recipient string, ipAddress net.IPAddr, returnPath string, isTLS bool, transport smtp.TransportType) (mailID uint64, err error)
	// CloseMessage finalizes the writing of an email. Additional data collected while parsing the email is saved
	CloseMessage(mailID uint64, size int64, partsInfo *PartsInfo, subject string, deliveryID string, to string, from string) error
	// AddChunk saves a chunk of bytes to a given hash key
	AddChunk(data []byte, hash []byte) error
	// GetEmail returns an email that's been saved
	GetEmail(mailID uint64) (*Email, error)
	// GetChunks loads in the specified chunks of bytes from storage
	GetChunks(hash ...HashKey) ([]*Chunk, error)
	// Initialize is called when the backend is started
	Initialize(cfg backends.BackendConfig) error
	// Shutdown is called when the backend gets shutdown.
	Shutdown() (err error)
}

// Email represents an email
type Email struct {
	mailID     uint64
	createdAt  time.Time
	size       int64
	from       string // from stores the email address found in the "From" header field
	to         string // to stores the email address found in the "From" header field
	partsInfo  PartsInfo
	helo       string // helo message given by the client when the message was transmitted
	subject    string // subject stores the value from the first "Subject" header field
	deliveryID string
	recipient  string             // recipient is the email address that the server received from the RCPT TO command
	ipv4       net.IPAddr         // set to a value if client connected via ipv4
	ipv6       net.IPAddr         // set to a value if client connected via ipv6
	returnPath string             // returnPath is the email address that the server received from the MAIL FROM command
	isTLS      bool               // isTLS is true when TLS was used to connect
	transport  smtp.TransportType // did the sender signal 8bitmime?
}

type Chunk struct {
	modifiedAt     time.Time
	referenceCount uint // referenceCount counts how many emails reference this chunk
	data           io.Reader
}
