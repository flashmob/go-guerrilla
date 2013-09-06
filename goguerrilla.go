/** 
Go-Guerrilla SMTPd
A minimalist SMTP server written in Go, made for receiving large volumes of mail.
Works either as a stand-alone or in conjunction with Nginx SMTP proxy.
TO DO: add http server for nginx

Copyright (c) 2012 Flashmob, GuerrillaMail.com

See LICENSE for licensing details

What is Go Guerrilla SMTPd?
It's a small SMTP server written in Go, optimized for receiving email.
Written for GuerrillaMail.com which processes tens of thousands of emails
every hour.

Benchmarking:
http://www.jrh.org/smtp/index.html
Test 500 clients:
$ time smtp-source -c -l 5000 -t test@spam4.me -s 500 -m 5000 5.9.7.183

Version: 1.1
Author: Flashmob, GuerrillaMail.com
Contact: flashmob@gmail.com
License: MIT
Repository: https://github.com/flashmob/Go-Guerrilla-SMTPd
Site: http://www.guerrillamail.com/

See README for more details

To build
Install the following
$ go get github.com/ziutek/mymysql/thrsafe
$ go get github.com/ziutek/mymysql/autorc
$ go get github.com/ziutek/mymysql/godrv
$ go get github.com/sloonz/go-iconv

TODO: after failing tls, 

patch:
rebuild all: go build -a -v new.go

*/

package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/sloonz/go-iconv"
	"github.com/sloonz/go-qprintable"
	"github.com/ziutek/mymysql/autorc"
	_ "github.com/ziutek/mymysql/godrv"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	state       int
	helo        string
	mail_from   string
	rcpt_to     string
	read_buffer string
	response    string
	address     string
	data        string
	subject     string
	hash        string
	time        int64
	tls_on      bool
	conn        net.Conn
	bufin       *bufio.Reader
	bufout      *bufio.Writer
	kill_time   int64
	errors      int
	clientId    int64
	savedNotify chan int
}

var TLSconfig *tls.Config
var max_size int // max email DATA size
var timeout time.Duration
var allowedHosts = make(map[string]bool, 15)
var sem chan int // currently active clients
var SaveMailChan chan *Client // workers for saving mail

// defaults. Overwrite any of these in the configure() function which loads them from a json file
var gConfig = map[string]string{
	"GSMTP_MAX_SIZE":         "131072",
	"GSMTP_HOST_NAME":        "gsmtp.example.com", // This should also be set to reflect your RDNS
	"GSMTP_VERBOSE":          "Y",
	"GSMTP_LOG_FILE":         "",    // Eg. /var/log/goguerrilla.log or leave blank if no logging
	"GSMTP_TIMEOUT":          "100", // how many seconds before timeout.
	"MYSQL_HOST":             "127.0.0.1:3306",
	"MYSQL_USER":             "go_guerrilla",
	"MYSQL_PASS":             "ok",
	"MYSQL_DB":               "go_guerrilla",
	"GM_MAIL_TABLE":          "mail_queue",
	"GSTMP_LISTEN_INTERFACE": "127.0.0.1:25",
	"GSMTP_PUB_KEY":          "./certs/ssl-cert-snakeoil.pem",
	"GSMTP_PRV_KEY":          "./certs/ssl-cert-snakeoil.key",
	"GM_ALLOWED_HOSTS":       "example.com",
	"GM_PRIMARY_MAIL_HOST":   "smarthost.example.com",
	"GM_MAX_CLIENTS":         "500",
	"NGINX_AUTH_ENABLED":     "N",              // Y or N
	"NGINX_AUTH":             "127.0.0.1:8025", // If using Nginx proxy, ip and port to serve Auth requsts
	"SGID":                   "1008",           // group id
	"SUID":                   "1008",           // user id, from /etc/passwd
}

type redisClient struct {
	count int
	conn  redis.Conn
	time  int
}

func logln(level int, s string) {

	if gConfig["GSMTP_VERBOSE"] == "Y" {
		fmt.Println(s)
	}
	if level == 2 {
		log.Fatalf(s)
	}
	if len(gConfig["GSMTP_LOG_FILE"]) > 0 {
		log.Println(s)
	}
}

