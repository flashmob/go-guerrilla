package chunk

import (
	"crypto/md5"
	"errors"
	"hash"
	"strings"

	"github.com/flashmob/go-guerrilla/mail/mime"
)

type flushEvent func() error

type chunkingBuffer struct {
	buf          []byte
	flushTrigger flushEvent
}

// Flush signals that it's time to write the buffer out to storage
func (c *chunkingBuffer) Flush() error {
	if len(c.buf) == 0 {
		return nil
	}
	if c.flushTrigger != nil {
		if err := c.flushTrigger(); err != nil {
			return err
		}
	}
	c.Reset()
	return nil
}

// Reset sets the length back to 0, making it re-usable
func (c *chunkingBuffer) Reset() {
	c.buf = c.buf[:0] // set the length back to 0
}

// Write takes a p slice of bytes and writes it to the buffer.
// It will never grow the buffer, flushing it as soon as it's full.
func (c *chunkingBuffer) Write(p []byte) (i int, err error) {
	remaining := len(p) // number of bytes remaining to write
	bufCap := cap(c.buf)
	for {
		free := bufCap - len(c.buf)
		if free > remaining {
			// enough of room in the buffer
			c.buf = append(c.buf, p[i:i+remaining]...)
			i += remaining
			return
		} else {
			// fill the buffer to the 'brim' with a slice from p
			c.buf = append(c.buf, p[i:i+free]...)
			remaining -= free
			i += free
			err = c.Flush()
			if err != nil {
				return i, err
			}
			if remaining == 0 {
				return
			}
		}
	}
}

// CapTo caps the internal buffer to specified number of bytes, sets the length back to 0
func (c *chunkingBuffer) CapTo(n int) {
	if cap(c.buf) == n {
		return
	}
	c.buf = make([]byte, 0, n)
}

// ChunkingBufferMime decorates chunkingBuffer, specifying that to do when a flush event is triggered
type ChunkingBufferMime struct {
	chunkingBuffer
	current  *mime.Part
	Info     PartsInfo
	md5      hash.Hash
	database Storage
}

func NewChunkedBytesBufferMime() *ChunkingBufferMime {
	b := new(ChunkingBufferMime)
	b.chunkingBuffer.flushTrigger = func() error {
		return b.onFlush()
	}
	b.md5 = md5.New()
	b.buf = make([]byte, 0, chunkMaxBytes)
	return b
}

func (b *ChunkingBufferMime) SetDatabase(database Storage) {
	b.database = database
}

// onFlush is called whenever the flush event fires.
// - It saves the chunk to disk and adds the chunk's hash to the list.
// - It builds the b.Info.Parts structure
func (b *ChunkingBufferMime) onFlush() error {
	b.md5.Write(b.buf)
	var chash HashKey
	copy(chash[:], b.md5.Sum([]byte{}))
	if b.current == nil {
		return errors.New("b.current part is nil")
	}
	if size := len(b.Info.Parts); size > 0 && b.Info.Parts[size-1].PartId == b.current.Node {
		// existing part, just append the hash
		lastPart := &b.Info.Parts[size-1]
		lastPart.ChunkHash = append(lastPart.ChunkHash, chash)
		b.fillInfo(lastPart, size-1)
		lastPart.Size += uint(len(b.buf))
	} else {
		// add it as a new part
		part := ChunkedPart{
			PartId:          b.current.Node,
			ChunkHash:       []HashKey{chash},
			ContentBoundary: b.Info.boundary(b.current.ContentBoundary),
			Size:            uint(len(b.buf)),
		}
		b.fillInfo(&part, 0)
		b.Info.Parts = append(b.Info.Parts, part)
		b.Info.Count++
	}
	if err := b.database.AddChunk(b.buf, chash[:]); err != nil {
		return err
	}
	return nil
}

func (b *ChunkingBufferMime) fillInfo(cp *ChunkedPart, index int) {
	if cp.ContentType == "" && b.current.ContentType != nil {
		cp.ContentType = b.current.ContentType.String()
	}
	if cp.Charset == "" && b.current.Charset != "" {
		cp.Charset = b.current.Charset
	}
	if cp.TransferEncoding == "" && b.current.TransferEncoding != "" {
		cp.TransferEncoding = b.current.TransferEncoding
	}
	if cp.ContentDisposition == "" && b.current.ContentDisposition != "" {
		cp.ContentDisposition = b.current.ContentDisposition
		if strings.Contains(cp.ContentDisposition, "attach") {
			b.Info.HasAttach = true
		}
	}
	if cp.ContentType != "" {
		if b.Info.TextPart == -1 && strings.Contains(cp.ContentType, "text/plain") {
			b.Info.TextPart = index
		} else if b.Info.HTMLPart == -1 && strings.Contains(cp.ContentType, "text/html") {
			b.Info.HTMLPart = index
		}
	}
}

// Reset decorates the Reset method of the chunkingBuffer
func (b *ChunkingBufferMime) Reset() {
	b.md5.Reset()
	b.chunkingBuffer.Reset()
}

func (b *ChunkingBufferMime) CurrentPart(cp *mime.Part) {
	if b.current == nil {
		b.Info = *NewPartsInfo()
		b.Info.Parts = make([]ChunkedPart, 0, 3)
		b.Info.TextPart = -1
		b.Info.HTMLPart = -1
	}
	b.current = cp
}
