package chunk

import (
	"bytes"
	"compress/zlib"
	"errors"
	"github.com/flashmob/go-guerrilla/backends"
	"net"
	"time"
)

type ChunkSaverMemory struct {
	chunks        map[HashKey]*chunkSaverMemoryChunk
	emails        []*chunkSaverMemoryEmail
	nextID        uint64
	IDOffset      uint64
	CompressLevel int
}

type chunkSaverMemoryEmail struct {
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
}

type chunkSaverMemoryChunk struct {
	modifiedAt     time.Time
	referenceCount uint
	data           []byte
}

// OpenMessage implements the ChunkSaverStorage interface
func (m *ChunkSaverMemory) OpenMessage(from string, helo string, recipient string, ipAddress net.IPAddr, returnPath string, isTLS bool) (mailID uint64, err error) {
	var ip4, ip6 net.IPAddr
	if ip := ipAddress.IP.To4(); ip != nil {
		ip4 = ipAddress
	} else {
		ip6 = ipAddress
	}
	email := chunkSaverMemoryEmail{
		mailID:     m.nextID,
		createdAt:  time.Now(),
		from:       from,
		helo:       helo,
		recipient:  recipient,
		ipv4:       ip4,
		ipv6:       ip6,
		returnPath: returnPath,
		isTLS:      isTLS,
	}
	m.emails = append(m.emails, &email)
	m.nextID++
	return email.mailID, nil
}

// CloseMessage implements the ChunkSaverStorage interface
func (m *ChunkSaverMemory) CloseMessage(mailID uint64, size int64, partsInfo *PartsInfo, subject string, deliveryID string, to string, from string) error {
	if email := m.emails[mailID-m.IDOffset]; email == nil {
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

// AddChunk implements the ChunkSaverStorage interface
func (m *ChunkSaverMemory) AddChunk(data []byte, hash []byte) error {
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
		newChunk := chunkSaverMemoryChunk{
			modifiedAt:     time.Now(),
			referenceCount: 1,
			data:           compressed.Bytes(),
		}
		m.chunks[key] = &newChunk
	}
	return nil
}

// Initialize implements the ChunkSaverStorage interface
func (m *ChunkSaverMemory) Initialize(cfg backends.BackendConfig) error {
	m.IDOffset = 1
	m.nextID = m.IDOffset
	m.emails = make([]*chunkSaverMemoryEmail, 0, 100)
	m.chunks = make(map[HashKey]*chunkSaverMemoryChunk, 1000)
	m.CompressLevel = zlib.NoCompression
	return nil
}

// Shutdown implements the ChunkSaverStorage interface
func (m *ChunkSaverMemory) Shutdown() (err error) {
	m.emails = nil
	m.chunks = nil
	return nil
}

// GetEmail implements the ChunkSaverStorage interface
func (m *ChunkSaverMemory) GetEmail(mailID uint64) (*ChunkSaverEmail, error) {
	if size := uint64(len(m.emails)) - m.IDOffset; size > mailID-m.IDOffset {
		return nil, errors.New("mail not found")
	}
	email := m.emails[mailID-m.IDOffset]
	pi := NewPartsInfo()
	if err := pi.UnmarshalJSONZlib(email.partsInfo); err != nil {
		return nil, err
	}
	return &ChunkSaverEmail{
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
	}, nil
}

// GetChunk implements the ChunkSaverStorage interface
func (m *ChunkSaverMemory) GetChunks(hash ...HashKey) ([]*ChunkSaverChunk, error) {
	result := make([]*ChunkSaverChunk, 0, len(hash))
	var key HashKey
	for i := range hash {
		key = hash[i]
		if c, ok := m.chunks[key]; ok {
			zwr, err := zlib.NewReader(bytes.NewReader(c.data))
			if err != nil {
				return nil, err
			}
			result = append(result, &ChunkSaverChunk{
				modifiedAt:     c.modifiedAt,
				referenceCount: c.referenceCount,
				data:           zwr,
			})
		}
	}
	return result, nil
}
