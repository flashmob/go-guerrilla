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
	"crypto/md5"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/mime"
	"hash"
	"net"
	"strings"
	"time"
)

type chunkSaverConfig struct {
	// ChunkMaxBytes controls the maximum buffer size for saving
	// 16KB default.
	ChunkMaxBytes int    `json:"chunksaver_chunk_size"`
	StorageEngine string `json:"chunksaver_storage_engine"`
}

func init() {
	streamers["chunksaver"] = func() *StreamDecorator {
		return Chunksaver()
	}
}

type PartsInfo struct {
	Count     uint32        `json:"c"`  // number of parts
	TextPart  int           `json:"tp"` // id of the main text part to display
	HTMLPart  int           `json:"hp"` // id of the main html part to display (if any)
	HasAttach bool          `json:"a"`
	Parts     []chunkedPart `json:"p"`
}

type chunkedPart struct {
	PartId             string     `json:"i"`
	ChunkHash          [][16]byte `json:"h"` // sequence of hashes the data is stored at
	ContentType        string     `json:"t"`
	Charset            string     `json:"c"`
	TransferEncoding   string     `json:"e"`
	ContentDisposition string     `json:"d"`
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

func (b *chunkedBytesBufferMime) onFlush() error {
	b.md5.Write(b.buf)
	var chash [16]byte
	copy(chash[:], b.md5.Sum([]byte{}))
	if b.current != nil {
		if size := len(b.info.Parts); size > 0 && b.info.Parts[size-1].PartId == b.current.Node {
			// existing part, just append the hash
			lastPart := &b.info.Parts[size-1]
			lastPart.ChunkHash = append(lastPart.ChunkHash, chash)
			b.fillInfo(lastPart, size-1)
		} else {
			// add it as a new part
			part := chunkedPart{
				PartId:    b.current.Node,
				ChunkHash: [][16]byte{chash},
			}
			b.fillInfo(&part, 0)
			b.info.Parts = append(b.info.Parts, part)
			b.info.Count++
		}
		if err := b.database.AddChunk(b.buf, chash[:]); err != nil {
			return err
		}
	}
	return nil
}

func (b *chunkedBytesBufferMime) fillInfo(cp *chunkedPart, index int) {
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

func (b *chunkedBytesBufferMime) Reset() {
	b.md5.Reset()
	b.chunkedBytesBuffer.Reset()
}

func (b *chunkedBytesBufferMime) currentPart(cp *mime.Part) {
	if b.current == nil {
		b.info = PartsInfo{Parts: make([]chunkedPart, 0, 3), TextPart: -1, HTMLPart: -1}
	}
	b.current = cp

}

// ChunkSaverStorage defines an interface to the storage layer (the database)
type ChunkSaverStorage interface {
	OpenMessage(from string, helo string, recipient string, ipAddress net.IPAddr, returnPath string, isTLS bool) (mailID uint64, err error)
	CloseMessage(mailID uint64, size int64, partsInfo *PartsInfo, subject string, deliveryID string, to string, from string) error
	AddChunk(data []byte, hash []byte) error
	GetEmail(mailID uint64) (*ChunkSaverEmail, error)
	GetChunks(hash ...[]byte) ([]*ChunkSaverChunk, error)
	Initialize(cfg BackendConfig) error
	Shutdown() (err error)
}

type ChunkSaverEmail struct {
	mailID     uint64
	createdAt  time.Time
	size       int64
	from       string
	to         string
	partsInfo  PartsInfo
	helo       string
	subject    string
	deliveryID string
	recipient  string
	ipv4       net.IPAddr
	ipv6       net.IPAddr
	returnPath string
	isTLS      bool
}

type ChunkSaverChunk struct {
	modifiedAt     time.Time
	referenceCount uint
	data           []byte
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
	chunks   map[[16]byte]*chunkSaverMemoryChunk
	emails   []*chunkSaverMemoryEmail
	nextID   uint64
	IDOffset uint64
}

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

func (m *chunkSaverMemory) CloseMessage(mailID uint64, size int64, partsInfo *PartsInfo, subject string, deliveryID string, to string, from string) error {
	if email := m.emails[mailID-m.IDOffset]; email == nil {
		return errors.New("email not found")
	} else {
		email.size = size
		if info, err := json.Marshal(partsInfo); err != nil {
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

func (m *chunkSaverMemory) AddChunk(data []byte, hash []byte) error {
	var key [16]byte
	if len(hash) != 16 {
		return errors.New("invalid hash")
	}
	copy(key[:], hash[0:16])
	if chunk, ok := m.chunks[key]; ok {
		// only update the counters and update time
		chunk.referenceCount++
		chunk.modifiedAt = time.Now()
	} else {
		// add a new chunk
		newChunk := chunkSaverMemoryChunk{
			modifiedAt:     time.Now(),
			referenceCount: 1,
			//	data:           data,
		}
		newChunk.data = make([]byte, len(data))
		copy(newChunk.data, data)
		m.chunks[key] = &newChunk
	}
	return nil
}

func (m *chunkSaverMemory) Initialize(cfg BackendConfig) error {
	m.IDOffset = 1
	m.nextID = m.IDOffset
	m.emails = make([]*chunkSaverMemoryEmail, 0, 100)
	m.chunks = make(map[[16]byte]*chunkSaverMemoryChunk, 1000)
	return nil
}

func (m *chunkSaverMemory) Shutdown() (err error) {
	m.emails = nil
	m.chunks = nil
	return nil
}

func (m *chunkSaverMemory) GetEmail(mailID uint64) (*ChunkSaverEmail, error) {
	if size := uint64(len(m.emails)) - m.IDOffset; size > mailID-m.IDOffset {
		return nil, errors.New("mail not found")
	}
	email := m.emails[mailID-m.IDOffset]
	pi := &PartsInfo{}
	if err := json.Unmarshal(email.partsInfo, pi); err != nil {
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

func (m *chunkSaverMemory) GetChunks(hash ...[]byte) ([]*ChunkSaverChunk, error) {
	result := make([]*ChunkSaverChunk, 0, len(hash))
	var key [16]byte
	for i := range hash {
		copy(key[:], hash[i][:16])
		if c, ok := m.chunks[key]; ok {
			result = append(result, &ChunkSaverChunk{
				modifiedAt:     c.modifiedAt,
				referenceCount: c.referenceCount,
				data:           c.data,
			})
		}
	}
	return result, nil
}

type chunkSaverSQLConfig struct {
	EmailTable  string `json:"email_table"`
	ChunkTable  string `json:"chunk_table"`
	Driver      string `json:"sql_driver"`
	DSN         string `json:"sql_dsn"`
	PrimaryHost string `json:"primary_mail_host"`
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

	// sweep old chunks

	// sweep incomplete emails

	return nil
}

func (c *chunkSaverSQL) OpenMessage(from string, helo string, recipient string, ipAddress net.IPAddr, returnPath string, isTLS bool) (mailID uint64, err error) {

	// if it's ipv4 then we want ipv6 to be 0, and vice-versa
	var ip4 uint32
	ip6 := make([]byte, 16)
	if ip := ipAddress.IP.To4(); ip != nil {
		ip4 = binary.BigEndian.Uint32(ip)
	} else {
		_ = copy(ip6, []byte(ipAddress.IP))
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

func (m *chunkSaverSQL) GetEmail(mailID uint64) (*ChunkSaverEmail, error) {
	return &ChunkSaverEmail{}, nil
}
func (m *chunkSaverSQL) GetChunks(hash ...[]byte) ([]*ChunkSaverChunk, error) {
	result := make([]*ChunkSaverChunk, 0, len(hash))
	return result, nil
}

type chunkMailReader struct {
	db ChunkSaverStorage
}

func (r *chunkMailReader) Info(mailID uint64) (*PartsInfo, error) {
	return &PartsInfo{}, nil
}

func (r *chunkMailReader) Read(p []byte) (int, error) {
	return 1, nil
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
	sd.P =

		func(sp StreamProcessor) StreamProcessor {
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
			)

			var config *chunkSaverConfig

			Svc.AddInitializer(InitializeWith(func(backendConfig BackendConfig) error {
				chunkBuffer = newChunkedBytesBufferMime()
				configType := BaseConfig(&chunkSaverConfig{})
				bcfg, err := Svc.ExtractConfig(backendConfig, configType)
				if err != nil {
					return err
				}
				config = bcfg.(*chunkSaverConfig)
				if config.ChunkMaxBytes > 0 {
					chunkBuffer.capTo(config.ChunkMaxBytes)
				} else {
					chunkBuffer.capTo(chunkMaxBytes)
				}
				if config.StorageEngine == "memory" {
					db := new(chunkSaverMemory)
					chunkBuffer.setDatabase(db)
					database = db
				} else {
					db := new(chunkSaverSQL)
					chunkBuffer.setDatabase(db)
					database = db
				}

				err = database.Initialize(backendConfig)
				if err != nil {
					return err
				}
				return nil
			}))

			Svc.AddShutdowner(ShutdownWith(func() error {
				err := database.Shutdown()
				return err
			}))

			sd.Open = func(e *mail.Envelope) error {
				// create a new entry & grab the id
				written = 0
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
				if parts, ok := envelope.Values["MimeParts"].(*[]*mime.Part); ok {
					var (
						pos      int
						progress int
					)
					if len(*parts) > 2 {
						progress = len(*parts) - 2 // skip to 2nd last part, assume previous parts are already out
					}
					subject, to, from = fillVars(parts, subject, to, from)
					offset := msgPos
					for i := progress; i < len(*parts); i++ {
						part := (*parts)[i]

						chunkBuffer.currentPart(part)
						// break chunk on new part
						if part.StartingPos > 0 && part.StartingPos > msgPos {
							count, _ = chunkBuffer.Write(p[pos : part.StartingPos-offset])
							written += int64(count)
							err = chunkBuffer.flush()
							if err != nil {
								return count, err
							}
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
				}
				return sp.Write(p)
			})
		}

	return sd
}
