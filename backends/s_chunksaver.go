package backends

// ----------------------------------------------------------------------------------
// Processor Name: ChunkSaver
// ----------------------------------------------------------------------------------
// Description   : Takes the stream and saves it in chunks. Chunks are split on the
//               : chunksaver_chunk_size config setting, and also at the end of MIME parts,
//               : and after a header. This allows for basic de-duplication: we can take a
//               : hash of each chunk, then check the database to see if we have it already.
//               : We don't need to write it to the database, but take the reference of the
//               : previously saved chunk and only increment the reference count.
//               : The rationale to put headers and bodies into separate chunks is
//               : due to headers often containing more unique data, while the bodies are
//               : often duplicated, especially for messages that are CC'd or forwarded
// ----------------------------------------------------------------------------------
// Requires      : "mimeanalyzer" stream processor to be enabled before it
// ----------------------------------------------------------------------------------
// Config Options: chunksaver_chunk_size - maximum chunk size, in bytes
// --------------:-------------------------------------------------------------------
// Input         : e.Values["MimeParts"] Which is of type *[]*mime.Part, as populated by "mimeanalyzer"
// ----------------------------------------------------------------------------------
// Output        :
// ----------------------------------------------------------------------------------

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/mime"
	"hash"
	"io"
	"io/ioutil"
	"net"
	"strings"
	"sync"
	"time"
)

type chunkSaverConfig struct {
	// ChunkMaxBytes controls the maximum buffer size for saving
	// 16KB default.
	ChunkMaxBytes int    `json:"chunksaver_chunk_size,omitempty"`
	StorageEngine string `json:"chunksaver_storage_engine,omitempty"`
	CompressLevel int    `json:"chunksaver_compress_level,omitempty"`
}

func init() {
	streamers["chunksaver"] = func() *StreamDecorator {
		return Chunksaver()
	}
}

const hashByteSize = 16

type HashKey [hashByteSize]byte

// Pack takes a slice and copies each byte to HashKey internal representation
func (h *HashKey) Pack(b []byte) {
	if len(b) < hashByteSize {
		return
	}
	copy(h[:], b[0:hashByteSize])
}

// String implements the Stringer interface from fmt
func (h HashKey) String() string {
	return base64.RawStdEncoding.EncodeToString(h[0:hashByteSize])
}

// UnmarshalJSON implements the Unmarshaler interface from encoding/json
func (h *HashKey) UnmarshalJSON(b []byte) error {
	dbuf := make([]byte, base64.RawStdEncoding.DecodedLen(len(b[1:len(b)-1])))
	_, err := base64.RawStdEncoding.Decode(dbuf, b[1:len(b)-1])
	if err != nil {
		return err
	}
	h.Pack(dbuf)
	return nil
}

// MarshalJSON implements the Marshaler interface from encoding/json
// The value is marshaled as a raw base64 to save some bytes
// eg. instead of typically using hex, de17038001170380011703ff01170380 would be represented as 3hcDgAEXA4ABFwP/ARcDgA
func (h *HashKey) MarshalJSON() ([]byte, error) {
	return []byte(`"` + h.String() + `"`), nil
}

// PartsInfo describes the mime-parts contained in the email
type PartsInfo struct {
	Count       uint32        `json:"c"`   // number of parts
	TextPart    int           `json:"tp"`  // index of the main text part to display
	HTMLPart    int           `json:"hp"`  // index of the main html part to display (if any)
	HasAttach   bool          `json:"a"`   // is there an attachment?
	Parts       []ChunkedPart `json:"p"`   // info describing a mime-part
	CBoundaries []string      `json:"cbl"` // content boundaries list

	bp sync.Pool // bytes.buffer pool
}

// ChunkedPart contains header information about a mime-part, including keys pointing to where the data is stored at
type ChunkedPart struct {
	PartId             string    `json:"i"`
	Size               uint      `json:"s"`
	ChunkHash          []HashKey `json:"h"` // sequence of hashes the data is stored at
	ContentType        string    `json:"t"`
	Charset            string    `json:"c"`
	TransferEncoding   string    `json:"e"`
	ContentDisposition string    `json:"d"`
	ContentBoundary    int       `json:"cb"` // index to the CBoundaries list in PartsInfo
}

func NewPartsInfo() *PartsInfo {
	pi := new(PartsInfo)
	pi.bp = sync.Pool{
		// if not available, then create a new one
		New: func() interface{} {
			var b bytes.Buffer
			return &b
		},
	}
	return pi
}

