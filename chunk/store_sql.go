package chunk

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/smtp"
	"github.com/go-sql-driver/mysql"
	"net"
	"strings"
	"time"
)

/*

SQL schema

```

create schema gmail collate utf8mb4_unicode_ci;

CREATE TABLE `in_emails` (
     `mail_id` bigint unsigned NOT NULL AUTO_INCREMENT,
     `created_at` datetime NOT NULL,
     `size` int unsigned NOT NULL,
     `from` varbinary(255) NOT NULL,
     `to` varbinary(255) NOT NULL,
     `parts_info` text COLLATE utf8mb4_unicode_ci,
     `helo` varchar(255) COLLATE latin1_swedish_ci NOT NULL,
     `subject` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
     `queued_id` binary(16) NOT NULL,
     `recipient` varbinary(255) NOT NULL,
     `ipv4_addr` int unsigned DEFAULT NULL,
     `ipv6_addr` varbinary(16) DEFAULT NULL,
     `return_path` varbinary(255) NOT NULL,
     `protocol` set('SMTP','SMTPS','ESMTP','ESMTPS','LMTP','LMTPS') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'SMTP',
     `transport` set('7bit','8bit','unknown','invalid') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'unknown',
     PRIMARY KEY (`mail_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `in_emails_chunks` (
    `modified_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `reference_count` int unsigned DEFAULT '1',
    `data` mediumblob NOT NULL,
    `hash` varbinary(16) NOT NULL,
    UNIQUE KEY `in_emails_chunks_hash_uindex` (`hash`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;


```

ipv6_addr is big endian

TODO compression, configurable SQL strings, logger

*/
func init() {
	StorageEngines["sql"] = func() Storage {
		return new(StoreSQL)
	}
}

type sqlConfig struct {

	// EmailTable is the name of the main database table for the headers
	EmailTable string `json:"email_table,omitempty"`
	// EmailChunkTable stores the data of the emails in de-duplicated chunks
	EmailChunkTable string `json:"email_table_chunks,omitempty"`

	// Connection settings
	// Driver to use, eg "mysql"
	Driver string `json:"sql_driver,omitempty"`
	// DSN (required) is the connection string, eg.
	// "user:passt@tcp(127.0.0.1:3306)/db_name?readTimeout=10s&writeTimeout=10s&charset=utf8mb4&collation=utf8mb4_unicode_ci"
	DSN string `json:"sql_dsn,omitempty"`
	// MaxConnLifetime (optional) is a duration, eg. "30s"
	MaxConnLifetime string `json:"sql_max_conn_lifetime,omitempty"`
	// MaxOpenConns (optional) specifies the number of maximum open connections
	MaxOpenConns int `json:"sql_max_open_conns,omitempty"`
	// MaxIdleConns
	MaxIdleConns int `json:"sql_max_idle_conns,omitempty"`

	// CompressLevel controls the gzip compression level of email chunks.
	// 0 = no compression, 1 == best speed, 9 == best compression, -1 == default, -2 == huffman only
	CompressLevel int `json:"compress_level,omitempty"`
}

// StoreSQL implements the Storage interface
type StoreSQL struct {
	config sqlConfig
	db     *sql.DB

	sqlSelectChunk        []*sql.Stmt
	sqlInsertEmail        *sql.Stmt
	sqlInsertChunk        *sql.Stmt
	sqlFinalizeEmail      *sql.Stmt
	sqlChunkReferenceIncr *sql.Stmt
	sqlChunkReferenceDecr *sql.Stmt
	sqlSelectMail         *sql.Stmt
}

func (s *StoreSQL) StartWorker() (stop chan bool) {

	timeo := time.Second * 1
	stop = make(chan bool)
	go func() {
		select {

		case <-stop:
			return

		case <-time.After(timeo):
			t1 := int64(time.Now().UnixNano())
			// do stuff here

			if (time.Now().UnixNano())-t1 > int64(time.Second*3) {

			}

		}
	}()
	return stop

}

func (s *StoreSQL) connect() (*sql.DB, error) {
	var err error
	if s.db, err = sql.Open(s.config.Driver, s.config.DSN); err != nil {
		backends.Log().Error("cannot open database: ", err)
		return nil, err
	}
	if s.config.MaxOpenConns != 0 {
		s.db.SetMaxOpenConns(s.config.MaxOpenConns)
	}
	if s.config.MaxIdleConns != 0 {
		s.db.SetMaxIdleConns(s.config.MaxIdleConns)
	}
	if s.config.MaxConnLifetime != "" {
		t, err := time.ParseDuration(s.config.MaxConnLifetime)
		if err != nil {
			return nil, err
		}
		s.db.SetConnMaxLifetime(t)
	}
	// do we have permission to access the table?
	_, err = s.db.Query("SELECT mail_id FROM " + s.config.EmailTable + " LIMIT 1")
	if err != nil {
		return nil, err
	}
	return s.db, err
}

