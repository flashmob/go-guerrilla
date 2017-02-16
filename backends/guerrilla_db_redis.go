package backends

// This backend is presented here as an example only, please modify it to your needs.
//
// Deprecated: as of 14th Feb 2017, backends are composed via config, by chaining Processors (files prefixed with p_*)
//
// The backend stores the email data in Redis.
// Other meta-information is stored in MySQL to be joined later.
// A lot of email gets discarded without viewing on Guerrilla Mail,
// so it's much faster to put in Redis, where other programs can
// process it later, without touching the disk.
//
// Some features:
// - It batches the SQL inserts into a single query and inserts either after a time threshold or if the batch is full
// - If the mysql driver crashes, it's able to recover, log the incident and resume again.
// - It also does a clean shutdown - it tries to save everything before returning
//
// Short history:
// Started with issuing an insert query for each single email and another query to update the tally
// Then applied the following optimizations:
// - Moved tally updates to another background process which does the tallying in a single query
// - Changed the MySQL queries to insert in batch
// - Made a Compressor that recycles buffers using sync.Pool
// The result was around 400% speed improvement. If you know of any more improvements, please share!
// - Added the recovery mechanism,

import (
	"fmt"

	"time"

	"github.com/garyburd/redigo/redis"

	"bytes"
	"compress/zlib"
	"database/sql"

	"github.com/flashmob/go-guerrilla/envelope"
	"github.com/go-sql-driver/mysql"
	"io"
	"runtime/debug"
	"strings"
	"sync"
)

// how many rows to batch at a time
const GuerrillaDBAndRedisBatchMax = 2

// tick on every...
const GuerrillaDBAndRedisBatchTimeout = time.Second * 3

func init() {
	backends["guerrilla-db-redis"] = &GuerrillaDBAndRedisBackend{}
}

type GuerrillaDBAndRedisBackend struct {
	config    guerrillaDBAndRedisConfig
	batcherWg sync.WaitGroup
	// cache prepared queries
	cache stmtCache
}

// statement cache. It's an array, not slice
type stmtCache [GuerrillaDBAndRedisBatchMax]*sql.Stmt

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
	configType := BaseConfig(&guerrillaDBAndRedisConfig{})
	bcfg, err := Service.ExtractConfig(backendConfig, configType)
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

type redisClient struct {
	isConnected bool
	conn        redis.Conn
	time        int
}