// boundary takes a string and returns the index of the string in the info.CBoundaries slice
func (info *PartsInfo) boundary(cb string) int {
	for i := range info.CBoundaries {
		if info.CBoundaries[i] == cb {
			return i
		}
	}
	info.CBoundaries = append(info.CBoundaries, cb)
	return len(info.CBoundaries) - 1
}

// UnmarshalJSON unmarshals the JSON and decompresses using zlib
func (info *PartsInfo) UnmarshalJSONZlib(b []byte) error {

	r, err := zlib.NewReader(bytes.NewReader(b[1 : len(b)-1]))
	if err != nil {
		return err
	}
	all, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	err = json.Unmarshal(all, info)
	if err != nil {
		return err
	}
	return nil
}

// MarshalJSONZlib marshals and compresses the bytes using zlib
func (info *PartsInfo) MarshalJSONZlib() ([]byte, error) {

	buf, err := json.Marshal(info)
	if err != nil {
		return buf, err
	}
	// borrow a buffer form the pool
	compressed := info.bp.Get().(*bytes.Buffer)
	// put back in the pool
	defer func() {
		compressed.Reset()
		info.bp.Put(b)
	}()

	zlibw, err := zlib.NewWriterLevel(compressed, 9)
	if err != nil {
		return buf, err
	}
	if _, err := zlibw.Write(buf); err != nil {
		return buf, err
	}
	if err := zlibw.Close(); err != nil {
		return buf, err
	}
	return []byte(`"` + compressed.String() + `"`), nil
}

type flushEvent func() error

type chunkedBytesBuffer struct {
	buf          []byte
	flushTrigger flushEvent
}

// flush signals that it's time to write the buffer out to storage
func (c *chunkedBytesBuffer) flush() error {
	if len(c.buf) == 0 {
		return nil
	}
	fmt.Print(string(c.buf))
	if c.flushTrigger != nil {
		if err := c.flushTrigger(); err != nil {
			return err
		}
	}
	c.Reset()
	return nil
}

// Reset sets the length back to 0, making it re-usable
func (c *chunkedBytesBuffer) Reset() {
	c.buf = c.buf[:0] // set the length back to 0
}

// Write takes a p slice of bytes and writes it to the buffer.
// It will never grow the buffer, flushing it as soon as it's full.
func (c *chunkedBytesBuffer) Write(p []byte) (i int, err error) {
	remaining := len(p)
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
			err = c.flush()
			if err != nil {
				return i, err
			}
			if remaining == 0 {
				return
			}
		}
	}
}

// capTo caps the internal buffer to specified number of bytes, sets the length back to 0
func (c *chunkedBytesBuffer) capTo(n int) {
	if cap(c.buf) == n {
		return
	}
	c.buf = make([]byte, 0, n)
}

// chunkedBytesBufferMime decorates chunkedBytesBuffer, specifying that to do when a flush event is triggered
type chunkedBytesBufferMime struct {
	chunkedBytesBuffer
	current  *mime.Part
	info     PartsInfo
	md5      hash.Hash
	database ChunkSaverStorage
}

func newChunkedBytesBufferMime() *chunkedBytesBufferMime {
	b := new(chunkedBytesBufferMime)
	b.chunkedBytesBuffer.flushTrigger = func() error {
		return b.onFlush()
	}
	b.md5 = md5.New()
	b.buf = make([]byte, 0, chunkMaxBytes)
	return b
}

func (b *chunkedBytesBufferMime) setDatabase(database ChunkSaverStorage) {
	b.database = database
}

// onFlush is called whenever the flush event fires.
// - It saves the chunk to disk and adds the chunk's hash to the list.
// - It builds the b.info.Parts structure
func (b *chunkedBytesBufferMime) onFlush() error {
	b.md5.Write(b.buf)
	var chash HashKey
	copy(chash[:], b.md5.Sum([]byte{}))
	if b.current == nil {
		return errors.New("b.current part is nil")
	}
	if size := len(b.info.Parts); size > 0 && b.info.Parts[size-1].PartId == b.current.Node {
		// existing part, just append the hash
		lastPart := &b.info.Parts[size-1]
		lastPart.ChunkHash = append(lastPart.ChunkHash, chash)
		b.fillInfo(lastPart, size-1)
		lastPart.Size += uint(len(b.buf))
	} else {
		// add it as a new part
		part := ChunkedPart{
			PartId:          b.current.Node,
			ChunkHash:       []HashKey{chash},
			ContentBoundary: b.info.boundary(b.current.ContentBoundary),
			Size:            uint(len(b.buf)),
		}
		b.fillInfo(&part, 0)
		b.info.Parts = append(b.info.Parts, part)
		b.info.Count++
	}
	if err := b.database.AddChunk(b.buf, chash[:]); err != nil {
		return err
	}
	return nil
}

