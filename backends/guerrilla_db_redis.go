package backends

import (
	"errors"
	"fmt"

	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"

	"bytes"
	"compress/zlib"
	"github.com/flashmob/go-guerrilla/envelope"
	"github.com/ziutek/mymysql/autorc"
	_ "github.com/ziutek/mymysql/godrv"
	"io"
	"sync"
)

func init() {
	backends["guerrilla-db-redis"] = &AbstractBackend{
		extend: &GuerrillaDBAndRedisBackend{}}
}

type GuerrillaDBAndRedisBackend struct {
	AbstractBackend
	config guerrillaDBAndRedisConfig
}

type guerrillaDBAndRedisConfig struct {
	NumberOfWorkers    int    `json:"save_workers_size"`
	MysqlTable         string `json:"mail_table"`
	MysqlDB            string `json:"mysql_db"`
	MysqlHost          string `json:"mysql_host"`
	MysqlPass          string `json:"mysql_pass"`
	MysqlUser          string `json:"mysql_user"`
	RedisExpireSeconds int    `json:"redis_expire_seconds"`
	RedisInterface     string `json:"redis_interface"`
	PrimaryHost        string `json:"primary_mail_host"`
}

func convertError(name string) error {
	return fmt.Errorf("failed to load backend config (%s)", name)
}

// Load the backend config for the backend. It has already been unmarshalled
// from the main config file 'backend' config "backend_config"
// Now we need to convert each type and copy into the guerrillaDBAndRedisConfig struct
func (g *GuerrillaDBAndRedisBackend) loadConfig(backendConfig BackendConfig) (err error) {
	configType := baseConfig(&guerrillaDBAndRedisConfig{})
	bcfg, err := g.extractConfig(backendConfig, configType)
	if err != nil {
		return err
	}
	m := bcfg.(*guerrillaDBAndRedisConfig)
	g.config = *m
	return nil
}

func (g *GuerrillaDBAndRedisBackend) getNumberOfWorkers() int {
	return g.config.NumberOfWorkers
}

func (g *GuerrillaDBAndRedisBackend) Process(mail *envelope.Envelope) BackendResult {
	to := mail.RcptTo
	log.Info("(g *GuerrillaDBAndRedisBackend) Process called")
	if len(to) == 0 {
		return NewBackendResult("554 Error: no recipient")
	}
	return nil
}

type redisClient struct {
	isConnected bool
	conn        redis.Conn
	time        int
}

// compressedData struct will be compressed using zlib when printed via fmt
type compressedData struct {
	extraHeaders []byte
	data         *bytes.Buffer
	pool         sync.Pool
}

// newCompressedData returns a new CompressedData
func newCompressedData() *compressedData {
	var p = sync.Pool{
		New: func() interface{} {
			var b bytes.Buffer
			return &b
		},
	}
	return &compressedData{
		pool: p,
	}
}

// Set the extraheaders and buffer of data to compress
func (c *compressedData) set(b []byte, d *bytes.Buffer) {
	c.extraHeaders = b
	c.data = d
}

// implement Stringer interface
func (c *compressedData) String() string {
	if c.data == nil {
		return ""
	}
	//borrow a buffer form the pool
	b := c.pool.Get().(*bytes.Buffer)
	// put back in the pool
	defer func() {
		b.Reset()
		c.pool.Put(b)
	}()

	var r *bytes.Reader
	w, _ := zlib.NewWriterLevel(b, zlib.BestSpeed)
	r = bytes.NewReader(c.extraHeaders)
	io.Copy(w, r)
	io.Copy(w, c.data)
	w.Close()
	return b.String()
}

// clear it, without clearing the pool
func (c *compressedData) clear() {
	c.extraHeaders = []byte{}
	c.data = nil
}