func configure() {
	var configFile, verbose, iface string
	log.SetOutput(os.Stdout)
	// parse command line arguments
	flag.StringVar(&configFile, "config", "goguerrilla.conf", "Path to the configuration file")
	flag.StringVar(&verbose, "v", "n", "Verbose, [y | n] ")
	flag.StringVar(&iface, "if", "", "Interface and port to listen on, eg. 127.0.0.1:2525 ")
	flag.Parse()
	// load in the config.
	b, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatalln("Could not read config file")
	}
	var myConfig map[string]string
	err = json.Unmarshal(b, &myConfig)
	if err != nil {
		log.Fatalln("Could not parse config file")
	}
	for k, v := range myConfig {
		gConfig[k] = v
	}
	gConfig["GSMTP_VERBOSE"] = strings.ToUpper(verbose)
	if len(iface) > 0 {
		gConfig["GSTMP_LISTEN_INTERFACE"] = iface
	}
	// map the allow hosts for easy lookup
	if arr := strings.Split(gConfig["GM_ALLOWED_HOSTS"], ","); len(arr) > 0 {
		for i := 0; i < len(arr); i++ {
			allowedHosts[arr[i]] = true
		}
	}
	var n int
	var n_err error
	// sem is an active clients channel used for counting clients
	if n, n_err = strconv.Atoi(gConfig["GM_MAX_CLIENTS"]); n_err != nil {
		n = 50
	}
	// currently active client list
	sem = make(chan int, n)
	// database writing workers
	SaveMailChan = make(chan *Client, 5)
	// timeout for reads
	if n, n_err = strconv.Atoi(gConfig["GSMTP_TIMEOUT"]); n_err != nil {
		timeout = time.Duration(10)
	} else {
		timeout = time.Duration(n)
	}
	// max email size
	if max_size, n_err = strconv.Atoi(gConfig["GSMTP_MAX_SIZE"]); n_err != nil {
		max_size = 131072
	}
	// custom log file
	if len(gConfig["GSMTP_LOG_FILE"]) > 0 {
		logfile, err := os.OpenFile(gConfig["GSMTP_LOG_FILE"], os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_SYNC, 0600)
		if err != nil {
			log.Fatal("Unable to open log file ["+gConfig["GSMTP_LOG_FILE"]+"]: ", err)
		}
		log.SetOutput(logfile)
	}

	return
}

func main() {
	configure()
	cert, err := tls.LoadX509KeyPair(gConfig["GSMTP_PUB_KEY"], gConfig["GSMTP_PRV_KEY"])
	if err != nil {
		logln(2, fmt.Sprintf("There was a problem with loading the certificate: %s", err))
	}
	TLSconfig = &tls.Config{Certificates: []tls.Certificate{cert}, ClientAuth: tls.VerifyClientCertIfGiven, ServerName: gConfig["GSMTP_HOST_NAME"]}
	TLSconfig.Rand = rand.Reader
	// start some savemail workers
	for i := 0; i < 3; i++ {
		go saveMail()
	}
	if gConfig["NGINX_AUTH_ENABLED"] == "Y" {
		go nginxHTTPAuth()
	}
	// Start listening for SMTP connections
	listener, err := net.Listen("tcp", gConfig["GSTMP_LISTEN_INTERFACE"])
	if err != nil {
		logln(2, fmt.Sprintf("Cannot listen on port, %v", err))
	} else {
		logln(1, fmt.Sprintf("Listening on tcp %s", gConfig["GSTMP_LISTEN_INTERFACE"]))
	}
	var clientId int64
	clientId = 1
	for {
		conn, err := listener.Accept()
		if err != nil {
			logln(1, fmt.Sprintf("Accept error: %s", err))
			continue
		}
		logln(1, fmt.Sprintf(" There are now "+strconv.Itoa(runtime.NumGoroutine())+" serving goroutines"))
		sem <- 1 // Wait for active queue to drain.
		go handleClient(&Client{
			conn:        conn,
			address:     conn.RemoteAddr().String(),
			time:        time.Now().Unix(),
			bufin:       bufio.NewReader(conn),
			bufout:      bufio.NewWriter(conn),
			clientId:    clientId,
			savedNotify: make(chan int),
		})
		clientId++
	}
}

