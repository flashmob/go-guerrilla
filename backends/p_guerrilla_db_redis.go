package backends

import (
	"bytes"
	"compress/zlib"
	"database/sql"
	"fmt"
	"io"
	"math/rand"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/flashmob/go-guerrilla/mail"
)

// ----------------------------------------------------------------------------------
// Processor Name: GuerrillaRedisDB
// ----------------------------------------------------------------------------------
// Description   : Saves the body to redis, meta data to SQL. Example only.
//               : Limitation: it doesn't save multiple recipients or validate them
// ----------------------------------------------------------------------------------
// Config Options: ...
// --------------:-------------------------------------------------------------------
// Input         : envelope
// ----------------------------------------------------------------------------------
// Output        :
// ----------------------------------------------------------------------------------
func init() {
	processors["guerrillaredisdb"] = func() Decorator {
		return GuerrillaDbRedis()
	}
}

var queryBatcherId = 0

// how many rows to batch at a time
const GuerrillaDBAndRedisBatchMax = 50

// tick on every...
const GuerrillaDBAndRedisBatchTimeout = time.Second * 3

type GuerrillaDBAndRedisBackend struct {
	config    *guerrillaDBAndRedisConfig
	batcherWg sync.WaitGroup
	// cache prepared queries
	cache stmtCache

	batcherStoppers []chan bool
}

// statement cache. It's an array, not slice
type stmtCache [GuerrillaDBAndRedisBatchMax]*sql.Stmt

type guerrillaDBAndRedisConfig struct {
	Table              string `json:"mail_table"`
	Driver             string `json:"sql_driver"`
	DSN                string `json:"sql_dsn"`
	RedisExpireSeconds int    `json:"redis_expire_seconds"`
	RedisInterface     string `json:"redis_interface"`
	PrimaryHost        string `json:"primary_mail_host"`
	BatchTimeout       int    `json:"redis_sql_batch_timeout,omitempty"`
}

// Load the backend config for the backend. It has already been unmarshalled
// from the main config file 'backend' config "backend_config"
// Now we need to convert each type and copy into the guerrillaDBAndRedisConfig struct
func (g *GuerrillaDBAndRedisBackend) loadConfig(backendConfig BackendConfig) (err error) {
	configType := BaseConfig(&guerrillaDBAndRedisConfig{})
	bcfg, err := Svc.ExtractConfig(ConfigProcessors, "guerrillaredisdb", backendConfig, configType)
	if err != nil {
		return err
	}
	m := bcfg.(*guerrillaDBAndRedisConfig)
	g.config = m
	return nil
}