func (g *GuerrillaDBAndRedisBackend) saveMailWorker(saveMailChan chan *savePayload) {
	var to, body string
	var err error

	var redisErr error
	var length int
	redisClient := &redisClient{}
	db := autorc.New(
		"tcp",
		"",
		g.config.MysqlHost,
		g.config.MysqlUser,
		g.config.MysqlPass,
		g.config.MysqlDB)
	db.Register("set names utf8")
	sql := "INSERT INTO " + g.config.MysqlTable + " "
	sql += "(`date`, `to`, `from`, `subject`, `body`, `charset`, `mail`, `spam_score`, `hash`, `content_type`, `recipient`, `has_attach`, `ip_addr`, `return_path`, `is_tls`)"
	sql += " values (NOW(), ?, ?, ?, ? , 'UTF-8' , ?, 0, ?, '', ?, 0, ?, ?, ?)"
	ins, sqlErr := db.Prepare(sql)
	if sqlErr != nil {
		log.WithError(sqlErr).Fatalf("failed while db.Prepare(INSERT...)")
	}
	sql = "UPDATE gm2_setting SET `setting_value` = `setting_value`+1 WHERE `setting_name`='received_emails' LIMIT 1"
	incr, sqlErr := db.Prepare(sql)
	if sqlErr != nil {
		log.WithError(sqlErr).Fatalf("failed while db.Prepare(UPDATE...)")
	}
	defer func() {
		if r := recover(); r != nil {
			//recover form closed channel
			fmt.Println("Recovered in f", r)
		}
		if db.Raw != nil {
			db.Raw.Close()
		}
		if redisClient.conn != nil {
			log.Infof("closed redis")
			redisClient.conn.Close()
		}
	}()

	data := newCompressedData()
	//  receives values from the channel repeatedly until it is closed.
	for {
		payload := <-saveMailChan
		if payload == nil {
			log.Debug("No more saveMailChan payload")
			return
		}
		to = payload.recipient.User + "@" + g.config.PrimaryHost
		length = payload.mail.Data.Len()

		ts := fmt.Sprintf("%d", time.Now().UnixNano())
		payload.mail.ParseHeaders()
		hash := MD5Hex(
			to,
			payload.mail.MailFrom.String(),
			payload.mail.Subject,
			ts)
		// Add extra headers
		var addHead string
		addHead += "Delivered-To: " + to + "\r\n"
		addHead += "Received: from " + payload.mail.Helo + " (" + payload.mail.Helo + "  [" + payload.mail.RemoteAddress + "])\r\n"
		addHead += "	by " + payload.recipient.Host + " with SMTP id " + hash + "@" + payload.recipient.Host + ";\r\n"
		addHead += "	" + time.Now().Format(time.RFC1123Z) + "\r\n"

		// data will be compressed when printed, with addHead added to beginning

		data.set([]byte(addHead), &payload.mail.Data)
		body = "gzencode"

		// data will be written to redis - it implements the Stringer interface, redigo uses fmt to
		// print the data to redis.

		redisErr = redisClient.redisConnection(g.config.RedisInterface)
		if redisErr == nil {
			_, doErr := redisClient.conn.Do("SETEX", hash, g.config.RedisExpireSeconds, data)
			if doErr == nil {
				//payload.mail.Data = ""
				//payload.mail.Data.Reset()
				body = "redis" // the backend system will know to look in redis for the message data
				data.clear()   // blank
			}
		} else {
			log.WithError(redisErr).Warn("Error while SETEX on redis")
		}

		// bind data to cursor
		ins.Bind(
			to,
			payload.mail.MailFrom.String(),
			payload.mail.Subject,
			body,
			data.String(),
			hash,
			to,
			payload.mail.RemoteAddress,
			payload.mail.MailFrom.String(),
			payload.mail.TLS,
		)
		// save, discard result
		_, _, err = ins.Exec()
		if err != nil {
			errMsg := "Database error while inserting"
			log.WithError(err).Warn(errMsg)
			payload.savedNotify <- &saveStatus{errors.New(errMsg), hash}
		} else {
			log.Debugf("Email saved %s (len=%d)", hash, length)
			_, _, err = incr.Exec()
			if err != nil {
				log.WithError(err).Warn("Database error while incr count")
			}
			payload.savedNotify <- &saveStatus{nil, hash}
		}
	}
}

func (c *redisClient) redisConnection(redisInterface string) (err error) {
	if c.isConnected == false {
		c.conn, err = redis.Dial("tcp", redisInterface)
		if err != nil {
			// handle error
			return err
		}
		c.isConnected = true
	}
	return nil
}

// test database connection settings
func (g *GuerrillaDBAndRedisBackend) testSettings() (err error) {
	db := autorc.New(
		"tcp",
		"",
		g.config.MysqlHost,
		g.config.MysqlUser,
		g.config.MysqlPass,
		g.config.MysqlDB)

	if mysqlErr := db.Raw.Connect(); mysqlErr != nil {
		err = fmt.Errorf("MySql cannot connect, check your settings: %s", mysqlErr)
	} else {
		db.Raw.Close()
	}

	redisClient := &redisClient{}
	if redisErr := redisClient.redisConnection(g.config.RedisInterface); redisErr != nil {
		err = fmt.Errorf("Redis cannot connect, check your settings: %s", redisErr)
	}

	return
}
