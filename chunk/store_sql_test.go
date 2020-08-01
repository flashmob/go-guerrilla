package chunk

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/mail/smtp"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/chunk/transfer"
	_ "github.com/go-sql-driver/mysql" // activate the mysql driver
)

// This test requires that you pass the -sql-dsn flag,
// eg: go test -run ^TestSQLStore$ -sql-dsn 'user:pass@tcp(127.0.0.1:3306)/dbname?readTimeout=10s&writeTimeout=10s'

var (
	mailTableFlag  = flag.String("mail-table", "in_emails", "Table to use for testing the SQL backend")
	chunkTableFlag = flag.String("mail-chunk-table", "in_emails_chunks", "Table to use for testing the chunking SQL backend")
	sqlDSNFlag     = flag.String("sql-dsn", "", "DSN to use for testing the SQL backend")
	sqlDriverFlag  = flag.String("sql-driver", "mysql", "Driver to use for testing the SQL backend")
)

func TestSQLStore(t *testing.T) {

	if *sqlDSNFlag == "" {
		t.Skip("requires -sql-dsn to run")
	}

	cfg := &backends.ConfigGroup{
		"chunk_size":         150,
		"storage_engine":     "sql",
		"compress_level":     9,
		"sql_driver":         *sqlDriverFlag,
		"sql_dsn":            *sqlDSNFlag,
		"email_table":        *mailTableFlag,
		"email_table_chunks": *chunkTableFlag,
	}

	store, chunksaver, mimeanalyzer, stream, e, err := initTestStream(false, cfg)
	if err != nil {
		t.Error(err)
		return
	}
	storeSql := store.(*StoreSQL)
	defer func() {
		storeSql.zap() // purge everything from db before exiting the test
	}()
	var out bytes.Buffer
	buf := make([]byte, 128)
	if written, err := io.CopyBuffer(stream, bytes.NewBuffer([]byte(email)), buf); err != nil {
		t.Error(err)
	} else {
		_ = mimeanalyzer.Close()
		_ = chunksaver.Close()

		fmt.Println("written:", written)
		/*
			total := 0
			for _, chunk := range storeMemory.chunks {
				total += len(chunk.data)
			}
			fmt.Println("compressed", total, "saved:", written-int64(total))
		*/

		email, err := storeSql.GetEmail(e.MessageID)

		if err != nil {
			t.Error("email not found")
			return
		}

		// check email
		if email.transport != smtp.TransportType8bit {
			t.Error("email.transport not ", smtp.TransportType8bit.String())
		}
		if email.protocol != mail.ProtocolESMTPS {
			t.Error("email.protocol not ", mail.ProtocolESMTPS)
		}

		// this should read all parts
		r, err := NewChunkedReader(storeSql, email, 0)
		if w, err := io.Copy(&out, r); err != nil {
			t.Error(err)
		} else if w != email.size {
			t.Error("email.size != number of bytes copied from reader", w, email.size)
		} else if !strings.Contains(out.String(), "GIF89") {
			t.Error("The email didn't decode properly, expecting GIF89")
		}
		out.Reset()

		// test the seek feature
		r, err = NewChunkedReader(storeSql, email, 0)
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		// we start from 1 because if the start from 0, all the parts will be read
		for i := 1; i < len(email.partsInfo.Parts); i++ {
			fmt.Println("seeking to", i)
			err = r.SeekPart(i)
			if err != nil {
				t.Error(err)
			}
			w, err := io.Copy(&out, r)
			if err != nil {
				t.Error(err)
			}
			if w != int64(email.partsInfo.Parts[i-1].Size) {
				t.Error(i, "incorrect size, expecting", email.partsInfo.Parts[i-1].Size, "but read:", w)
			}
			out.Reset()
		}

		r, err = NewChunkedReader(storeSql, email, 0)
		if err != nil {
			t.Error(err)
		}
		part := email.partsInfo.Parts[0]
		encoding := transfer.QuotedPrintable
		if strings.Contains(part.TransferEncoding, "base") {
			encoding = transfer.Base64
		}
		dr, err := transfer.NewDecoder(r, encoding, part.Charset)
		_ = dr
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		//var decoded bytes.Buffer
		//io.Copy(&decoded, dr)
		io.Copy(os.Stdout, dr)

	}
}