type redisClient struct {
	isConnected bool
	conn        RedisConn
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
	_, _ = io.Copy(w, r)
	_, _ = io.Copy(w, c.data)
	_ = w.Close()
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
	sqlstr := "INSERT INTO " + g.config.Table + "" +
		"(" +
		"`date`, " +
		"`to`, " +
		"`from`, " +
		"`subject`, " +
		"`body`, " +
		"`charset`, " +
		"`mail`, " +
		"`spam_score`, " +
		"`hash`, " +
		"`content_type`, " +
		"`recipient`, " +
		"`has_attach`, " +
		"`ip_addr`, " +
		"`return_path`, " +
		"`is_tls`" +
		")" +
		" values "
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

func (g *GuerrillaDBAndRedisBackend) doQuery(c int, db *sql.DB, insertStmt *sql.Stmt, vals *[]interface{}) error {
	var execErr error
	defer func() {
		if r := recover(); r != nil {
			sum := 0
			for _, v := range *vals {
				if str, ok := v.(string); ok {
					sum = sum + len(str)
				}
			}
			Log().Fields("panic", fmt.Sprintf("%v", r),
				"size", sum,
				"error", execErr,
				"stack", string(debug.Stack())).
				Error("panic while inserting query")
			panic("query failed")
		}
	}()
	// prepare the query used to insert when rows reaches batchMax
	insertStmt = g.prepareInsertQuery(c, db)
	_, execErr = insertStmt.Exec(*vals...)
	if execErr != nil {
		Log().WithError(execErr).Error("There was a problem the insert")
	}
	return execErr
}

// Batches the rows from the feeder chan in to a single INSERT statement.
// Execute the batches query when:
// - number of batched rows reaches a threshold, i.e. count n = threshold
// - or, no new rows within a certain time, i.e. times out
// The goroutine can either exit if there's a panic or feeder channel closes
// it returns feederOk which signals if the feeder chanel was ok (still open) while returning
// if it feederOk is false, then it means the feeder chanel is closed
func (g *GuerrillaDBAndRedisBackend) insertQueryBatcher(
	feeder feedChan,
	db *sql.DB,
	batcherId int,
	stop chan bool) (feederOk bool) {

	// controls shutdown
	defer g.batcherWg.Done()
	g.batcherWg.Add(1)
	// vals is where values are batched to
	var vals []interface{}
	// how many rows were batched
	count := 0
	// The timer will tick x seconds.
	// Interrupting the select clause when there's no data on the feeder channel
	timeo := GuerrillaDBAndRedisBatchTimeout
	if g.config.BatchTimeout > 0 {
		timeo = time.Duration(g.config.BatchTimeout)
	}
	t := time.NewTimer(timeo)
	// prepare the query used to insert when rows reaches batchMax
	insertStmt := g.prepareInsertQuery(GuerrillaDBAndRedisBatchMax, db)
	// inserts executes a batched insert query, clears the vals and resets the count
	inserter := func(c int) {
		if c > 0 {
			err := g.doQuery(c, db, insertStmt, &vals)
			if err != nil {
				// maybe connection prob?
				// retry the sql query
				attempts := 3
				for i := 0; i < attempts; i++ {
					Log().Fields("rows", c).Info("retrying query query rows ")
					time.Sleep(time.Second)
					err = g.doQuery(c, db, insertStmt, &vals)
					if err == nil {
						continue
					}
				}
			}
		}
		vals = nil
		count = 0
	}
	rand.Seed(time.Now().UnixNano())
	defer func() {
		if r := recover(); r != nil {
			Log().Error("insertQueryBatcher caught a panic", r, string(debug.Stack()))
		}
	}()
	// Keep getting values from feeder and add to batch.
	// if feeder times out, execute the batched query
	// otherwise, execute the batched query once it reaches the GuerrillaDBAndRedisBatchMax threshold
	feederOk = true
	for {
		select {
		// it may panic when reading on a closed feeder channel. feederOK detects if it was closed
		case <-stop:
			Log().Fields("batcherID", batcherId).Info("MySQL query batcher stopped")
			// Insert any remaining rows
			inserter(count)
			feederOk = false
			close(feeder)
			return
		case row := <-feeder:

			vals = append(vals, row...)
			count++
			Log().Fields(
				"row", row,
				"cols", len(row),
				"count", count,
				"worker", batcherId,
			).Debug("new feeder row")
			if count >= GuerrillaDBAndRedisBatchMax {
				inserter(GuerrillaDBAndRedisBatchMax)
			}
			// stop timer from firing (reset the interrupt)
			if !t.Stop() {
				// darin the timer
				<-t.C
			}
			t.Reset(timeo)
		case <-t.C:
			// anything to insert?
			if n := len(vals); n > 0 {
				inserter(count)
			}
			t.Reset(timeo)
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

func (g *GuerrillaDBAndRedisBackend) sqlConnect() (*sql.DB, error) {
	if db, err := sql.Open(g.config.Driver, g.config.DSN); err != nil {
		Log().Error("cannot open database", err, "]")
		return nil, err
	} else {
		// do we have access?
		_, err = db.Query("SELECT mail_id FROM " + g.config.Table + " LIMIT 1")
		if err != nil {
			Log().Error("cannot select table:", err)
			return nil, err
		}
		return db, nil
	}
}

func (c *redisClient) redisConnection(redisInterface string) (err error) {
	if c.isConnected == false {
		c.conn, err = RedisDialer("tcp", redisInterface)
		if err != nil {
			// handle error
			return err
		}
		c.isConnected = true
	}
	return nil
}

type feedChan chan []interface{}

// GuerrillaDbRedis is a specialized processor for Guerrilla mail. It is here as an example.
// It's an example of a 'monolithic' processor.
func GuerrillaDbRedis() Decorator {

	g := GuerrillaDBAndRedisBackend{}
	redisClient := &redisClient{}

	var (
		db       *sql.DB
		to, body string
		redisErr error
		feeders  []feedChan
	)

	g.batcherStoppers = make([]chan bool, 0)

	Svc.AddInitializer(InitializeWith(func(backendConfig BackendConfig) error {

		configType := BaseConfig(&guerrillaDBAndRedisConfig{})
		bcfg, err := Svc.ExtractConfig(
			ConfigProcessors, "guerrillaredisdb", backendConfig, configType)
		if err != nil {
			return err
		}
		g.config = bcfg.(*guerrillaDBAndRedisConfig)
		db, err = g.sqlConnect()
		if err != nil {
			return err
		}
		queryBatcherId++
		// start the query SQL batching where we will send data via the feeder channel
		stop := make(chan bool)
		feeder := make(feedChan, 1)
		go func(qbID int, stop chan bool) {
			// we loop so that if insertQueryBatcher panics, it can recover and go in again
			for {
				if feederOK := g.insertQueryBatcher(feeder, db, qbID, stop); !feederOK {
					Log().Fields("qbID", qbID).Debug("insertQueryBatcher exited")
					return
				}
				Log().Debug("resuming insertQueryBatcher")
			}
		}(queryBatcherId, stop)
		g.batcherStoppers = append(g.batcherStoppers, stop)
		feeders = append(feeders, feeder)
		return nil
	}))

	Svc.AddShutdowner(ShutdownWith(func() error {
		if db != nil {
			if err := db.Close(); err != nil {
				Log().WithError(err).Error("close sql database")
			} else {
				Log().Info("closed sql database")
			}
		}

		if redisClient.conn != nil {
			if err := redisClient.conn.Close(); err != nil {
				Log().WithError(err).Error("close redis failed")
			} else {
				Log().Info("closed redis")
			}
		}
		// send a close signal to all query batchers to exit.
		for i := range g.batcherStoppers {
			g.batcherStoppers[i] <- true
		}
		g.batcherWg.Wait()

		return nil
	}))

	var vals []interface{}
	data := newCompressedData()

	return func(p Processor) Processor {
		return ProcessWith(func(e *mail.Envelope, task SelectTask) (Result, error) {
			if task == TaskSaveMail {
				Log().Debug("Got mail from chan,", e.RemoteIP)
				to = trimToLimit(strings.TrimSpace(e.RcptTo[0].User)+"@"+g.config.PrimaryHost, 255)
				e.Helo = trimToLimit(e.Helo, 255)
				e.RcptTo[0].Host = trimToLimit(e.RcptTo[0].Host, 255)
				ts := fmt.Sprintf("%d", time.Now().UnixNano())
				if err := e.ParseHeaders(); err != nil {
					Log().WithError(err).Error("failed to parse headers")
				}
				hash := MD5Hex(
					to,
					e.MailFrom.String(),
					e.Subject,
					ts)
				e.QueuedId.FromHex(hash)
				// Add extra headers
				var addHead string
				addHead += "Delivered-To: " + to + "\r\n"
				addHead += "Received: from " + e.RemoteIP + " ([" + e.RemoteIP + "])\r\n"
				addHead += "	by " + e.RcptTo[0].Host + " with " + e.Protocol().String() + " id " + hash + "@" + e.RcptTo[0].Host + ";\r\n"
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
				// give the values to a random query batcher
				feeders[rand.Intn(len(feeders))] <- vals
				return p.Process(e, task)

			} else {
				return p.Process(e, task)
			}
		})
	}
}