func (b *chunkedBytesBufferMime) fillInfo(cp *ChunkedPart, index int) {
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
			b.info.HasAttach = true
		}
	}
	if cp.ContentType != "" {
		if b.info.TextPart == -1 && strings.Contains(cp.ContentType, "text/plain") {
			b.info.TextPart = index
		} else if b.info.HTMLPart == -1 && strings.Contains(cp.ContentType, "text/html") {
			b.info.HTMLPart = index
		}
	}
}

// Reset decorates the Reset method of the chunkedBytesBuffer
func (b *chunkedBytesBufferMime) Reset() {
	b.md5.Reset()
	b.chunkedBytesBuffer.Reset()
}

func (b *chunkedBytesBufferMime) currentPart(cp *mime.Part) {
	if b.current == nil {
		b.info = *NewPartsInfo()
		b.info.Parts = make([]ChunkedPart, 0, 3)
		b.info.TextPart = -1
		b.info.HTMLPart = -1
	}
	b.current = cp
}

// ChunkSaverStorage defines an interface to the storage layer (the database)
type ChunkSaverStorage interface {
	// OpenMessage is used to begin saving an email. An email id is returned and used to call CloseMessage later
	OpenMessage(from string, helo string, recipient string, ipAddress net.IPAddr, returnPath string, isTLS bool) (mailID uint64, err error)
	// CloseMessage finalizes the writing of an email. Additional data collected while parsing the email is saved
	CloseMessage(mailID uint64, size int64, partsInfo *PartsInfo, subject string, deliveryID string, to string, from string) error
	// AddChunk saves a chunk of bytes to a given hash key
	AddChunk(data []byte, hash []byte) error
	// GetEmail returns an email that's been saved
	GetEmail(mailID uint64) (*ChunkSaverEmail, error)
	// GetChunks loads in the specified chunks of bytes from storage
	GetChunks(hash ...HashKey) ([]*ChunkSaverChunk, error)
	// Initialize is called when the backend is started
	Initialize(cfg BackendConfig) error
	// Shutdown is called when the backend gets shutdown.
	Shutdown() (err error)
}

// ChunkSaverEmail represents an email
type ChunkSaverEmail struct {
	mailID     uint64
	createdAt  time.Time
	size       int64
	from       string // from stores the email address found in the "From" header field
	to         string // to stores the email address found in the "From" header field
	partsInfo  PartsInfo
	helo       string // helo message given by the client when the message was transmitted
	subject    string // subject stores the value from the first "Subject" header field
	deliveryID string
	recipient  string     // recipient is the email address that the server received from the RCPT TO command
	ipv4       net.IPAddr // set to a value if client connected via ipv4
	ipv6       net.IPAddr // set to a value if client connected via ipv6
	returnPath string     // returnPath is the email address that the server received from the MAIL FROM command
	isTLS      bool       // isTLS is true when TLS was used to connect
}

