package main

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/ziutek/mymysql/autorc"
	_ "github.com/ziutek/mymysql/godrv"
	"log"
	"strconv"
	"time"
	"errors"
)

type savePayload struct {
	client *Client
	server *SmtpdServer
}

var SaveMailChan chan *savePayload // workers for saving mail

type redisClient struct {
	count int
	conn  redis.Conn
	time  int
}

func saveMail() {
	var to, recipient, body string
	var err error

	var redis_err error
	var length int
	redisClient := &redisClient{}
	db := autorc.New(
		"tcp",
		"",
		mainConfig.Mysql_host,
		mainConfig.Mysql_user,
		mainConfig.Mysql_pass,
		mainConfig.Mysql_db)
	db.Register("set names utf8")
	sql := "INSERT INTO " + mainConfig.Mysql_table + " "
	sql += "(`date`, `to`, `from`, `subject`, `body`, `charset`, `mail`, `spam_score`, `hash`, `content_type`, `recipient`, `has_attach`, `ip_addr`, `return_path`, `is_tls`)"
	sql += " values (NOW(), ?, ?, ?, ? , 'UTF-8' , ?, 0, ?, '', ?, 0, ?, ?, ?)"
	ins, sql_err := db.Prepare(sql)
	if sql_err != nil {
		log.Fatalf(fmt.Sprintf("Sql statement incorrect: %s\n", sql_err))
	}
	sql = "UPDATE gm2_setting SET `setting_value` = `setting_value`+1 WHERE `setting_name`='received_emails' LIMIT 1"
	incr, sql_err := db.Prepare(sql)
	if sql_err != nil {
		log.Fatalf(fmt.Sprintf("Sql statement incorrect: %s\n", sql_err))
	}

	//  receives values from the channel repeatedly until it is closed.
	for {
		payload := <-SaveMailChan
		if user, host, addr_err := validateEmailData(payload.client); addr_err != nil {
			payload.server.logln(1, fmt.Sprintf("mail_from didnt validate: %v", addr_err)+" client.mail_from:"+payload.client.mail_from)
			// notify client that a save completed, -1 = error
			payload.client.savedNotify <- -1
			continue
		} else {
			recipient = user + "@" + host
			to = user + "@" + mainConfig.Primary_host
		}
		length = len(payload.client.data)
		ts := strconv.FormatInt(time.Now().UnixNano(), 10);
		payload.client.subject = mimeHeaderDecode(payload.client.subject)
		payload.client.hash = md5hex(
			&to,
			&payload.client.mail_from,
			&payload.client.subject,
			&ts)
		// Add extra headers
		add_head := ""
		add_head += "Delivered-To: " + to + "\r\n"
		add_head += "Received: from " + payload.client.helo + " (" + payload.client.helo + "  [" + payload.client.address + "])\r\n"
		add_head += "	by " + payload.server.Config.Host_name + " with SMTP id " + payload.client.hash + "@" +
			payload.server.Config.Host_name + ";\r\n"
		add_head += "	" + time.Now().Format(time.RFC1123Z) + "\r\n"
		// compress to save space
		payload.client.data = compress(&add_head, &payload.client.data)
		body = "gzencode"
		redis_err = redisClient.redisConnection()
		if redis_err == nil {
			_, do_err := redisClient.conn.Do("SETEX", payload.client.hash, mainConfig.Redis_expire_seconds, payload.client.data)
			if do_err == nil {
				payload.client.data = ""
				body = "redis"
			}
		} else {
			payload.server.logln(1, fmt.Sprintf("redis: %v", redis_err))
		}
		// bind data to cursor
		ins.Bind(
			to,
			payload.client.mail_from,
			payload.client.subject,
			body,
			payload.client.data,
			payload.client.hash,
			recipient,
			payload.client.address,
			payload.client.mail_from,
			payload.client.tls_on,
		)
		// save, discard result
		_, _, err = ins.Exec()
		if err != nil {
			payload.server.logln(1, fmt.Sprintf("Database error, %v ", err))
			payload.client.savedNotify <- -1
		} else {
			payload.server.logln(0, "Email saved "+payload.client.hash+" len:"+strconv.Itoa(length))
			_, _, err = incr.Exec()
			if err != nil {
				payload.server.logln(1, fmt.Sprintf("Failed to incr count: %v", err))
			}
			payload.client.savedNotify <- 1
		}
	}
}

func (c *redisClient) redisConnection() (err error) {

	if c.count == 0 {
		c.conn, err = redis.Dial("tcp", mainConfig.Redis_interface)
		if err != nil {
			// handle error
			return err
		}
	}
	return nil
}

// test database connection settings
func testDbConnections() (err error) {

	db := autorc.New(
		"tcp",
		"",
		mainConfig.Mysql_host,
		mainConfig.Mysql_user,
		mainConfig.Mysql_pass,
		mainConfig.Mysql_db)

	if mysql_err := db.Raw.Connect(); mysql_err != nil {
		err = errors.New("MySql cannot connect, check your settings. " + mysql_err.Error() )
	} else {
		db.Raw.Close();
	}

	redisClient := &redisClient{}
	if redis_err := redisClient.redisConnection(); redis_err != nil {
		err = errors.New("Redis cannot connect, check your settings. " + redis_err.Error())
	}

	return
}