func handleClient(client *Client) {
	defer closeClient(client)
	//	defer closeClient(client)
	greeting := "220 " + gConfig["GSMTP_HOST_NAME"] +
		" SMTP Guerrilla-SMTPd #" + strconv.FormatInt(client.clientId, 10) + " (" + strconv.Itoa(len(sem)) + ") " + time.Now().Format(time.RFC1123Z)
	advertiseTls := "250-STARTTLS\r\n"
	for i := 0; i < 100; i++ {
		switch client.state {
		case 0:
			responseAdd(client, greeting)
			client.state = 1
		case 1:
			input, err := readSmtp(client)
			if err != nil {
				logln(1, fmt.Sprintf("Read error: %v", err))
				if err == io.EOF {
					// client closed the connection already
					return
				}
				if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					// too slow, timeout
					return
				}
				break
			}
			input = strings.Trim(input, " \n\r")
			cmd := strings.ToUpper(input)
			switch {
			case strings.Index(cmd, "HELO") == 0:
				if len(input) > 5 {
					client.helo = input[5:]
				}
				responseAdd(client, "250 "+gConfig["GSMTP_HOST_NAME"]+" Hello ")
			case strings.Index(cmd, "EHLO") == 0:
				if len(input) > 5 {
					client.helo = input[5:]
				}
				responseAdd(client, "250-"+gConfig["GSMTP_HOST_NAME"]+" Hello "+client.helo+"["+client.address+"]"+"\r\n"+"250-SIZE "+gConfig["GSMTP_MAX_SIZE"]+"\r\n"+advertiseTls+"250 HELP")
			case strings.Index(cmd, "MAIL FROM:") == 0:
				if len(input) > 10 {
					client.mail_from = input[10:]
				}
				responseAdd(client, "250 Ok")
			case strings.Index(cmd, "XCLIENT") == 0:
				// Nginx sends this
				// XCLIENT ADDR=212.96.64.216 NAME=[UNAVAILABLE]
				client.address = input[13:]
				client.address = client.address[0:strings.Index(client.address, " ")]
				fmt.Println("client address:[" + client.address + "]")
				responseAdd(client, "250 OK")
			case strings.Index(cmd, "RCPT TO:") == 0:
				if len(input) > 8 {
					client.rcpt_to = input[8:]
				}
				responseAdd(client, "250 Accepted")
			case strings.Index(cmd, "NOOP") == 0:
				responseAdd(client, "250 OK")
			case strings.Index(cmd, "RSET") == 0:
				client.mail_from = ""
				client.rcpt_to = ""
				responseAdd(client, "250 OK")
			case strings.Index(cmd, "DATA") == 0:
				responseAdd(client, "354 Enter message, ending with \".\" on a line by itself")
				client.state = 2
			case (strings.Index(cmd, "STARTTLS") == 0) && !client.tls_on:
				responseAdd(client, "220 Ready to start TLS")
				// go to start TLS state
				client.state = 3
			case strings.Index(cmd, "QUIT") == 0:
				responseAdd(client, "221 Bye")
				killClient(client)
			default:
				responseAdd(client, fmt.Sprintf("500 unrecognized command"))
				client.errors++
				if client.errors > 3 {
					responseAdd(client, fmt.Sprintf("500 Too many unrecognized commands"))
					killClient(client)
				}
			}
		case 2:
			var err error
			client.data, err = readSmtp(client)
			if err == nil {
				// to do: timeout when adding to SaveMailChan
				// place on the channel so that one of the save mail workers can pick it up
				SaveMailChan <- client
				// wait for the save to complete
				status := <-client.savedNotify

				if status == 1 {
					responseAdd(client, "250 OK : queued as "+client.hash)
				} else {
					responseAdd(client, "554 Error: transaction failed, blame it on the weather")
				}
			} else {
				logln(1, fmt.Sprintf("DATA read error: %v", err))
			}
			client.state = 1
		case 3:
			// upgrade to TLS
			var tlsConn *tls.Conn
			tlsConn = tls.Server(client.conn, TLSconfig)
			err := tlsConn.Handshake() // not necessary to call here, but might as well
			if err == nil {
				client.conn = net.Conn(tlsConn)
				client.bufin = bufio.NewReader(client.conn)
				client.bufout = bufio.NewWriter(client.conn)
				client.tls_on = true
			} else {
				logln(1, fmt.Sprintf("Could not TLS handshake:%v", err))
			}
			advertiseTls = ""
			client.state = 1
		}
		// Send a response back to the client
		err := responseWrite(client)
		if err != nil {
			if err == io.EOF {
				// client closed the connection already
				return
			}
			if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				// too slow, timeout
				return
			}
		}
		if client.kill_time > 1 {
			return
		}
	}

}

