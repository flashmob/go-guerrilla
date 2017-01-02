package backends

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/flashmob/go-guerrilla"
	"github.com/garyburd/redigo/redis"

	"github.com/ziutek/mymysql/autorc"
	_ "github.com/ziutek/mymysql/godrv"
)

type GuerrillaDBAndRedisBackend struct {
	config       guerrillaDBAndRedisConfig
	saveMailChan chan *savePayload
	wg           sync.WaitGroup
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
func (g *GuerrillaDBAndRedisBackend) loadConfig(backendConfig map[string]interface{}) error {
	data, err := json.Marshal(backendConfig)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, &g.config)
	if g.config.NumberOfWorkers < 1 {
		return errors.New("Must have more than 1 worker")
	}

	return err
}

func (g *GuerrillaDBAndRedisBackend) Initialize(backendConfig map[string]interface{}) error {
	err := g.loadConfig(backendConfig)
	if err != nil {
		return err
	}

	if err := g.testDbConnections(); err != nil {
		return err
	}

	g.saveMailChan = make(chan *savePayload, g.config.NumberOfWorkers)

	// start some savemail workers
	g.wg.Add(g.config.NumberOfWorkers)
	for i := 0; i < g.config.NumberOfWorkers; i++ {
		go g.saveMail()
	}

	return nil
}

func (g *GuerrillaDBAndRedisBackend) Shutdown() error {
	close(g.saveMailChan) // workers will stop
	g.wg.Wait()
	return nil
}

func (g *GuerrillaDBAndRedisBackend) Process(mail *guerrilla.Envelope) guerrilla.BackendResult {
	to := mail.RcptTo
	from := mail.MailFrom
	if len(to) == 0 {
		return guerrilla.NewBackendResult("554 Error: no recipient")
	}

	// to do: timeout when adding to SaveMailChan
	// place on the channel so that one of the save mail workers can pick it up
	// TODO: support multiple recipients
	savedNotify := make(chan *saveStatus)
	g.saveMailChan <- &savePayload{mail, from, &to[0], savedNotify}
	// wait for the save to complete
	// or timeout
	select {
	case status := <-savedNotify:
		if status.err != nil {
			return guerrilla.NewBackendResult("554 Error: " + status.err.Error())
		}
		return guerrilla.NewBackendResult(fmt.Sprintf("250 OK : queued as %s", status.hash))
	case <-time.After(time.Second * 30):
		log.Debug("timeout")
		return guerrilla.NewBackendResult("554 Error: transaction timeout")
	}
}

type savePayload struct {
	mail        *guerrilla.Envelope
	from        *guerrilla.EmailAddress
	recipient   *guerrilla.EmailAddress
	savedNotify chan *saveStatus
}

type saveStatus struct {
	err  error
	hash string
}

type redisClient struct {
	isConnected bool
	conn        redis.Conn
	time        int
}

func (g *GuerrillaDBAndRedisBackend) saveMail() {
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
			// recover form closed channel
			fmt.Println("Recovered in f", r)
		}
		if db.Raw != nil {
			db.Raw.Close()
		}
		if redisClient.conn != nil {
			log.Infof("closed redis")
			redisClient.conn.Close()
		}

		g.wg.Done()
	}()

	//  receives values from the channel repeatedly until it is closed.
	for {
		payload := <-g.saveMailChan
		if payload == nil {
			log.Debug("No more saveMailChan payload")
			return
		}
		to = payload.recipient.User + "@" + g.config.PrimaryHost
		length = len(payload.mail.Data)
		ts := fmt.Sprintf("%d", time.Now().UnixNano())
		payload.mail.Subject = MimeHeaderDecode(payload.mail.Subject)
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
		// compress to save space
		payload.mail.Data = Compress(addHead, payload.mail.Data)
		body = "gzencode"
		redisErr = redisClient.redisConnection(g.config.RedisInterface)
		if redisErr == nil {
			_, doErr := redisClient.conn.Do("SETEX", hash, g.config.RedisExpireSeconds, payload.mail.Data)
			if doErr == nil {
				payload.mail.Data = ""
				body = "redis"
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
			payload.mail.Data,
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
func (g *GuerrillaDBAndRedisBackend) testDbConnections() (err error) {
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
