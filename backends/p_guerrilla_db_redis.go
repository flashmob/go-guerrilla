package backends

import (
	"bytes"
	"compress/zlib"
	"database/sql"
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/garyburd/redigo/redis"
	"github.com/go-sql-driver/mysql"
	"io"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// ----------------------------------------------------------------------------------
// Processor Name: Guerrilla-reds-db
// ----------------------------------------------------------------------------------
// Description   : Saves the body to redis, meta data to mysql. Example
// ----------------------------------------------------------------------------------
// Config Options: ...
// --------------:-------------------------------------------------------------------
// Input         : envelope
// ----------------------------------------------------------------------------------
// Output        :
// ----------------------------------------------------------------------------------
func init() {
	processors["GuerrillaRedisDB"] = func() Decorator {
		return GuerrillaDbReddis()
	}
}

// how many rows to batch at a time
const GuerrillaDBAndRedisBatchMax = 2

// tick on every...
const GuerrillaDBAndRedisBatchTimeout = time.Second * 3

type GuerrillaDBAndRedisBackend struct {
	config    *guerrillaDBAndRedisConfig
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

// Load the backend config for the backend. It has already been unmarshalled
// from the main config file 'backend' config "backend_config"
// Now we need to convert each type and copy into the guerrillaDBAndRedisConfig struct
func (g *GuerrillaDBAndRedisBackend) loadConfig(backendConfig BackendConfig) (err error) {
	configType := BaseConfig(&guerrillaDBAndRedisConfig{})
	bcfg, err := Svc.ExtractConfig(backendConfig, configType)
	if err != nil {
		return err
	}
	m := bcfg.(*guerrillaDBAndRedisConfig)
	g.config = m
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

var workerId = 0

// GuerrillaDbReddis is a specialized processor for Guerrilla mail. It is here as an example.
// It's an example of a 'monolithic' processor.
func GuerrillaDbReddis() Decorator {

	g := GuerrillaDBAndRedisBackend{}
	redisClient := &redisClient{}

	var db *sql.DB
	var to, body string

	var redisErr error

	Svc.AddInitializer(InitializeWith(func(backendConfig BackendConfig) error {
		configType := BaseConfig(&guerrillaDBAndRedisConfig{})
		bcfg, err := Svc.ExtractConfig(backendConfig, configType)
		if err != nil {
			return err
		}
		g.config = bcfg.(*guerrillaDBAndRedisConfig)
		db, err = g.mysqlConnect()
		if err != nil {
			Log().Fatalf("cannot open mysql: %s", err)
		}
		return nil
	}))

	workerId++

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

	return func(c Processor) Processor {
		return ProcessWith(func(e *mail.Envelope, task SelectTask) (Result, error) {
			if task == TaskSaveMail {
				Log().Debug("Got mail from chan", e.RemoteIP)
				to = trimToLimit(strings.TrimSpace(e.RcptTo[0].User)+"@"+g.config.PrimaryHost, 255)
				e.Helo = trimToLimit(e.Helo, 255)
				e.RcptTo[0].Host = trimToLimit(e.RcptTo[0].Host, 255)
				ts := fmt.Sprintf("%d", time.Now().UnixNano())
				e.ParseHeaders()
				hash := MD5Hex(
					to,
					e.MailFrom.String(),
					e.Subject,
					ts)
				// Add extra headers
				var addHead string
				addHead += "Delivered-To: " + to + "\r\n"
				addHead += "Received: from " + e.Helo + " (" + e.Helo + "  [" + e.RemoteIP + "])\r\n"
				addHead += "	by " + e.RcptTo[0].Host + " with SMTP id " + hash + "@" + e.RcptTo[0].Host + ";\r\n"
				addHead += "	" + time.Now().Format(time.RFC1123Z) + "\r\n"

				// data will be compressed when printed, with addHead added to beginning

				data.set([]byte(addHead), &e.Data)
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
					trimToLimit(e.MailFrom.String(), 255),
					trimToLimit(e.Subject, 255),
					body,
					data.String(),
					hash,
					trimToLimit(to, 255),
					e.RemoteIP,
					trimToLimit(e.MailFrom.String(), 255),
					e.TLS)
				return c.Process(e, task)

			} else {
				return c.Process(e, task)
			}
		})
	}
}
