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
)

type chunkSaverConfig struct {
	// ChunkMaxBytes controls the maximum buffer size for saving
	// 16KB default. The smallest possible size is 64 bytes to to bytes.Buffer limitation
	ChunkMaxBytes int `json:"chunksaver_chunk_size"`
}

func init() {
	streamers["chunksaver"] = func() *StreamDecorator {
		return Chunksaver()
	}
}

type partsInfo struct {
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

type flushEvent func()

type chunkedBytesBuffer struct {
	buf          []byte
	flushTrigger flushEvent
}

// flush signals that it's time to write the buffer out to disk
func (c *chunkedBytesBuffer) flush() {
	if len(c.buf) == 0 {
		return
	}
	fmt.Print(string(c.buf))
	if c.flushTrigger != nil {
		c.flushTrigger()
	}
	c.Reset()
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
			c.flush()
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
	current *mime.Part
	info    partsInfo
	md5     hash.Hash
}

func newChunkedBytesBufferMime() *chunkedBytesBufferMime {
	b := new(chunkedBytesBufferMime)
	var chash [16]byte
	b.chunkedBytesBuffer.flushTrigger = func() {
		b.md5.Write(b.buf)
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
			// TODO : send chunk to db
			// db.savechunk(
		}
	}
	b.md5 = md5.New()
	return b
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
		b.info = partsInfo{Parts: make([]chunkedPart, 0, 3), TextPart: -1, HTMLPart: -1}
	}
	b.current = cp

}

type ChunkSaverStorage interface {
	OpenMessage(
		from []byte,
		helo []byte,
		recipient []byte,
		ipAddress net.IPAddr,
		returnPath []byte,
		isTLS bool) (mailID uint64, err error)
	CloseMessage(
		mailID uint64,
		size uint,
		partsInfo *partsInfo,
		subject []byte,
		charset []byte,
		deliveryID []byte,
		to []byte) error
	AddChunk(data []byte, hash []byte) error
	Initialize(cfg BackendConfig) error
	Shutdown() (err error)
}

type chunkSaverSQLConfig struct {
	EmailTable  string `json:"email_table"`
	ChunkTable  string `json:"chunk_table"`
	Driver      string `json:"sql_driver"`
	DSN         string `json:"sql_dsn"`
	PrimaryHost string `json:"primary_mail_host"`
}

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
			SET size=?, parts_info = ?, subject, charset = ?, delivery_id = ?, to = ? 
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

func (c *chunkSaverSQL) OpenMessage(
	from []byte,
	helo []byte,
	recipient []byte,
	ipAddress net.IPAddr,
	returnPath []byte,
	isTLS bool) (mailID uint64, err error) {

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

func (c *chunkSaverSQL) CloseMessage(
	mailID uint64,
	size uint,
	partsInfo *partsInfo,
	subject []byte,
	charset []byte,
	deliveryID []byte,
	to []byte) error {
	partsInfoJson, err := json.Marshal(partsInfo)
	if err != nil {
		return err
	}
	_, err = c.statements["finalizeEmail"].Exec(size, partsInfoJson, subject, charset, deliveryID, to, mailID)
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
	sd.p =

		func(sp StreamProcessor) StreamProcessor {
			var (
				envelope    *mail.Envelope
				chunkBuffer *chunkedBytesBufferMime
				msgPos      uint
				database    ChunkSaverStorage
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
				database = new(chunkSaverSQL)
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
				envelope = e
				return nil
			}

			sd.Close = func() error {
				chunkBuffer.flush()
				chunkBuffer.Reset()
				return nil
			}

			return StreamProcessWith(func(p []byte) (int, error) {

				if envelope.Values == nil {
					return 0, errors.New("no message headers found")
				}

				if parts, ok := envelope.Values["MimeParts"].(*[]*mime.Part); ok {
					var (
						pos      int
						progress int
					)
					if len(*parts) > 2 {
						progress = len(*parts) - 2 // skip to 2nd last part, assume previous parts are already out
					}

					offset := msgPos
					for i := progress; i < len(*parts); i++ {
						part := (*parts)[i]
						chunkBuffer.currentPart(part)

						// break chunk on new part
						if part.StartingPos > 0 && part.StartingPos > msgPos {
							count, _ := chunkBuffer.Write(p[pos : part.StartingPos-offset])

							chunkBuffer.flush()
							fmt.Println("->N")
							pos += count
							msgPos = part.StartingPos
						}

						// break chunk on header
						if part.StartingPosBody > 0 && part.StartingPosBody >= msgPos {
							count, _ := chunkBuffer.Write(p[pos : part.StartingPosBody-offset])

							chunkBuffer.flush()
							fmt.Println("->H")
							pos += count
							msgPos = part.StartingPosBody
						}

						// if on the latest (last) part, and yet there is still data to be written out
						if len(*parts)-1 == i && len(p)-1 > pos {
							count, _ := chunkBuffer.Write(p[pos:])
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
