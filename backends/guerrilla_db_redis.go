package backends

import (
	"fmt"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/garyburd/redigo/redis"
	"github.com/ziutek/mymysql/autorc"
	_ "github.com/ziutek/mymysql/godrv"

	guerrilla "github.com/flashmob/go-guerrilla"
	"github.com/flashmob/go-guerrilla/util"
	"reflect"
	"strings"
)

func init() {
	backends["guerrilla-db-redis"] = &GuerrillaDBAndRedisBackend{}
}

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
func (g *GuerrillaDBAndRedisBackend) loadConfig(backendConfig guerrilla.BackendConfig) error {
	// Use reflection so that we can provide a nice error message
	g.config = guerrillaDBAndRedisConfig{}
	s := reflect.ValueOf(&g.config).Elem() // so that we can set the values
	typeOfT := s.Type()
	types := reflect.TypeOf(g.config)
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		// read the tags of the config struct
		field_name := types.Field(i).Tag.Get("json")
		if len(field_name) > 0 {
			// parse the tag to
			// get the field name from struct tag
			split := strings.Split(field_name, ",")
			field_name = split[0]
		} else {
			// could have no tag
			// so use the reflected field name
			field_name = typeOfT.Field(i).Name
		}
		if f.Type().Name() == "int" {
			if intVal, converted := backendConfig[field_name].(float64); converted {
				s.Field(i).SetInt(int64(intVal))
			} else {
				return convertError("property missing/invalid: '" + field_name + "' of expected type: " + f.Type().Name())
			}
		}
		if f.Type().Name() == "string" {
			if stringVal, converted := backendConfig[field_name].(string); converted {
				s.Field(i).SetString(stringVal)
			} else {
				return convertError("missing/invalid: '" + field_name + "' of type: " + f.Type().Name())
			}
		}
	}
	return nil
}

func (g *GuerrillaDBAndRedisBackend) Initialize(backendConfig guerrilla.BackendConfig) error {
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

func (g *GuerrillaDBAndRedisBackend) Finalize() error {
	close(g.saveMailChan)
	g.wg.Wait()
	return nil
}

func (g *GuerrillaDBAndRedisBackend) Process(client *guerrilla.Client, from *guerrilla.EmailParts, to []*guerrilla.EmailParts) string {
	if len(to) == 0 {
		return "554 Error: no recipient"
	}

	// to do: timeout when adding to SaveMailChan
	// place on the channel so that one of the save mail workers can pick it up
	// TODO: support multiple recipients
	g.saveMailChan <- &savePayload{client: client, from: from, recipient: to[0]}
	// wait for the save to complete
	// or timeout
	select {
	case status := <-client.SavedNotify:
		if status == 1 {
			return fmt.Sprintf("250 OK : queued as %s", client.Hash)
		}
		return "554 Error: transaction failed, blame it on the weather"
	case <-time.After(time.Second * 30):
		log.Debug("timeout")
		return "554 Error: transaction timeout"
	}
}

type savePayload struct {
	client    *guerrilla.Client
	from      *guerrilla.EmailParts
	recipient *guerrilla.EmailParts
}

type redisClient struct {
	count int
	conn  redis.Conn
	time  int
}

func (g *GuerrillaDBAndRedisBackend) saveMail() {
	var to,  body string
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

	//  receives values from the channel repeatedly until it is closed.
	for {
		payload := <-g.saveMailChan
		if payload == nil {
			log.Debug("No more payload")
			g.wg.Done()
			return
		}
		to = payload.recipient.User + "@" + g.config.PrimaryHost
		length = len(payload.client.Data)
		ts := fmt.Sprintf("%d", time.Now().UnixNano())
		payload.client.Subject = util.MimeHeaderDecode(payload.client.Subject)
		payload.client.Hash = util.MD5Hex(
			&to,
			&payload.client.MailFrom,
			&payload.client.Subject,
			&ts)
		// Add extra headers
		var addHead string
		addHead += "Delivered-To: " + to + "\r\n"
		addHead += "Received: from " + payload.client.Helo + " (" + payload.client.Helo + "  [" + payload.client.Address + "])\r\n"
		addHead += "	by " + payload.recipient.Host + " with SMTP id " + payload.client.Hash + "@" + payload.recipient.Host + ";\r\n"
		addHead += "	" + time.Now().Format(time.RFC1123Z) + "\r\n"
		// compress to save space
		payload.client.Data = util.Compress(&addHead, &payload.client.Data)
		body = "gzencode"
		redisErr = redisClient.redisConnection(g.config.RedisInterface)
		if redisErr == nil {
			_, doErr := redisClient.conn.Do("SETEX", payload.client.Hash, g.config.RedisExpireSeconds, payload.client.Data)
			if doErr == nil {
				payload.client.Data = ""
				body = "redis"
			}
		} else {
			log.WithError(redisErr).Warn("Error while SETEX on redis")
		}
		// bind data to cursor
		ins.Bind(
			to,
			payload.client.MailFrom,
			payload.client.Subject,
			body,
			payload.client.Data,
			payload.client.Hash,
			to,
			payload.client.Address,
			payload.client.MailFrom,
			payload.client.TLS,
		)
		// save, discard result
		_, _, err = ins.Exec()
		if err != nil {
			log.WithError(err).Warn("Database error while inster")
			payload.client.SavedNotify <- -1
		} else {
			log.Debugf("Email saved %s (len=%d)", payload.client.Hash, length)
			_, _, err = incr.Exec()
			if err != nil {
				log.WithError(err).Warn("Database error while incr count")
			}
			payload.client.SavedNotify <- 1
		}
	}
}

func (c *redisClient) redisConnection(redisInterface string) (err error) {
	if c.count == 0 {
		c.conn, err = redis.Dial("tcp", redisInterface)
		if err != nil {
			// handle error
			return err
		}
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