func (s *StoreSQL) prepareSql() error {

	// begin inserting an email (before saving chunks)
	if stmt, err := s.db.Prepare(`INSERT INTO ` +
		s.config.EmailTable +
		` (queued_id, created_at, ` + "`from`" + `, helo, recipient, ipv4_addr, ipv6_addr, return_path, transport, protocol)
 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`); err != nil {
		return err
	} else {
		s.sqlInsertEmail = stmt
	}

	// insert a chunk of email's data
	if stmt, err := s.db.Prepare(`INSERT INTO ` +
		s.config.EmailChunkTable +
		` (data, hash) 
 VALUES(?, ?)`); err != nil {
		return err
	} else {
		s.sqlInsertChunk = stmt
	}

	// finalize the email (the connection closed)
	if stmt, err := s.db.Prepare(`
		UPDATE ` + s.config.EmailTable + ` 
			SET size=?, parts_info=?, subject=?, ` + "`to`" + `=?, ` + "`from`" + `=?
		WHERE mail_id = ? `); err != nil {
		return err
	} else {
		s.sqlFinalizeEmail = stmt
	}

	// Check the existence of a chunk (the reference_count col is incremented if it exists)
	// This means we can avoid re-inserting an existing chunk, only update its reference_count
	// check the "affected rows" count after executing query
	if stmt, err := s.db.Prepare(`
		UPDATE ` + s.config.EmailChunkTable + ` 
			SET reference_count=reference_count+1 
		WHERE hash = ? `); err != nil {
		return err
	} else {
		s.sqlChunkReferenceIncr = stmt
	}

	// If the reference_count is 0 then it means the chunk has been deleted
	// Chunks are soft-deleted for now, hard-deleted by another sweeper query as they become stale.
	if stmt, err := s.db.Prepare(`
		UPDATE ` + s.config.EmailChunkTable + ` 
			SET reference_count=reference_count-1 
		WHERE hash = ? AND reference_count > 0`); err != nil {
		return err
	} else {
		s.sqlChunkReferenceDecr = stmt
	}

	// fetch an email
	if stmt, err := s.db.Prepare(`
		SELECT * 
		from ` + s.config.EmailTable + ` 
		where mail_id=?`); err != nil {
		return err
	} else {
		s.sqlSelectMail = stmt
	}

	// fetch a chunk, used in GetChunks
	// prepare a query for all possible combinations is prepared

	for i := 0; i < chunkPrefetchMax; i++ {
		if stmt, err := s.db.Prepare(
			s.getChunksSQL(i + 1),
		); err != nil {
			return err
		} else {
			s.sqlSelectChunk[i] = stmt
		}
	}

	// TODO sweep old chunks

	// TODO sweep incomplete emails

	return nil
}

const mysqlYYYY_m_d_s_H_i_s = "2006-01-02 15:04:05"

// OpenMessage implements the Storage interface
func (s *StoreSQL) OpenMessage(
	queuedID mail.Hash128,
	from string,
	helo string,
	recipient string,
	ipAddress IPAddr,
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
		copy(ip6, ipAddress.IP)
	}
	r, err := s.sqlInsertEmail.Exec(
		queuedID.Bytes(),
		time.Now().Format(mysqlYYYY_m_d_s_H_i_s),
		from,
		helo,
		recipient,
		ip4,
		ip6,
		returnPath,
		transport.String(),
		protocol.String())
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
	r, err := s.sqlChunkReferenceIncr.Exec(hash)
	if err != nil {
		return err
	}
	affected, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		// chunk isn't in there, let's insert it
		_, err := s.sqlInsertChunk.Exec(data, hash)
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
	to string, from string) error {
	partsInfoJson, err := json.Marshal(partsInfo)
	if err != nil {
		return err
	}
	_, err = s.sqlFinalizeEmail.Exec(size, partsInfoJson, subject, to, from, mailID)
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
	if s.config.EmailTable == "" {
		s.config.EmailTable = "in_emails"
	}
	if s.config.EmailChunkTable == "" {
		s.config.EmailChunkTable = "in_emails_chunks"
	}
	if s.config.Driver == "" {
		s.config.Driver = "mysql"
	}
	// because it uses an IN(?) query, so we need a different query for each possible ? combination (max chunkPrefetchMax)
	s.sqlSelectChunk = make([]*sql.Stmt, chunkPrefetchMax)

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
	toClose := []*sql.Stmt{
		s.sqlInsertEmail,
		s.sqlFinalizeEmail,
		s.sqlInsertChunk,
		s.sqlChunkReferenceIncr,
		s.sqlChunkReferenceDecr,
		s.sqlSelectMail,
	}
	toClose = append(toClose, s.sqlSelectChunk...)

	for i := range toClose {
		if err = toClose[i].Close(); err != nil {
			backends.Log().WithError(err).Error("failed to close sql statement")
		}
	}
	return err
}

