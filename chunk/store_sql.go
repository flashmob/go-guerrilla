package chunk

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/smtp"
	"net"
)

func init() {
	StorageEngines["sql"] = func() Storage {
		return new(StoreSQL)
	}
}

type sqlConfig struct {
	EmailTable    string `json:"email_table,omitempty"`
	ChunkTable    string `json:"chunk_table,omitempty"`
	Driver        string `json:"sql_driver,omitempty"`
	DSN           string `json:"sql_dsn,omitempty"`
	PrimaryHost   string `json:"primary_mail_host,omitempty"`
	CompressLevel int    `json:"compress_level,omitempty"`
}

// StoreSQL implements the Storage interface
type StoreSQL struct {
	config     sqlConfig
	statements map[string]*sql.Stmt
	db         *sql.DB
}

func (s *StoreSQL) connect() (*sql.DB, error) {
	var err error
	if s.db, err = sql.Open(s.config.Driver, s.config.DSN); err != nil {
		backends.Log().Error("cannot open database: ", err)
		return nil, err
	}
	// do we have permission to access the table?
	_, err = s.db.Query("SELECT mail_id FROM " + s.config.EmailTable + " LIMIT 1")
	if err != nil {
		return nil, err
	}
	return s.db, err
}

func (s *StoreSQL) prepareSql() error {
	if s.statements == nil {
		s.statements = make(map[string]*sql.Stmt)
	}

	// begin inserting an email (before saving chunks)
	if stmt, err := s.db.Prepare(`INSERT INTO ` +
		s.config.EmailTable +
		` (from, helo, recipient, ipv4_addr, ipv6_addr, return_path, transport, protocol) 
 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`); err != nil {
		return err
	} else {
		s.statements["insertEmail"] = stmt
	}

	// insert a chunk of email's data
	if stmt, err := s.db.Prepare(`INSERT INTO ` +
		s.config.ChunkTable +
		` (data, hash) 
 VALUES(?, ?)`); err != nil {
		return err
	} else {
		s.statements["insertChunk"] = stmt
	}

	// finalize the email (the connection closed)
	if stmt, err := s.db.Prepare(`
		UPDATE ` + s.config.EmailTable + ` 
			SET size=?, parts_info = ?, subject, queued_id = ?, to = ? 
		WHERE mail_id = ? `); err != nil {
		return err
	} else {
		s.statements["finalizeEmail"] = stmt
	}

	// Check the existence of a chunk (the reference_count col is incremented if it exists)
	// This means we can avoid re-inserting an existing chunk, only update its reference_count
	// check the "affected rows" count after executing query
	if stmt, err := s.db.Prepare(`
		UPDATE ` + s.config.ChunkTable + ` 
			SET reference_count=reference_count+1 
		WHERE hash = ? `); err != nil {
		return err
	} else {
		s.statements["chunkReferenceIncr"] = stmt
	}

	// If the reference_count is 0 then it means the chunk has been deleted
	// Chunks are soft-deleted for now, hard-deleted by another sweeper query as they become stale.
	if stmt, err := s.db.Prepare(`
		UPDATE ` + s.config.ChunkTable + ` 
			SET reference_count=reference_count-1 
		WHERE hash = ? AND reference_count > 0`); err != nil {
		return err
	} else {
		s.statements["chunkReferenceDecr"] = stmt
	}

	// fetch an email
	if stmt, err := s.db.Prepare(`
		SELECT * 
		from ` + s.config.EmailTable + ` 
		where mail_id=?`); err != nil {
		return err
	} else {
		s.statements["selectMail"] = stmt
	}

	// fetch a chunk
	if stmt, err := s.db.Prepare(`
		SELECT * 
		from ` + s.config.ChunkTable + ` 
		where hash=?`); err != nil {
		return err
	} else {
		s.statements["selectChunk"] = stmt
	}

	// TODO sweep old chunks

	// TODO sweep incomplete emails

	return nil
}

// OpenMessage implements the Storage interface
func (s *StoreSQL) OpenMessage(
	from string,
	helo string,
	recipient string,
	ipAddress net.IPAddr,
	returnPath string,
	protocol mail.Protocol,
	transport smtp.TransportType,
) (mailID uint64, err error) {

	// if it's ipv4 then we want ipv6 to be 0, and vice-versa
	var ip4 uint32
	ip6 := make([]byte, 16)
	if ip := ipAddress.IP.To4(); ip != nil {
		ip4 = binary.BigEndian.Uint32(ip)
	} else {
		_ = copy(ip6, ipAddress.IP)
	}
	r, err := s.statements["insertEmail"].Exec(from, helo, recipient, ip4, ip6, returnPath, transport, protocol)
	if err != nil {
		return 0, err
	}
	id, err := r.LastInsertId()
	if err != nil {
		return 0, err
	}
	return uint64(id), err
}

// AddChunk implements the Storage interface
func (s *StoreSQL) AddChunk(data []byte, hash []byte) error {
	// attempt to increment the reference_count (it means the chunk is already in there)
	r, err := s.statements["chunkReferenceIncr"].Exec(hash)
	if err != nil {
		return err
	}
	affected, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		// chunk isn't in there, let's insert it
		_, err := s.statements["insertChunk"].Exec(data, hash)
		if err != nil {
			return err
		}
	}
	return nil
}

// CloseMessage implements the Storage interface
func (s *StoreSQL) CloseMessage(
	mailID uint64,
	size int64,
	partsInfo *PartsInfo,
	subject string,
	queuedID string,
	to string, from string) error {
	partsInfoJson, err := json.Marshal(partsInfo)
	if err != nil {
		return err
	}
	_, err = s.statements["finalizeEmail"].Exec(size, partsInfoJson, subject, queuedID, to, mailID)
	if err != nil {
		return err
	}
	return nil
}

// Initialize loads the specific database config, connects to the db, prepares statements
func (s *StoreSQL) Initialize(cfg backends.ConfigGroup) error {
	sd := backends.StreamDecorator{}
	err := sd.ExtractConfig(cfg, &s.config)
	if err != nil {
		return err
	}
	s.db, err = s.connect()
	if err != nil {
		return err
	}
	err = s.prepareSql()
	if err != nil {
		return err
	}
	return nil
}

// Shutdown implements the Storage interface
func (s *StoreSQL) Shutdown() (err error) {
	defer func() {
		closeErr := s.db.Close()
		if closeErr != err {
			backends.Log().WithError(err).Error("failed to close sql database")
			err = closeErr
		}
	}()
	for i := range s.statements {
		if err = s.statements[i].Close(); err != nil {
			backends.Log().WithError(err).Error("failed to close sql statement")
		}
	}
	return err
}

// GetEmail implements the Storage interface
func (s *StoreSQL) GetEmail(mailID uint64) (*Email, error) {
	return &Email{}, nil
}

// GetChunk implements the Storage interface
func (s *StoreSQL) GetChunks(hash ...HashKey) ([]*Chunk, error) {
	result := make([]*Chunk, 0, len(hash))
	return result, nil
}
