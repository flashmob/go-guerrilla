package chunk

import (
	"bytes"
	"compress/zlib"
	"errors"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/smtp"
	"net"
	"time"
)

func init() {
	StorageEngines["memory"] = func() Storage {
		return new(StoreMemory)
	}
}

type storeMemoryConfig struct {
	CompressLevel int `json:"compress_level,omitempty"`
}

// A StoreMemory stores emails and chunked data in memory
type StoreMemory struct {
	chunks map[HashKey]*memoryChunk
	emails []*memoryEmail
	nextID uint64
	offset uint64
	config storeMemoryConfig
}

type memoryEmail struct {
	mailID     uint64
	createdAt  time.Time
	size       int64
	from       string
	to         string
	partsInfo  []byte
	helo       string
	subject    string
	queuedID   string
	recipient  string
	ipv4       IPAddr
	ipv6       IPAddr
	returnPath string
	transport  smtp.TransportType
	protocol   mail.Protocol
}

type memoryChunk struct {
	modifiedAt     time.Time
	referenceCount uint
	data           []byte
}

// OpenMessage implements the Storage interface
func (m *StoreMemory) OpenMessage(
	queuedID mail.Hash128,
	from string,
	helo string,
	recipient string,
	ipAddress IPAddr,
	returnPath string,
	protocol mail.Protocol,
	transport smtp.TransportType,
) (mailID uint64, err error) {
	var ip4, ip6 IPAddr
	if ip := ipAddress.IP.To4(); ip != nil {
		ip4 = IPAddr{net.IPAddr{IP: ip}}
	} else {
		ip6 = IPAddr{net.IPAddr{IP: ip}}
	}
	email := memoryEmail{
		queuedID:   queuedID.String(),
		mailID:     m.nextID,
		createdAt:  time.Now(),
		from:       from,
		helo:       helo,
		recipient:  recipient,
		ipv4:       ip4,
		ipv6:       ip6,
		returnPath: returnPath,
		transport:  transport,
		protocol:   protocol,
	}
	m.emails = append(m.emails, &email)
	m.nextID++
	return email.mailID, nil
}

// CloseMessage implements the Storage interface
func (m *StoreMemory) CloseMessage(
	mailID uint64,
	size int64,
	partsInfo *PartsInfo,
	subject string,
	to string,
	from string) error {
	if email := m.emails[mailID-m.offset]; email == nil {
		return errors.New("email not found")
	} else {
		email.size = size
		if info, err := partsInfo.MarshalJSONZlib(); err != nil {
			return err
		} else {
			email.partsInfo = info
		}
		email.subject = subject
		email.to = to
		email.from = from
		email.size = size
	}
	return nil
}

// AddChunk implements the Storage interface
func (m *StoreMemory) AddChunk(data []byte, hash []byte) error {
	var key HashKey
	if len(hash) != hashByteSize {
		return errors.New("invalid hash")
	}
	key.Pack(hash)
	var compressed bytes.Buffer
	zlibw, err := zlib.NewWriterLevel(&compressed, m.config.CompressLevel)
	if err != nil {
		return err
	}
	if chunk, ok := m.chunks[key]; ok {
		// only update the counters and update time
		chunk.referenceCount++
		chunk.modifiedAt = time.Now()
	} else {
		if _, err := zlibw.Write(data); err != nil {
			return err
		}
		if err := zlibw.Close(); err != nil {
			return err
		}
		// add a new chunk
		newChunk := memoryChunk{
			modifiedAt:     time.Now(),
			referenceCount: 1,
			data:           compressed.Bytes(),
		}
		m.chunks[key] = &newChunk
	}
	return nil
}

// Initialize implements the Storage interface
func (m *StoreMemory) Initialize(cfg backends.ConfigGroup) error {

	sd := backends.StreamDecorator{}
	err := sd.ExtractConfig(cfg, &m.config)
	if err != nil {
		return err
	}
	m.offset = 1
	m.nextID = m.offset
	m.emails = make([]*memoryEmail, 0, 100)
	m.chunks = make(map[HashKey]*memoryChunk, 1000)
	if m.config.CompressLevel > 9 || m.config.CompressLevel < 0 {
		m.config.CompressLevel = zlib.BestCompression
	}
	return nil
}

// Shutdown implements the Storage interface
func (m *StoreMemory) Shutdown() (err error) {
	m.emails = nil
	m.chunks = nil
	return nil
}

// GetEmail implements the Storage interface
func (m *StoreMemory) GetMessage(mailID uint64) (*Email, error) {
	if count := len(m.emails); count == 0 {
		return nil, errors.New("storage is empty")
	} else if overflow := uint64(count) - m.offset; overflow > mailID-m.offset {
		return nil, errors.New("mail not found")
	}
	email := m.emails[mailID-m.offset]
	pi := NewPartsInfo()
	if err := pi.UnmarshalJSONZlib(email.partsInfo); err != nil {
		return nil, err
	}
	return &Email{
		mailID:     email.mailID,
		createdAt:  email.createdAt,
		size:       email.size,
		from:       email.from,
		to:         email.to,
		partsInfo:  *pi,
		helo:       email.helo,
		subject:    email.subject,
		queuedID:   email.queuedID,
		recipient:  email.recipient,
		ipv4:       email.ipv4,
		ipv6:       email.ipv6,
		returnPath: email.returnPath,
		transport:  email.transport,
		protocol:   email.protocol,
	}, nil
}

// GetChunk implements the Storage interface
func (m *StoreMemory) GetChunks(hash ...HashKey) ([]*Chunk, error) {
	result := make([]*Chunk, 0, len(hash))
	var key HashKey
	for i := range hash {
		key = hash[i]
		if c, ok := m.chunks[key]; ok {
			zwr, err := zlib.NewReader(bytes.NewReader(c.data))
			if err != nil {
				return nil, err
			}
			result = append(result, &Chunk{
				modifiedAt:     c.modifiedAt,
				referenceCount: c.referenceCount,
				data:           zwr,
			})
		}
	}
	return result, nil
}