func responseAdd(client *Client, line string) {
	client.response = line + "\r\n"
}
func closeClient(client *Client) {
	client.conn.Close()
	<-sem // Done; enable next client to run.
}
func killClient(client *Client) {
	client.kill_time = time.Now().Unix()
}

func readSmtp(client *Client) (input string, err error) {
	var reply string
	// Command state terminator by default
	suffix := "\r\n"
	if client.state == 2 {
		// DATA state
		suffix = "\r\n.\r\n"
	}
	for err == nil {
		client.conn.SetDeadline(time.Now().Add(timeout * time.Second))
		reply, err = client.bufin.ReadString('\n')
		if reply != "" {
			input = input + reply
			if len(input) > max_size {
				err = errors.New("Maximum DATA size exceeded (" + strconv.Itoa(max_size) + ")")
				return input, err
			}
			if client.state == 2 {
				// Extract the subject while we are at it.
				scanSubject(client, reply)
			}
		}
		if err != nil {
			break
		}
		if strings.HasSuffix(input, suffix) {
			break
		}
	}
	return input, err
}

// Scan the data part for a Subject line. Can be a multi-line
func scanSubject(client *Client, reply string) {
	if client.subject == "" && (len(reply) > 8) {
		test := strings.ToUpper(reply[0:9])
		if i := strings.Index(test, "SUBJECT: "); i == 0 {
			// first line with \r\n
			client.subject = reply[9:]
		}
	} else if strings.HasSuffix(client.subject, "\r\n") {
		// chop off the \r\n
		client.subject = client.subject[0 : len(client.subject)-2]
		if (strings.HasPrefix(reply, " ")) || (strings.HasPrefix(reply, "\t")) {
			// subject is multi-line
			client.subject = client.subject + reply[1:]
		}
	}
}

func responseWrite(client *Client) (err error) {
	var size int
	client.conn.SetDeadline(time.Now().Add(timeout * time.Second))
	size, err = client.bufout.WriteString(client.response)
	client.bufout.Flush()
	client.response = client.response[size:]
	return err
}