// GetEmail implements the Storage interface
func (s *StoreSQL) GetMessage(mailID uint64) (*Email, error) {

	email := &Email{}
	var createdAt mysql.NullTime
	var transport transportType
	var protocol protocol
	err := s.sqlSelectMail.QueryRow(mailID).Scan(
		&email.mailID,
		&createdAt,
		&email.size,
		&email.from,
		&email.to,
		&email.partsInfo,
		&email.helo,
		&email.subject,
		&email.queuedID,
		&email.recipient,
		&email.ipv4,
		&email.ipv6,
		&email.returnPath,
		&protocol,
		&transport,
	)
	email.createdAt = createdAt.Time
	email.protocol = protocol.Protocol
	email.transport = transport.TransportType
	if err != nil {
		return email, err
	}
	return email, nil
}

// Value implements the driver.Valuer interface
func (h HashKey) Value() (driver.Value, error) {
	return h[:], nil
}

func (h *HashKey) Scan(value interface{}) error {
	b := value.([]uint8)
	h.Pack(b)
	return nil
}

type chunkData []uint8

func (v chunkData) Value() (driver.Value, error) {
	return v[:], nil
}

func (s *StoreSQL) getChunksSQL(size int) string {
	return fmt.Sprintf("SELECT modified_at, reference_count, data, `hash` FROM %s WHERE `hash` in (%s)",
		s.config.EmailChunkTable,
		"?"+strings.Repeat(",?", size-1),
	)
}

// GetChunks implements the Storage interface
func (s *StoreSQL) GetChunks(hash ...HashKey) ([]*Chunk, error) {
	result := make([]*Chunk, len(hash))
	// we need to wrap these in an interface{} so that they can be passed to db.Query
	args := make([]interface{}, len(hash))
	for i := range hash {
		args[i] = &hash[i]
	}
	rows, err := s.sqlSelectChunk[len(args)-1].Query(args...)
	defer func() {
		if rows != nil {
			_ = rows.Close()
		}
	}()
	if err != nil {
		return result, err
	}
	// temp is a lookup table for hash -> chunk
	// since rows can come in different order, we need to make sure
	// that result is sorted in the order of args
	temp := make(map[HashKey]*Chunk, len(hash))
	i := 0
	for rows.Next() {
		var createdAt mysql.NullTime
		var data chunkData
		var h HashKey
		c := Chunk{}
		if err := rows.Scan(
			&createdAt,
			&c.referenceCount,
			&data,
			&h,
		); err != nil {
			return result, err
		}
		c.data = bytes.NewBuffer(data)
		c.modifiedAt = createdAt.Time
		temp[h] = &c
		i++
	}
	// re-order the rows according to the order of the args (very important)
	for i := range args {
		b := args[i].(*HashKey)
		if _, ok := temp[*b]; ok {
			result[i] = temp[*b]
		}
	}
	if err := rows.Err(); err != nil || i == 0 {
		return result, errors.New("data chunks not found")
	}
	return result, nil
}

// zap is used in testing, purges everything
func (s *StoreSQL) zap() error {
	if r, err := s.db.Exec("DELETE from " + s.config.EmailTable + " "); err != nil {
		return err
	} else {
		affected, _ := r.RowsAffected()
		fmt.Println(fmt.Sprintf("deleted %v emails", affected))
	}

	if r, err := s.db.Exec("DELETE from " + s.config.EmailChunkTable + " "); err != nil {
		return err
	} else {
		affected, _ := r.RowsAffected()
		fmt.Println(fmt.Sprintf("deleted %v chunks", affected))
	}

	return nil

}

// Scan implements database/sql scanner interface, for parsing PartsInfo
func (info *PartsInfo) Scan(value interface{}) error {
	if value == nil {
		return errors.New("parts_info is null")
	}
	if data, ok := value.([]byte); !ok {
		return errors.New("parts_info is not str")
	} else {
		if err := json.Unmarshal(data, info); err != nil {
			return err
		}
	}
	return nil
}

// /Scan implements database/sql scanner interface, for parsing net.IPAddr
func (ip *IPAddr) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	if data, ok := value.([]uint8); ok {
		if len(data) == 16 { // 128 bits
			// ipv6
			ipv6 := make(net.IP, 16)
			copy(ipv6, data)
			ip.IPAddr.IP = ipv6
		}
	}
	if data, ok := value.(int64); ok {
		// ipv4
		ipv4 := make(net.IP, 4)
		binary.BigEndian.PutUint32(ipv4, uint32(data))
		ip.IPAddr.IP = ipv4
	}

	return nil
}

type transportType struct {
	smtp.TransportType
}

type protocol struct {
	mail.Protocol
}

// Scan implements database/sql scanner interface, for parsing smtp.TransportType
func (t *transportType) Scan(value interface{}) error {
	if data, ok := value.([]uint8); ok {
		v := smtp.ParseTransportType(string(data))
		t.TransportType = v
	}
	return nil
}

// Scan implements database/sql scanner interface, for parsing mail.Protocol
func (p *protocol) Scan(value interface{}) error {
	if data, ok := value.([]uint8); ok {
		v := mail.ParseProtocolType(string(data))
		p.Protocol = v
	}
	return nil
}
