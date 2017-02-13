package backends

import (
	"database/sql"
	"strings"
	"time"

	"github.com/flashmob/go-guerrilla/envelope"
	"github.com/go-sql-driver/mysql"

	"runtime/debug"
)

type MysqlProcessorConfig struct {
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

type MysqlProcessor struct {
	cache  stmtCache
	config *MysqlProcessorConfig
}

func (m *MysqlProcessor) connect(config *MysqlProcessorConfig) (*sql.DB, error) {
	var db *sql.DB
	var err error
	conf := mysql.Config{
		User:         config.MysqlUser,
		Passwd:       config.MysqlPass,
		DBName:       config.MysqlDB,
		Net:          "tcp",
		Addr:         config.MysqlHost,
		ReadTimeout:  GuerrillaDBAndRedisBatchTimeout + (time.Second * 10),
		WriteTimeout: GuerrillaDBAndRedisBatchTimeout + (time.Second * 10),
		Params:       map[string]string{"collation": "utf8_general_ci"},
	}
	if db, err = sql.Open("mysql", conf.FormatDSN()); err != nil {
		mainlog.Error("cannot open mysql", err)
		return nil, err
	}
	mainlog.Info("connected to mysql on tcp ", config.MysqlHost)
	return db, err

}

// prepares the sql query with the number of rows that can be batched with it
func (g *MysqlProcessor) prepareInsertQuery(rows int, db *sql.DB) *sql.Stmt {
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
		mainlog.WithError(sqlErr).Fatalf("failed while db.Prepare(INSERT...)")
	}
	// cache it
	g.cache[rows-1] = stmt
	return stmt
}

func (g *MysqlProcessor) doQuery(c int, db *sql.DB, insertStmt *sql.Stmt, vals *[]interface{}) {
	var execErr error
	defer func() {
		if r := recover(); r != nil {
			//logln(1, fmt.Sprintf("Recovered in %v", r))
			mainlog.Error("Recovered form panic:", r, string(debug.Stack()))
			sum := 0
			for _, v := range *vals {
				if str, ok := v.(string); ok {
					sum = sum + len(str)
				}
			}
			mainlog.Errorf("panic while inserting query [%s] size:%d, err %v", r, sum, execErr)
			panic("query failed")
		}
	}()
	// prepare the query used to insert when rows reaches batchMax
	insertStmt = g.prepareInsertQuery(c, db)
	_, execErr = insertStmt.Exec(*vals...)
	if execErr != nil {
		mainlog.WithError(execErr).Error("There was a problem the insert")
	}
}

func MySql() Decorator {

	var config *MysqlProcessorConfig
	var vals []interface{}
	var db *sql.DB
	mp := &MysqlProcessor{}

	Service.AddInitializer(Initialize(func(backendConfig BackendConfig) error {
		configType := baseConfig(&MysqlProcessorConfig{})
		bcfg, err := ab.extractConfig(backendConfig, configType)
		if err != nil {
			return err
		}
		config = bcfg.(*MysqlProcessorConfig)
		mp.config = config
		db, err = mp.connect(config)
		if err != nil {
			mainlog.Fatalf("cannot open mysql: %s", err)
			return err
		}
		return nil
	}))

	// shutdown
	Service.AddShutdowner(Shutdown(func() error {
		if db != nil {
			db.Close()
		}
		return nil
	}))

	return func(c Processor) Processor {
		return ProcessorFunc(func(e *envelope.Envelope) (BackendResult, error) {
			var to, body string
			to = trimToLimit(strings.TrimSpace(e.RcptTo[0].User)+"@"+config.PrimaryHost, 255)
			hash := ""
			if len(e.Hashes) > 0 {
				hash = e.Hashes[0]
			}

			var co *compressor
			// a compressor was set
			if e.Meta != nil {
				if c, ok := e.Meta["zlib-compressor"]; ok {
					body = "gzip"
					co = c.(*compressor)
				}
				// was saved in redis
				if _, ok := e.Meta["redis"]; ok {
					body = "redis"
				}
			}

			// build the values for the query
			vals = []interface{}{} // clear the vals
			vals = append(vals,
				to,
				trimToLimit(e.MailFrom.String(), 255),
				trimToLimit(e.Subject, 255),
				body)
			if body == "redis" {
				// data already saved in redis
				vals = append(vals, "")
			} else if co != nil {
				// use a compressor (automatically adds e.DeliveryHeader)
				vals = append(vals, co.String())
				//co.clear()
			} else {
				vals = append(vals, e.DeliveryHeader+e.Data.String())
			}

			vals = append(vals,
				hash,
				to,
				e.RemoteAddress,
				trimToLimit(e.MailFrom.String(), 255),
				e.TLS)

			stmt := mp.prepareInsertQuery(1, db)
			mp.doQuery(1, db, stmt, &vals)
			// continue to the next Processor in the decorator chain
			return c.Process(e)
		})
	}
}
