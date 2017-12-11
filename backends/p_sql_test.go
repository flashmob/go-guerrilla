package backends

import (
	"database/sql"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"

	_ "github.com/go-sql-driver/mysql"
)

var (
	mailTableFlag = flag.String("mail-table", "test", "Table to use for testing the SQL backend")
	sqlDSNFlag    = flag.String("sql-dsn", "", "DSN to use for testing the SQL backend")
	sqlDriverFlag = flag.String("sql-driver", "mysql", "Driver to use for testing the SQL backend")
)

func TestSQL(t *testing.T) {
	if *sqlDSNFlag == "" {
		t.Skip("requires -sql-dsn to run")
	}

	logger, err := log.GetLogger(log.OutputOff.String(), log.DebugLevel.String())
	if err != nil {
		t.Fatal("get logger:", err)
	}

	cfg := BackendConfig{
		"save_process":      "sql",
		"mail_table":        *mailTableFlag,
		"primary_mail_host": "example.com",
		"sql_driver":        *sqlDriverFlag,
		"sql_dsn":           *sqlDSNFlag,
	}
	backend, err := New(cfg, logger)
	if err != nil {
		t.Fatal("new backend:", err)
	}
	if err := backend.Start(); err != nil {
		t.Fatal("start backend: ", err)
	}

	hash := strconv.FormatInt(time.Now().UnixNano(), 10)
	envelope := &mail.Envelope{
		RcptTo: []mail.Address{{User: "user", Host: "example.com"}},
		Hashes: []string{hash},
	}

	// The SQL processor is expected to use the hash to queue the mail.
	result := backend.Process(envelope)
	if !strings.Contains(result.String(), hash) {
		t.Errorf("expected message to be queued with hash, got %q", result)
	}

	// Ensure that a record actually exists.
	results, err := findRows(hash)
	if err != nil {
		t.Fatal("find rows: ", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one row, got %d", len(results))
	}
}

func findRows(hash string) ([]string, error) {
	db, err := sql.Open(*sqlDriverFlag, *sqlDSNFlag)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	stmt := fmt.Sprintf(`SELECT hash FROM %s WHERE hash = ?`, *mailTableFlag)
	rows, err := db.Query(stmt, hash)
	if err != nil {
		return nil, err
	}

	var results []string
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}
