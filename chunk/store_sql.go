package chunk

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"github.com/flashmob/go-guerrilla/backends"
	"net"
)

type chunkSaverSQLConfig struct {
	EmailTable  string `json:"chunksaver_email_table,omitempty"`
	ChunkTable  string `json:"chunksaver_chunk_table,omitempty"`
	Driver      string `json:"chunksaver_sql_driver,omitempty"`
	DSN         string `json:"chunksaver_sql_dsn,omitempty"`
	PrimaryHost string `json:"chunksaver_primary_mail_host,omitempty"`
}

// ChunkSaverSQL implements the ChunkSaverStorage interface
type ChunkSaverSQL struct {
	config     *chunkSaverSQLConfig
	statements map[string]*sql.Stmt
	db         *sql.DB
}

func (c *ChunkSaverSQL) connect() (*sql.DB, error) {
	var err error
	if c.db, err = sql.Open(c.config.Driver, c.config.DSN); err != nil {
		backends.Log().Error("cannot open database: ", err)
		return nil, err
	}
	// do we have permission to access the table?
	_, err = c.db.Query("SELECT mail_id FROM " + c.config.EmailTable + " LIMIT 1")
	if err != nil {
		return nil, err
	}
	return c.db, err
}

func (c *ChunkSaverSQL) prepareSql() error {
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
func (c *ChunkSaverSQL) OpenMessage(from string, helo string, recipient string, ipAddress net.IPAddr, returnPath string, isTLS bool) (mailID uint64, err error) {

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
func (c *ChunkSaverSQL) AddChunk(data []byte, hash []byte) error {
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
func (c *ChunkSaverSQL) CloseMessage(mailID uint64, size int64, partsInfo *PartsInfo, subject string, deliveryID string, to string, from string) error {
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
func (c *ChunkSaverSQL) Initialize(cfg backends.BackendConfig) error {
	configType := backends.BaseConfig(&chunkSaverSQLConfig{})
	bcfg, err := backends.Svc.ExtractConfig(cfg, configType)
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
func (c *ChunkSaverSQL) Shutdown() (err error) {
	defer func() {
		closeErr := c.db.Close()
		if closeErr != err {
			backends.Log().WithError(err).Error("failed to close sql database")
			err = closeErr
		}
	}()
	for i := range c.statements {
		if err = c.statements[i].Close(); err != nil {
			backends.Log().WithError(err).Error("failed to close sql statement")
		}
	}
	return err
}

// GetEmail implements the ChunkSaverStorage interface
func (c *ChunkSaverSQL) GetEmail(mailID uint64) (*ChunkSaverEmail, error) {
	return &ChunkSaverEmail{}, nil
}

// GetChunk implements the ChunkSaverStorage interface
func (c *ChunkSaverSQL) GetChunks(hash ...HashKey) ([]*ChunkSaverChunk, error) {
	result := make([]*ChunkSaverChunk, 0, len(hash))
	return result, nil
}