func saveMail() {
	var to string
	var err error
	var body string
	var redis_err error
	var length int
	redis := &redisClient{}
	db := autorc.New("tcp", "", gConfig["MYSQL_HOST"], gConfig["MYSQL_USER"], gConfig["MYSQL_PASS"], gConfig["MYSQL_DB"])
	db.Register("set names utf8")
	sql := "INSERT INTO " + gConfig["GM_MAIL_TABLE"] + " "
	sql += "(`date`, `to`, `from`, `subject`, `body`, `charset`, `mail`, `spam_score`, `hash`, `content_type`, `recipient`, `has_attach`, `ip_addr`)"
	sql += " values (NOW(), ?, ?, ?, ? , 'UTF-8' , ?, 0, ?, '', ?, 0, ?)"
	ins, sql_err := db.Prepare(sql)
	if sql_err != nil {
		logln(2, fmt.Sprintf("Sql statement incorrect: %s", sql_err))
	}
	sql = "UPDATE _settings SET `setting_value` = `setting_value`+1 WHERE `setting_name`='received_emails' LIMIT 1"
	incr, sql_err := db.Prepare(sql)
	if sql_err != nil {
		logln(2, fmt.Sprintf("Sql statement incorrect: %s", sql_err))
	}

	//  receives values from the channel repeatedly until it is closed.
	for {
		client := <-SaveMailChan
		if user, _, addr_err := validateEmailData(client); addr_err != nil { // user, host, addr_err
			logln(1, fmt.Sprintln("mail_from didnt validate: %v", addr_err)+" client.mail_from:"+client.mail_from)
			// notify client that a save completed, -1 = error
			client.savedNotify <- -1
			continue
		} else {
			to = user + "@" + gConfig["GM_PRIMARY_MAIL_HOST"]
		}
		length = len(client.data)
		client.subject = mimeHeaderDecode(client.subject)
		client.hash = md5hex(to + client.mail_from + client.subject + strconv.FormatInt(time.Now().UnixNano(), 10))
		// Add extra headers
		add_head := ""
		add_head += "Delivered-To: " + to + "\r\n"
		add_head += "Received: from " + client.helo + " (" + client.helo + "  [" + client.address + "])\r\n"
		add_head += "	by " + gConfig["GSMTP_HOST_NAME"] + " with SMTP id " + client.hash + "@" +
			gConfig["GSMTP_HOST_NAME"] + ";\r\n"
		add_head += "	" + time.Now().Format(time.RFC1123Z) + "\r\n"
		// compress to save space
		client.data = compress(add_head + client.data)
		body = "gzencode"
		redis_err = redis.redisConnection()
		if redis_err == nil {
			_, do_err := redis.conn.Do("SETEX", client.hash, 3600, client.data)
			if do_err == nil {
				client.data = ""
				body = "redis"
			}
		} else {
			logln(1, fmt.Sprintf("redis: %v", redis_err))
		}
		// bind data to cursor
		ins.Bind(
			to,
			client.mail_from,
			client.subject,
			body,
			client.data,
			client.hash,
			to,
			client.address)
		// save, discard result
		_, _, err = ins.Exec()
		if err != nil {
			logln(1, fmt.Sprintf("Database error, %v %v", err))
			client.savedNotify <- -1
		} else {
			logln(1, "Email saved "+client.hash+" len:"+strconv.Itoa(length))
			_, _, err = incr.Exec()
			if err != nil {
				logln(1, fmt.Sprintf("Failed to incr count:", err))
			}
			client.savedNotify <- 1
		}
	}
}

func (c *redisClient) redisConnection() (err error) {
	if c.count > 100 {
		c.conn.Close()
		c.count = 0
	}
	if c.count == 0 {
		c.conn, err = redis.Dial("tcp", ":6379")
		if err != nil {
			// handle error
			return err
		}
	}
	return nil
}

func validateEmailData(client *Client) (user string, host string, addr_err error) {
	if user, host, addr_err = extractEmail(client.mail_from); addr_err != nil {
		return user, host, addr_err
	}
	client.mail_from = user + "@" + host
	if user, host, addr_err = extractEmail(client.rcpt_to); addr_err != nil {
		return user, host, addr_err
	}
	client.rcpt_to = user + "@" + host
	// check if on allowed hosts
	if allowed := allowedHosts[host]; !allowed {
		return user, host, errors.New("invalid host:" + host)
	}
	return user, host, addr_err
}

func extractEmail(str string) (name string, host string, err error) {
	re, _ := regexp.Compile(`<(.+?)@(.+?)>`) // go home regex, you're drunk!
	if matched := re.FindStringSubmatch(str); len(matched) > 2 {
		host = validHost(matched[2])
		name = matched[1]
	} else {
		if res := strings.Split(str, "@"); len(res) > 1 {
			name = res[0]
			host = validHost(res[1])
		}
	}
	if host == "" || name == "" {
		err = errors.New("Invalid address, [" + name + "@" + host + "] address:" + str)
	}
	return name, host, err
}

// Decode strings in Mime header format
// eg. =?ISO-2022-JP?B?GyRCIVo9dztSOWJAOCVBJWMbKEI=?=
func mimeHeaderDecode(str string) string {
	reg, _ := regexp.Compile(`=\?(.+?)\?([QBqp])\?(.+?)\?=`)
	matched := reg.FindAllStringSubmatch(str, -1)
	var charset, encoding, payload string
	if matched != nil {
		for i := 0; i < len(matched); i++ {
			if len(matched[i]) > 2 {
				charset = matched[i][1]
				encoding = strings.ToUpper(matched[i][2])
				payload = matched[i][3]
				switch encoding {
				case "B":
					str = strings.Replace(str, matched[i][0], mailTransportDecode(payload, "base64", charset), 1)
				case "Q":
					str = strings.Replace(str, matched[i][0], mailTransportDecode(payload, "quoted-printable", charset), 1)
				}
			}
		}
	}
	return str
}

