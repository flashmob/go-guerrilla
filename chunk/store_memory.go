package chunk

import (
	"bytes"
	"compress/zlib"
	"errors"
	"github.com/flashmob/go-guerrilla/backends"
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

type StoreMemory struct {
	chunks        map[HashKey]*memoryChunk
	emails        []*memoryEmail
	nextID        uint64
	offset        uint64
	CompressLevel int
	config        storeMemoryConfig
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
	deliveryID string
	recipient  string
	ipv4       net.IPAddr
	ipv6       net.IPAddr
	returnPath string
	isTLS      bool
	is8Bit     smtp.TransportType
}

type memoryChunk struct {
	modifiedAt     time.Time
	referenceCount uint
	data           []byte
}

// OpenMessage implements the Storage interface
func (m *StoreMemory) OpenMessage(from string, helo string, recipient string, ipAddress net.IPAddr, returnPath string, isTLS bool, transport smtp.TransportType) (mailID uint64, err error) {
	var ip4, ip6 net.IPAddr
	if ip := ipAddress.IP.To4(); ip != nil {
		ip4 = ipAddress
	} else {
		ip6 = ipAddress
	}
	email := memoryEmail{
		mailID:     m.nextID,
		createdAt:  time.Now(),
		from:       from,
		helo:       helo,
		recipient:  recipient,
		ipv4:       ip4,
		ipv6:       ip6,
		returnPath: returnPath,
		isTLS:      isTLS,
		is8Bit:     transport,
	}
	m.emails = append(m.emails, &email)
	m.nextID++
	return email.mailID, nil
}

// CloseMessage implements the Storage interface
func (m *StoreMemory) CloseMessage(mailID uint64, size int64, partsInfo *PartsInfo, subject string, deliveryID string, to string, from string) error {
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
		email.deliveryID = deliveryID
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
	zlibw, err := zlib.NewWriterLevel(&compressed, m.CompressLevel)
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
	m.CompressLevel = m.config.CompressLevel
	return nil
}

// Shutdown implements the Storage interface
func (m *StoreMemory) Shutdown() (err error) {
	m.emails = nil
	m.chunks = nil
	return nil
}

// GetEmail implements the Storage interface
func (m *StoreMemory) GetEmail(mailID uint64) (*Email, error) {
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
		deliveryID: email.deliveryID,
		recipient:  email.recipient,
		ipv4:       email.ipv4,
		ipv6:       email.ipv6,
		returnPath: email.returnPath,
		isTLS:      email.isTLS,
		transport:  email.is8Bit,
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