type ChunkSaverChunk struct {
	modifiedAt     time.Time
	referenceCount uint // referenceCount counts how many emails reference this chunk
	data           io.Reader
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

type chunkSaverMemory struct {
	chunks        map[HashKey]*chunkSaverMemoryChunk
	emails        []*chunkSaverMemoryEmail
	nextID        uint64
	IDOffset      uint64
	compressLevel int
}

// OpenMessage implements the ChunkSaverStorage interface
func (m *chunkSaverMemory) OpenMessage(from string, helo string, recipient string, ipAddress net.IPAddr, returnPath string, isTLS bool) (mailID uint64, err error) {
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
func (m *chunkSaverMemory) CloseMessage(mailID uint64, size int64, partsInfo *PartsInfo, subject string, deliveryID string, to string, from string) error {
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
func (m *chunkSaverMemory) AddChunk(data []byte, hash []byte) error {
	var key HashKey
	if len(hash) != hashByteSize {
		return errors.New("invalid hash")
	}
	key.Pack(hash)
	var compressed bytes.Buffer
	zlibw, err := zlib.NewWriterLevel(&compressed, m.compressLevel)
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
func (m *chunkSaverMemory) Initialize(cfg BackendConfig) error {
	m.IDOffset = 1
	m.nextID = m.IDOffset
	m.emails = make([]*chunkSaverMemoryEmail, 0, 100)
	m.chunks = make(map[HashKey]*chunkSaverMemoryChunk, 1000)
	m.compressLevel = zlib.NoCompression
	return nil
}

// Shutdown implements the ChunkSaverStorage interface
func (m *chunkSaverMemory) Shutdown() (err error) {
	m.emails = nil
	m.chunks = nil
	return nil
}

// GetEmail implements the ChunkSaverStorage interface
func (m *chunkSaverMemory) GetEmail(mailID uint64) (*ChunkSaverEmail, error) {
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
func (m *chunkSaverMemory) GetChunks(hash ...HashKey) ([]*ChunkSaverChunk, error) {
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

type chunkSaverSQLConfig struct {
	EmailTable  string `json:"chunksaver_email_table,omitempty"`
	ChunkTable  string `json:"chunksaver_chunk_table,omitempty"`
	Driver      string `json:"chunksaver_sql_driver,omitempty"`
	DSN         string `json:"chunksaver_sql_dsn,omitempty"`
	PrimaryHost string `json:"chunksaver_primary_mail_host,omitempty"`
}

// chunkSaverSQL implements the ChunkSaverStorage interface
type chunkSaverSQL struct {
	config     *chunkSaverSQLConfig
	statements map[string]*sql.Stmt
	db         *sql.DB
}

func (c *chunkSaverSQL) connect() (*sql.DB, error) {
	var err error
	if c.db, err = sql.Open(c.config.Driver, c.config.DSN); err != nil {
		Log().Error("cannot open database: ", err)
		return nil, err
	}
	// do we have permission to access the table?
	_, err = c.db.Query("SELECT mail_id FROM " + c.config.EmailTable + " LIMIT 1")
	if err != nil {
		return nil, err
	}
	return c.db, err
}

func (c *chunkSaverSQL) prepareSql() error {
	if c.statements == nil {
		c.statements = make(map[string]*sql.Stmt)
	}

	if stmt, err := c.db.Prepare(`INSERT INTO ` +
		c.config.EmailTable +
		` (from, helo, recipient, ipv4_addr, ipv6_addr, return_path, is_tls) 
 VALUES(?, ?, ?, ?, ?, ?, ?)`); err != nil {
		return err
	} else {
		c.statements["insertEmail"] = stmt
	}

	// begin inserting an email (before saving chunks)
	if stmt, err := c.db.Prepare(`INSERT INTO ` +
		c.config.ChunkTable +
		` (data, hash) 
 VALUES(?, ?)`); err != nil {
		return err
	} else {
		c.statements["insertChunk"] = stmt
	}

	// finalize the email (the connection closed)
	if stmt, err := c.db.Prepare(`
		UPDATE ` + c.config.EmailTable + ` 
			SET size=?, parts_info = ?, subject, delivery_id = ?, to = ? 
		WHERE mail_id = ? `); err != nil {
		return err
	} else {
		c.statements["finalizeEmail"] = stmt
	}

	// Check the existence of a chunk (the reference_count col is incremented if it exists)
	// This means we can avoid re-inserting an existing chunk, only update its reference_count
	if stmt, err := c.db.Prepare(`
		UPDATE ` + c.config.ChunkTable + ` 
			SET reference_count=reference_count+1 
		WHERE hash = ? `); err != nil {
		return err
	} else {
		c.statements["chunkReferenceIncr"] = stmt
	}

	// If the reference_count is 0 then it means the chunk has been deleted
	// Chunks are soft-deleted for now, hard-deleted by another sweeper query as they become stale.
	if stmt, err := c.db.Prepare(`
		UPDATE ` + c.config.ChunkTable + ` 
			SET reference_count=reference_count-1 
		WHERE hash = ? AND reference_count > 0`); err != nil {
		return err
	} else {
		c.statements["chunkReferenceDecr"] = stmt
	}

	// fetch an email
	if stmt, err := c.db.Prepare(`
		SELECT * 
		from ` + c.config.EmailTable + ` 
		where mail_id=?`); err != nil {
		return err
	} else {
		c.statements["selectMail"] = stmt
	}

	// fetch a chunk
	if stmt, err := c.db.Prepare(`
		SELECT * 
		from ` + c.config.ChunkTable + ` 
		where hash=?`); err != nil {
		return err
	} else {
		c.statements["selectChunk"] = stmt
	}

	// TODO sweep old chunks

	// TODO sweep incomplete emails

	return nil
}

// OpenMessage implements the ChunkSaverStorage interface
func (c *chunkSaverSQL) OpenMessage(from string, helo string, recipient string, ipAddress net.IPAddr, returnPath string, isTLS bool) (mailID uint64, err error) {

	// if it's ipv4 then we want ipv6 to be 0, and vice-versa
	var ip4 uint32
	ip6 := make([]byte, 16)
	if ip := ipAddress.IP.To4(); ip != nil {
		ip4 = binary.BigEndian.Uint32(ip)
	} else {
		_ = copy(ip6, ipAddress.IP)
	}
	r, err := c.statements["insertEmail"].Exec(from, helo, recipient, ip4, ip6, returnPath, isTLS)
	if err != nil {
		return 0, err
	}
	id, err := r.LastInsertId()
	if err != nil {
		return 0, err
	}
	return uint64(id), err
}

// AddChunk implements the ChunkSaverStorage interface
func (c *chunkSaverSQL) AddChunk(data []byte, hash []byte) error {
	// attempt to increment the reference_count (it means the chunk is already in there)
	r, err := c.statements["chunkReferenceIncr"].Exec(hash)
	if err != nil {
		return err
	}
	affected, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		// chunk isn't in there, let's insert it
		_, err := c.statements["insertChunk"].Exec(data, hash)
		if err != nil {
			return err
		}
	}
	return nil
}

// CloseMessage implements the ChunkSaverStorage interface
func (c *chunkSaverSQL) CloseMessage(mailID uint64, size int64, partsInfo *PartsInfo, subject string, deliveryID string, to string, from string) error {
	partsInfoJson, err := json.Marshal(partsInfo)
	if err != nil {
		return err
	}
	_, err = c.statements["finalizeEmail"].Exec(size, partsInfoJson, subject, deliveryID, to, mailID)
	if err != nil {
		return err
	}
	return nil
}

// Initialize loads the specific database config, connects to the db, prepares statements
func (c *chunkSaverSQL) Initialize(cfg BackendConfig) error {
	configType := BaseConfig(&chunkSaverSQLConfig{})
	bcfg, err := Svc.ExtractConfig(cfg, configType)
	if err != nil {
		return err
	}
	c.config = bcfg.(*chunkSaverSQLConfig)
	c.db, err = c.connect()
	if err != nil {
		return err
	}
	err = c.prepareSql()
	if err != nil {
		return err
	}
	return nil
}

// Shutdown implements the ChunkSaverStorage interface
func (c *chunkSaverSQL) Shutdown() (err error) {
	defer func() {
		closeErr := c.db.Close()
		if closeErr != err {
			Log().WithError(err).Error("failed to close sql database")
			err = closeErr
		}
	}()
	for i := range c.statements {
		if err = c.statements[i].Close(); err != nil {
			Log().WithError(err).Error("failed to close sql statement")
		}
	}
	return err
}

// GetEmail implements the ChunkSaverStorage interface
func (c *chunkSaverSQL) GetEmail(mailID uint64) (*ChunkSaverEmail, error) {
	return &ChunkSaverEmail{}, nil
}

// GetChunk implements the ChunkSaverStorage interface
func (c *chunkSaverSQL) GetChunks(hash ...HashKey) ([]*ChunkSaverChunk, error) {
	result := make([]*ChunkSaverChunk, 0, len(hash))
	return result, nil
}

type chunkMailReader struct {
	db    ChunkSaverStorage
	email *ChunkSaverEmail
	// part requests a part. If 0, all the parts are read sequentially
	part int
	i, j int

	cache cachedChunks
}

// NewChunkMailReader loads the email and selects which mime-part Read will read, starting from 1
// if part is 0, Read will read in the entire message. 1 selects the first part, 2 2nd, and so on..
func NewChunkMailReader(db ChunkSaverStorage, email *ChunkSaverEmail, part int) (*chunkMailReader, error) {
	r := new(chunkMailReader)
	r.db = db
	r.part = part
	if email == nil {
		return nil, errors.New("nil email")
	} else {
		r.email = email
	}
	if err := r.SeekPart(part); err != nil {
		return nil, err
	}
	r.cache = cachedChunks{
		db: db,
	}
	return r, nil
}

// SeekPart resets the reader. The part argument chooses which part Read will read in
// If part is 0, Read will return the entire message
func (r *chunkMailReader) SeekPart(part int) error {
	if parts := len(r.email.partsInfo.Parts); parts == 0 {
		return errors.New("email has mime parts missing")
	} else if part > parts {
		return errors.New("no such part available")
	}
	r.i = part
	r.j = 0
	return nil
}

type cachedChunks struct {
	chunks    []*ChunkSaverChunk
	hashIndex map[int]HashKey
	db        ChunkSaverStorage
}

const chunkCachePreload = 2

// warm allocates the chunk cache, and gets the first few and stores them in the cache
func (c *cachedChunks) warm(hashes ...HashKey) (int, error) {

	if c.hashIndex == nil {
		c.hashIndex = make(map[int]HashKey, len(hashes))
	}
	if c.chunks == nil {
		c.chunks = make([]*ChunkSaverChunk, 0, 100)
	}
	if len(c.chunks) > 0 {
		// already been filled
		return len(c.chunks), nil
	}
	// let's pre-load some hashes.
	preload := chunkCachePreload
	if len(hashes) < preload {
		preload = len(hashes)
	}
	if chunks, err := c.db.GetChunks(hashes[0:preload]...); err != nil {
		return 0, err
	} else {
		for i := range hashes {
			c.hashIndex[i] = hashes[i]
			if i < preload {
				c.chunks = append(c.chunks, chunks[i])
			} else {
				// don't pre-load
				c.chunks = append(c.chunks, nil) // nil will be a placeholder for our chunk
			}
		}
	}
	return len(c.chunks), nil
}

// get returns a chunk. If the chunk doesn't exist, it gets it and pre-loads the next few
// also removes the previous chunks that now have become stale
func (c *cachedChunks) get(i int) (*ChunkSaverChunk, error) {
	if i > len(c.chunks) {
		return nil, errors.New("not enough chunks")
	}
	if c.chunks[i] != nil {
		// cache hit!
		return c.chunks[i], nil
	} else {
		var toGet []HashKey
		if key, ok := c.hashIndex[i]; ok {
			toGet = append(toGet, key)
		} else {
			return nil, errors.New(fmt.Sprintf("hash for key [%s] not found", key))
		}
		// make a list of chunks to load (extra ones to be pre-loaded)
		for to := i + 1; to < len(c.chunks) || to > chunkCachePreload+i; to++ {
			if key, ok := c.hashIndex[to]; ok {
				toGet = append(toGet, key)
			}
		}
		if chunks, err := c.db.GetChunks(toGet...); err != nil {
			return nil, err
		} else {
			// cache the pre-loaded chunks
			for j := i; j < len(c.chunks); j++ {
				c.chunks[j] = chunks[j-i]
				c.hashIndex[j] = toGet[j-i]
			}
			// remove any old ones (walk back)
			for j := i; j > -1; j-- {
				if c.chunks[j] != nil {
					c.chunks[j] = nil
				} else {
					break
				}
			}
			// return the chunk asked for
			return chunks[0], nil
		}
	}

}

func (c *cachedChunks) empty() {
	for i := range c.chunks {
		c.chunks[i] = nil
	}
	c.chunks = c.chunks[:] // set len to 0
	for key := range c.hashIndex {
		delete(c.hashIndex, key)
	}
}

// Read implements the io.Reader interface
func (r *chunkMailReader) Read(p []byte) (n int, err error) {
	var length int
	for ; r.i < len(r.email.partsInfo.Parts); r.i++ {
		length, err = r.cache.warm(r.email.partsInfo.Parts[r.i].ChunkHash...)
		if err != nil {
			return
		}
		var nRead int
		for r.j < length {
			chunk, err := r.cache.get(r.j)
			if err != nil {
				return nRead, err
			}
			nRead, err = chunk.data.Read(p)
			if err == io.EOF {
				r.j++ // advance to the next chunk
				err = nil
			}
			if r.j == length { // last chunk in a part?
				r.j = 0 // reset chunk index
				r.i++   // advance to the next part
				if r.i == len(r.email.partsInfo.Parts) || r.part > 0 {
					// there are no more parts to return
					err = io.EOF
					r.cache.empty()
				}
			}
			// unless there's an error, the next time this function will be
			// called, it will read the next chunk
			return nRead, err
		}
	}
	err = io.EOF
	return n, err
}

type transportEncoding int

const (
	encodingTypeBase64 transportEncoding = iota
	encodingTypeQP
)

// chunkPartDecoder decodes base64 and q-printable, then converting charset to utf8-8
type chunkPartDecoder struct {
	*chunkMailReader
	buf     []byte
	state   int
	charset string

	r io.Reader
}

// example
// db ChunkSaverStorage, email *ChunkSaverEmail, part int)
/*

r, err := NewChunkMailReader(db, email, part)
	if err != nil {
		return nil, err
	}

*/

// NewChunkPartDecoder reads from an underlying reader r and decodes base64, quoted-printable and decodes
func NewChunkPartDecoder(r io.Reader, enc transportEncoding, charset string) (*chunkPartDecoder, error) {

	decoder := new(chunkPartDecoder)
	decoder.r = r
	return decoder, nil
}

const chunkSaverNL = '\n'

const (
	decoderStateFindHeader int = iota
	decoderStateMatchNL
	decoderStateDecode
)

func (r *chunkPartDecoder) Read(p []byte) (n int, err error) {
	var part *ChunkedPart
	//if cap(p) != cap(r.buf) {
	r.buf = make([]byte, len(p), cap(p))
	var start, buffered int
	part = &r.email.partsInfo.Parts[r.part]
	_ = part
	buffered, err = r.chunkMailReader.Read(r.buf)
	if buffered == 0 {
		return
	}
	for {
		switch r.state {
		case decoderStateFindHeader:
			// finding the start of the header
			if start = bytes.Index(r.buf, []byte{chunkSaverNL, chunkSaverNL}); start != -1 {
				start += 2                   // skip the \n\n
				r.state = decoderStateDecode // found the header
				continue                     // continue scanning
			} else if r.buf[len(r.buf)-1] == chunkSaverNL {
				// the last char is a \n so next call to Read will check if it starts with a matching \n
				r.state = decoderStateMatchNL
			}
		case decoderStateMatchNL:
			if r.buf[0] == '\n' {
				// found the header
				start = 1
				r.state = decoderStateDecode
				continue
			} else {
				r.state = decoderStateFindHeader
				continue
			}

		case decoderStateDecode:
			if start < len(r.buf) {
				// todo decode here (q-printable, base64, charset)
				n += copy(p[:], r.buf[start:buffered])
			}
			return
		}

		buffered, err = r.chunkMailReader.Read(r.buf)
		if buffered == 0 {
			return
		}
	}

}

const chunkMaxBytes = 1024 * 16 // 16Kb is the default, change using chunksaver_chunk_size config setting
/**
*
 * A chunk ends ether:
 * after xKB or after end of a part, or end of header
 *
 * - buffer first chunk
 * - if didn't receive first chunk for more than x bytes, save normally
 *
*/
func Chunksaver() *StreamDecorator {

	sd := &StreamDecorator{}
	sd.Decorate =
		func(sp StreamProcessor, a ...interface{}) StreamProcessor {
			var (
				envelope    *mail.Envelope
				chunkBuffer *chunkedBytesBufferMime
				msgPos      uint
				database    ChunkSaverStorage
				written     int64

				// just some headers from the first mime-part
				subject string
				to      string
				from    string

				progress int // tracks which mime parts were processed
			)

			var config *chunkSaverConfig
			// optional dependency injection
			for i := range a {
				if db, ok := a[i].(ChunkSaverStorage); ok {
					database = db
				}
				if buff, ok := a[i].(*chunkedBytesBufferMime); ok {
					chunkBuffer = buff
				}
			}

			Svc.AddInitializer(InitializeWith(func(backendConfig BackendConfig) error {

				configType := BaseConfig(&chunkSaverConfig{})
				bcfg, err := Svc.ExtractConfig(backendConfig, configType)
				if err != nil {
					return err
				}
				config = bcfg.(*chunkSaverConfig)
				if chunkBuffer == nil {
					chunkBuffer = newChunkedBytesBufferMime()
				}
				// configure storage if none was injected
				if database == nil {
					if config.StorageEngine == "memory" {
						db := new(chunkSaverMemory)
						db.compressLevel = config.CompressLevel
						database = db
					} else {
						db := new(chunkSaverSQL)
						database = db
					}
				}
				err = database.Initialize(backendConfig)
				if err != nil {
					return err
				}
				// configure the chunks buffer
				if config.ChunkMaxBytes > 0 {
					chunkBuffer.capTo(config.ChunkMaxBytes)
				} else {
					chunkBuffer.capTo(chunkMaxBytes)
				}
				chunkBuffer.setDatabase(database)

				return nil
			}))

			Svc.AddShutdowner(ShutdownWith(func() error {
				err := database.Shutdown()
				return err
			}))

			sd.Open = func(e *mail.Envelope) error {
				// create a new entry & grab the id
				written = 0
				progress = 0
				var ip net.IPAddr
				if ret := net.ParseIP(e.RemoteIP); ret != nil {
					ip = net.IPAddr{IP: ret}
				}
				mid, err := database.OpenMessage(
					e.MailFrom.String(),
					e.Helo,
					e.RcptTo[0].String(),
					ip,
					e.MailFrom.String(),
					e.TLS)
				if err != nil {
					return err
				}
				e.Values["messageID"] = mid
				envelope = e
				return nil
			}

			sd.Close = func() (err error) {
				err = chunkBuffer.flush()
				if err != nil {
					// TODO we could delete the half saved message here
					return err
				}
				defer chunkBuffer.Reset()
				if mid, ok := envelope.Values["messageID"].(uint64); ok {
					err = database.CloseMessage(
						mid,
						written,
						&chunkBuffer.info,
						subject,
						envelope.QueuedId,
						to,
						from,
					)
					if err != nil {
						return err
					}
				}
				return nil
			}

			fillVars := func(parts *[]*mime.Part, subject, to, from string) (string, string, string) {
				if len(*parts) > 0 {
					if subject == "" {
						if val, ok := (*parts)[0].Headers["Subject"]; ok {
							subject = val[0]
						}
					}
					if to == "" {
						if val, ok := (*parts)[0].Headers["To"]; ok {
							addr, err := mail.NewAddress(val[0])
							if err == nil {
								to = addr.String()
							}
						}
					}
					if from == "" {
						if val, ok := (*parts)[0].Headers["From"]; ok {
							addr, err := mail.NewAddress(val[0])
							if err == nil {
								from = addr.String()
							}
						}
					}

				}
				return subject, to, from
			}

			return StreamProcessWith(func(p []byte) (count int, err error) {
				if envelope.Values == nil {
					return count, errors.New("no message headers found")
				}
				if parts, ok := envelope.Values["MimeParts"].(*[]*mime.Part); ok && len(*parts) > 0 {
					var pos int

					subject, to, from = fillVars(parts, subject, to, from)
					offset := msgPos
					chunkBuffer.currentPart((*parts)[0])
					for i := progress; i < len(*parts); i++ {
						part := (*parts)[i]

						// break chunk on new part
						if part.StartingPos > 0 && part.StartingPos > msgPos {
							count, _ = chunkBuffer.Write(p[pos : part.StartingPos-offset])
							written += int64(count)

							err = chunkBuffer.flush()
							if err != nil {
								return count, err
							}
							chunkBuffer.currentPart(part)
							fmt.Println("->N")
							pos += count
							msgPos = part.StartingPos
						}
						// break chunk on header
						if part.StartingPosBody > 0 && part.StartingPosBody >= msgPos {
							count, _ = chunkBuffer.Write(p[pos : part.StartingPosBody-offset])
							written += int64(count)

							err = chunkBuffer.flush()
							if err != nil {
								return count, err
							}
							chunkBuffer.currentPart(part)
							fmt.Println("->H")
							pos += count
							msgPos = part.StartingPosBody
						}
						// if on the latest (last) part, and yet there is still data to be written out
						if len(*parts)-1 == i && len(p)-1 > pos {
							count, _ = chunkBuffer.Write(p[pos:])
							written += int64(count)
							pos += count
							msgPos += uint(count)
						}
						// if there's no more data
						if pos >= len(p) {
							break
						}
					}
					if len(*parts) > 2 {
						progress = len(*parts) - 2 // skip to 2nd last part, assume previous parts are already processed
					}
				}
				return sp.Write(p)
			})
		}
	return sd
}