func validHost(host string) string {
	host = strings.Trim(host, " ")
	re, _ := regexp.Compile(`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$`)
	if re.MatchString(host) {
		return host
	}
	return ""
}

// decode from 7bit to 8bit UTF-8
// encoding_type can be "base64" or "quoted-printable"
func mailTransportDecode(str string, encoding_type string, charset string) string {
	if charset == "" {
		charset = "UTF-8"
	} else {
		charset = strings.ToUpper(charset)
	}
	if encoding_type == "base64" {
		str = fromBase64(str)
	} else if encoding_type == "quoted-printable" {
		str = fromQuotedP(str)
	}
	if charset != "UTF-8" {
		charset = fixCharset(charset)
		// eg. charset can be "ISO-2022-JP"
		convstr, err := iconv.Conv(str, "UTF-8", charset)
		if err == nil {
			return convstr
		}
	}
	return str
}

func fromBase64(data string) string {
	buf := bytes.NewBufferString(data)
	decoder := base64.NewDecoder(base64.StdEncoding, buf)
	res, _ := ioutil.ReadAll(decoder)
	return string(res)
}

func fromQuotedP(data string) string {
	buf := bytes.NewBufferString(data)
	decoder := qprintable.NewDecoder(qprintable.BinaryEncoding, buf)
	res, _ := ioutil.ReadAll(decoder)
	return string(res)
}

func compress(s string) string {
	var b bytes.Buffer
	w, _ := zlib.NewWriterLevel(&b, zlib.BestSpeed) // flate.BestCompression
	w.Write([]byte(s))
	w.Close()
	return b.String()
}

func fixCharset(charset string) string {
	reg, _ := regexp.Compile(`[_:.\/\\]`)
	fixed_charset := reg.ReplaceAllString(charset, "-")
	// Fix charset
	// borrowed from http://squirrelmail.svn.sourceforge.net/viewvc/squirrelmail/trunk/squirrelmail/include/languages.php?revision=13765&view=markup
	// OE ks_c_5601_1987 > cp949
	fixed_charset = strings.Replace(fixed_charset, "ks-c-5601-1987", "cp949", -1)
	// Moz x-euc-tw > euc-tw
	fixed_charset = strings.Replace(fixed_charset, "x-euc", "euc", -1)
	// Moz x-windows-949 > cp949
	fixed_charset = strings.Replace(fixed_charset, "x-windows_", "cp", -1)
	// windows-125x and cp125x charsets
	fixed_charset = strings.Replace(fixed_charset, "windows-", "cp", -1)
	// ibm > cp
	fixed_charset = strings.Replace(fixed_charset, "ibm", "cp", -1)
	// iso-8859-8-i -> iso-8859-8
	fixed_charset = strings.Replace(fixed_charset, "iso-8859-8-i", "iso-8859-8", -1)
	if charset != fixed_charset {
		return fixed_charset
	}
	return charset
}

func md5hex(str string) string {
	h := md5.New()
	h.Write([]byte(str))
	sum := h.Sum([]byte{})
	return hex.EncodeToString(sum)
}

// If running Nginx as a proxy, give Nginx the IP address and port for the SMTP server
// Primary use of Nginx is to terminate TLS so that Go doesn't need to deal with it.
// This could perform auth and load balancing too
// See http://wiki.nginx.org/MailCoreModule
func nginxHTTPAuth() {
	parts := strings.Split(gConfig["GSTMP_LISTEN_INTERFACE"], ":")
	gConfig["HTTP_AUTH_HOST"] = parts[0]
	gConfig["HTTP_AUTH_PORT"] = parts[1]
	fmt.Println(parts)
	http.HandleFunc("/", nginxHTTPAuthHandler)
	err := http.ListenAndServe(gConfig["NGINX_AUTH"], nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}

}

func nginxHTTPAuthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Auth-Status", "OK")
	w.Header().Add("Auth-Server", gConfig["HTTP_AUTH_HOST"])
	w.Header().Add("Auth-Port", gConfig["HTTP_AUTH_PORT"])
	fmt.Fprint(w, "")
}