// compressedData struct will be compressed using zlib when printed via fmt
type compressedData struct {
	extraHeaders []byte
	data         *bytes.Buffer
	pool         *sync.Pool
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
		pool: &p,
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

// prepares the sql query with the number of rows that can be batched with it
func (g *GuerrillaDBAndRedisBackend) prepareInsertQuery(rows int, db *sql.DB) *sql.Stmt {
	if rows == 0 {
		panic("rows argument cannot be 0")
	}
	if g.cache[rows-1] != nil {
		return g.cache[rows-1]
	}
	sqlstr := "INSERT INTO " + g.config.MysqlTable + " "
	sqlstr += "(`date`, `to`, `from`, `subject`, `body`, `charset`, `mail`, `spam_score`, `hash`, `content_type`, `recipient`, `has_attach`, `ip_addr`, `return_path`, `is_tls`)"
	sqlstr += " values "
	values := "(NOW(), ?, ?, ?, ? , 'UTF-8' , ?, 0, ?, '', ?, 0, ?, ?, ?)"
	// add more rows
	comma := ""
	for i := 0; i < rows; i++ {
		sqlstr += comma + values
		if comma == "" {
			comma = ","
		}
	}
	stmt, sqlErr := db.Prepare(sqlstr)
	if sqlErr != nil {
		Log().WithError(sqlErr).Fatalf("failed while db.Prepare(INSERT...)")
	}
	// cache it
	g.cache[rows-1] = stmt
	return stmt
}

func (g *GuerrillaDBAndRedisBackend) doQuery(c int, db *sql.DB, insertStmt *sql.Stmt, vals *[]interface{}) {
	var execErr error
	defer func() {
		if r := recover(); r != nil {
			//logln(1, fmt.Sprintf("Recovered in %v", r))
			Log().Error("Recovered form panic:", r, string(debug.Stack()))
			sum := 0
			for _, v := range *vals {
				if str, ok := v.(string); ok {
					sum = sum + len(str)
				}
			}
			Log().Errorf("panic while inserting query [%s] size:%d, err %v", r, sum, execErr)
			panic("query failed")
		}
	}()
	// prepare the query used to insert when rows reaches batchMax
	insertStmt = g.prepareInsertQuery(c, db)
	_, execErr = insertStmt.Exec(*vals...)
	if execErr != nil {
		Log().WithError(execErr).Error("There was a problem the insert")
	}
}

// Batches the rows from the feeder chan in to a single INSERT statement.
// Execute the batches query when:
// - number of batched rows reaches a threshold, i.e. count n = threshold
// - or, no new rows within a certain time, i.e. times out
// The goroutine can either exit if there's a panic or feeder channel closes
// it returns feederOk which signals if the feeder chanel was ok (still open) while returning
// if it feederOk is false, then it means the feeder chanel is closed
func (g *GuerrillaDBAndRedisBackend) insertQueryBatcher(feeder chan []interface{}, db *sql.DB) (feederOk bool) {
	// controls shutdown
	defer g.batcherWg.Done()
	g.batcherWg.Add(1)
	// vals is where values are batched to
	var vals []interface{}
	// how many rows were batched
	count := 0
	// The timer will tick every second.
	// Interrupting the select clause when there's no data on the feeder channel
	t := time.NewTimer(GuerrillaDBAndRedisBatchTimeout)
	// prepare the query used to insert when rows reaches batchMax
	insertStmt := g.prepareInsertQuery(GuerrillaDBAndRedisBatchMax, db)
	// inserts executes a batched insert query, clears the vals and resets the count
	insert := func(c int) {
		if c > 0 {
			g.doQuery(c, db, insertStmt, &vals)
		}
		vals = nil
		count = 0
	}
	defer func() {
		if r := recover(); r != nil {
			Log().Error("insertQueryBatcher caught a panic", r)
		}
	}()
	// Keep getting values from feeder and add to batch.
	// if feeder times out, execute the batched query
	// otherwise, execute the batched query once it reaches the GuerrillaDBAndRedisBatchMax threshold
	feederOk = true
	for {
		select {
		// it may panic when reading on a closed feeder channel. feederOK detects if it was closed
		case row, feederOk := <-feeder:
			if row == nil {
				Log().Info("Query batchaer exiting")
				// Insert any remaining rows
				insert(count)
				return feederOk
			}
			vals = append(vals, row...)
			count++
			Log().Debug("new feeder row:", row, " cols:", len(row), " count:", count, " worker", workerId)
			if count >= GuerrillaDBAndRedisBatchMax {
				insert(GuerrillaDBAndRedisBatchMax)
			}
			// stop timer from firing (reset the interrupt)
			if !t.Stop() {
				<-t.C
			}
			t.Reset(GuerrillaDBAndRedisBatchTimeout)
		case <-t.C:
			// anything to insert?
			if n := len(vals); n > 0 {
				insert(count)
			}
			t.Reset(GuerrillaDBAndRedisBatchTimeout)
		}
	}
}

func trimToLimit(str string, limit int) string {
	ret := strings.TrimSpace(str)
	if len(str) > limit {
		ret = str[:limit]
	}
	return ret
}

var workerId = 0

func (g *GuerrillaDBAndRedisBackend) mysqlConnect() (*sql.DB, error) {
	conf := mysql.Config{
		User:         g.config.MysqlUser,
		Passwd:       g.config.MysqlPass,
		DBName:       g.config.MysqlDB,
		Net:          "tcp",
		Addr:         g.config.MysqlHost,
		ReadTimeout:  GuerrillaDBAndRedisBatchTimeout + (time.Second * 10),
		WriteTimeout: GuerrillaDBAndRedisBatchTimeout + (time.Second * 10),
		Params:       map[string]string{"collation": "utf8_general_ci"},
	}
	if db, err := sql.Open("mysql", conf.FormatDSN()); err != nil {
		Log().Error("cannot open mysql", err)
		return nil, err
	} else {
		return db, nil
	}

}

func (g *GuerrillaDBAndRedisBackend) saveMailWorker(saveMailChan chan *savePayload) {
	var to, body string

	var redisErr error

	workerId++

	redisClient := &redisClient{}
	var db *sql.DB
	var err error
	db, err = g.mysqlConnect()
	if err != nil {
		Log().Fatalf("cannot open mysql: %s", err)
	}

	// start the query SQL batching where we will send data via the feeder channel
	feeder := make(chan []interface{}, 1)
	go func() {
		for {
			if feederOK := g.insertQueryBatcher(feeder, db); !feederOK {
				Log().Debug("insertQueryBatcher exited")
				return
			}
			// if insertQueryBatcher panics, it can recover and go in again
			Log().Debug("resuming insertQueryBatcher")
		}

	}()

	defer func() {
		if r := recover(); r != nil {
			//recover form closed channel
			Log().Error("panic recovered in saveMailWorker", r)
		}
		db.Close()
		if redisClient.conn != nil {
			Log().Infof("closed redis")
			redisClient.conn.Close()
		}
		// close the feeder & wait for query batcher to exit.
		close(feeder)
		g.batcherWg.Wait()

	}()
	var vals []interface{}
	data := newCompressedData()
	//  receives values from the channel repeatedly until it is closed.

	for {
		payload := <-saveMailChan
		if payload == nil {
			Log().Debug("No more saveMailChan payload")
			return
		}
		Log().Debug("Got mail from chan", payload.mail.RemoteAddress)
		to = trimToLimit(strings.TrimSpace(payload.mail.RcptTo[0].User)+"@"+g.config.PrimaryHost, 255)
		payload.mail.Helo = trimToLimit(payload.mail.Helo, 255)
		host := trimToLimit(payload.mail.RcptTo[0].Host, 255)
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
		addHead += "	by " + host + " with SMTP id " + hash + "@" + host + ";\r\n"
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
				body = "redis" // the backend system will know to look in redis for the message data
				data.clear()   // blank
			}
		} else {
			Log().WithError(redisErr).Warn("Error while connecting redis")
		}

		vals = []interface{}{} // clear the vals
		vals = append(vals,
			trimToLimit(to, 255),
			trimToLimit(payload.mail.MailFrom.String(), 255),
			trimToLimit(payload.mail.Subject, 255),
			body,
			data.String(),
			hash,
			trimToLimit(to, 255),
			payload.mail.RemoteAddress,
			trimToLimit(payload.mail.MailFrom.String(), 255),
			payload.mail.TLS)
		feeder <- vals
		payload.savedNotify <- &saveStatus{nil, hash}

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

	var db *sql.DB

	if db, err = g.mysqlConnect(); err != nil {
		err = fmt.Errorf("MySql cannot connect, check your settings: %s", err)
	} else {
		db.Close()
	}

	redisClient := &redisClient{}
	if redisErr := redisClient.redisConnection(g.config.RedisInterface); redisErr != nil {
		err = fmt.Errorf("Redis cannot connect, check your settings: %s", redisErr)
	}

	return
}

func (g *GuerrillaDBAndRedisBackend) Initialize(config BackendConfig) error {
	err := g.loadConfig(config)
	if err != nil {
		return err
	}
	return nil

}

// does nothing
func (g *GuerrillaDBAndRedisBackend) Process(mail *envelope.Envelope) BackendResult {
	return NewBackendResult("250 OK")
}

// does nothing
func (g *GuerrillaDBAndRedisBackend) Shutdown() error {

	return nil
}
