/**
Go-Guerrilla SMTPd

Version: 1.3
Author: Flashmob, GuerrillaMail.com
Contact: flashmob@gmail.com
License: MIT
Repository: https://github.com/flashmob/Go-Guerrilla-SMTPd
Site: http://www.guerrillamail.com/

See README for more details


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
	"errors"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/sloonz/go-qprintable"
	"github.com/ziutek/mymysql/autorc"
	_ "github.com/ziutek/mymysql/godrv"
	"gopkg.in/iconv.v1"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
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

type SmtpdServer struct {
	tlsConfig    *tls.Config
	max_size     int // max email DATA size
	timeout      time.Duration
	allowedHosts map[string]bool
	sem          chan int // currently active client list
	Config       ServerConfig
	logger       *log.Logger
}

type savePayload struct {
	client *Client
	server *SmtpdServer
}

var allowedHosts = make(map[string]bool, 15)

//var sem chan int // currently active clients
var signalChannel = make(chan os.Signal, 1) // for trapping SIG_HUB
var SaveMailChan chan *savePayload          // workers for saving mail

type redisClient struct {
	count int
	conn  redis.Conn
	time  int
}

func (server *SmtpdServer) logln(level int, s string) {

	if theConfig.Verbose {
		fmt.Println(s)
	}
	// fatal errors
	if level == 2 {
		server.logger.Fatalf(s)
	}
	// warnings
	if level == 1 && len(server.Config.Log_file) > 0 {
		server.logger.Println(s)
	}

}

func (server *SmtpdServer) openLog() {

	server.logger = log.New(&bytes.Buffer{}, "", log.Lshortfile)
	// custom log file
	if len(server.Config.Log_file) > 0 {
		logfile, err := os.OpenFile(
			server.Config.Log_file,
			os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_SYNC, 0600)
		if err != nil {
			server.logln(1, fmt.Sprintf("Unable to open log file [%s]: %s ", server.Config.Log_file, err))
		}
		server.logger.SetOutput(logfile)
	}
}

func sigHandler() {
	for sig := range signalChannel {
		if sig == syscall.SIGHUP {
			readConfig()
			fmt.Print("Reloading Configuration!\n")
		} else {
			os.Exit(0)
		}

	}
}

func initialise() {

	// database writing workers
	SaveMailChan = make(chan *savePayload, theConfig.Save_workers_size)

	// write out our PID
	if f, err := os.Create(theConfig.Pid_file); err == nil {
		defer f.Close()
		if _, err := f.WriteString(strconv.Itoa(os.Getpid())); err == nil {
			f.Sync()
		}
	}
	// handle SIGHUP for reloading the configuration while running
	signal.Notify(signalChannel, syscall.SIGHUP)
	//go sigHandler()
	return
}

func runServer(sConfig ServerConfig) {
	server := SmtpdServer{Config: sConfig, sem: make(chan int, sConfig.Max_clients)}

	// setup logging
	server.openLog()

	// configure ssl
	cert, err := tls.LoadX509KeyPair(sConfig.Public_key_file, sConfig.Private_key_file)
	if err != nil {
		server.logln(2, fmt.Sprintf("There was a problem with loading the certificate: %s", err))
	}
	server.tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.VerifyClientCertIfGiven,
		ServerName:   sConfig.Host_name,
	}
	server.tlsConfig.Rand = rand.Reader

	// configure timeout
	server.timeout = time.Duration(sConfig.Timeout)

	// Start listening for SMTP connections
	listener, err := net.Listen("tcp", sConfig.Listen_interface)
	if err != nil {
		server.logln(2, fmt.Sprintf("Cannot listen on port, %v", err))
	} else {
		server.logln(1, fmt.Sprintf("Listening on tcp %s", sConfig.Listen_interface))
	}
	var clientId int64
	clientId = 1
	for {
		conn, err := listener.Accept()
		if err != nil {
			server.logln(1, fmt.Sprintf("Accept error: %s", err))
			continue
		}
		server.logln(0, fmt.Sprintf(" There are now "+strconv.Itoa(runtime.NumGoroutine())+" serving goroutines"))
		server.sem <- 1 // Wait for active queue to drain.
		go server.handleClient(&Client{
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

func main() {
	readConfig()
	initialise()
	// start some savemail workers
	for i := 0; i < 3; i++ {
		go saveMail()
	}
	// run our servers
	for serverId := 0; serverId < len(theConfig.Servers); serverId++ {
		if theConfig.Servers[serverId].Is_enabled {
			go runServer(theConfig.Servers[serverId])
		}
	}
	sigHandler();

}

func (server *SmtpdServer) upgradeToTls(client *Client) bool {
	var tlsConn *tls.Conn
	tlsConn = tls.Server(client.conn, server.tlsConfig)
	err := tlsConn.Handshake() // not necessary to call here, but might as well
	if err == nil {
		client.conn = net.Conn(tlsConn)
		client.bufin = bufio.NewReader(client.conn)
		client.bufout = bufio.NewWriter(client.conn)
		client.tls_on = true
		return true;
	} else {
		server.logln(1, fmt.Sprintf("Could not TLS handshake:%v", err))
		return false;
	}

}

func (server *SmtpdServer) handleClient(client *Client) {
	defer server.closeClient(client)
	advertiseTls := "250-STARTTLS\r\n"
	if server.Config.Is_tls_on {
		if server.upgradeToTls(client) {
			advertiseTls = ""
		}
	}
	greeting := "220 " + server.Config.Host_name +
		" SMTP Guerrilla-SMTPd #" +
		strconv.FormatInt(client.clientId, 10) +
		" (" + strconv.Itoa(len(server.sem)) + ") " + time.Now().Format(time.RFC1123Z)

	if !server.Config.Start_tls_on {
		// STARTTLS turned off
		advertiseTls = ""
	}
	for i := 0; i < 100; i++ {
		switch client.state {
		case 0:
			responseAdd(client, greeting)
			client.state = 1
		case 1:
			input, err := server.readSmtp(client)
			if err != nil {
				if err == io.EOF {
					// client closed the connection already
					server.logln(0, fmt.Sprintf("%s: %v", client.address, err))
					return
				}
				if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					// too slow, timeout
					server.logln(0, fmt.Sprintf("%s: %v", client.address, err))
					return
				}
				server.logln(1, fmt.Sprintf("Read error: %v", err))
				break
			}
			input = strings.Trim(input, " \n\r")
			bound := len(input)
			if bound > 16 {
				bound = 16
			}
			cmd := strings.ToUpper(input[0:bound])
			switch {
			case strings.Index(cmd, "HELO") == 0:
				if len(input) > 5 {
					client.helo = input[5:]
				}
				responseAdd(client, "250 "+server.Config.Host_name+" Hello ")
			case strings.Index(cmd, "EHLO") == 0:
				if len(input) > 5 {
					client.helo = input[5:]
				}
				responseAdd(client, "250-"+server.Config.Host_name+
					" Hello "+client.helo+"["+client.address+"]"+"\r\n"+
					"250-SIZE "+strconv.Itoa(server.Config.Max_size)+"\r\n"+
					"250-PIPELINING \r\n"+
					advertiseTls+"250 HELP")
			case strings.Index(cmd, "HELP") == 0:
				responseAdd(client, "250 Help! I need somebody...")
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
			case (strings.Index(cmd, "STARTTLS") == 0) &&
				!client.tls_on &&
				server.Config.Start_tls_on:
				responseAdd(client, "220 Ready to start TLS")
				// go to start TLS state
				client.state = 3
			case strings.Index(cmd, "QUIT") == 0:
				responseAdd(client, "221 Bye")
				killClient(client)
			default:
				responseAdd(client, "500 unrecognized command")
				client.errors++
				if client.errors > 3 {
					responseAdd(client, "500 Too many unrecognized commands")
					killClient(client)
				}
			}
		case 2:
			var err error
			client.data, err = server.readSmtp(client)
			if err == nil {
				if _, _, mailErr := validateEmailData(client); mailErr == nil {
					// to do: timeout when adding to SaveMailChan
					// place on the channel so that one of the save mail workers can pick it up
					SaveMailChan <- &savePayload{client: client, server: server}
					// wait for the save to complete
					status := <-client.savedNotify
					if status == 1 {
						responseAdd(client, "250 OK : queued as "+client.hash)
					} else {
						responseAdd(client, "554 Error: transaction failed, blame it on the weather")
					}
				} else {
					responseAdd(client, "550 Error: "+mailErr.Error())
				}

			} else {
				server.logln(1, fmt.Sprintf("DATA read error: %v", err))
			}
			client.state = 1
		case 3:
			// upgrade to TLS
			if server.upgradeToTls(client) {
				advertiseTls = ""
				client.state = 1
			}
		}
		// Send a response back to the client
		err := server.responseWrite(client)
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
func (server SmtpdServer) closeClient(client *Client) {
	client.conn.Close()
	<-server.sem // Done; enable next client to run.
}
func killClient(client *Client) {
	client.kill_time = time.Now().Unix()
}

func (server SmtpdServer) readSmtp(client *Client) (input string, err error) {
	var reply string
	// Command state terminator by default
	suffix := "\r\n"
	if client.state == 2 {
		// DATA state
		suffix = "\r\n.\r\n"
	}
	for err == nil {
		client.conn.SetDeadline(time.Now().Add(server.timeout * time.Second))
		reply, err = client.bufin.ReadString('\n')
		if reply != "" {
			input = input + reply
			if len(input) > server.Config.Max_size {
				err = errors.New("Maximum DATA size exceeded (" + strconv.Itoa(server.Config.Max_size) + ")")
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

func (server SmtpdServer) responseWrite(client *Client) (err error) {
	var size int
	client.conn.SetDeadline(time.Now().Add(server.timeout * time.Second))
	size, err = client.bufout.WriteString(client.response)
	client.bufout.Flush()
	client.response = client.response[size:]
	return err
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
		theConfig.Mysql_host,
		theConfig.Mysql_user,
		theConfig.Mysql_pass,
		theConfig.Mysql_db)
	db.Register("set names utf8")
	sql := "INSERT INTO " + theConfig.Mysql_table + " "
	sql += "(`date`, `to`, `from`, `subject`, `body`, `charset`, `mail`, `spam_score`, `hash`, `content_type`, `recipient`, `has_attach`, `ip_addr`, `return_path`)"
	sql += " values (NOW(), ?, ?, ?, ? , 'UTF-8' , ?, 0, ?, '', ?, 0, ?, ?)"
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
			to = user + "@" + theConfig.Primary_host
		}
		length = len(payload.client.data)
		payload.client.subject = mimeHeaderDecode(payload.client.subject)
		payload.client.hash = md5hex(to + payload.client.mail_from + payload.client.subject + strconv.FormatInt(time.Now().UnixNano(), 10))
		// Add extra headers
		add_head := ""
		add_head += "Delivered-To: " + to + "\r\n"
		add_head += "Received: from " + payload.client.helo + " (" + payload.client.helo + "  [" + payload.client.address + "])\r\n"
		add_head += "	by " + payload.server.Config.Host_name + " with SMTP id " + payload.client.hash + "@" +
			payload.server.Config.Host_name + ";\r\n"
		add_head += "	" + time.Now().Format(time.RFC1123Z) + "\r\n"
		// compress to save space
		payload.client.data = compress(add_head + payload.client.data)
		body = "gzencode"
		redis_err = redisClient.redisConnection()
		if redis_err == nil {
			_, do_err := redisClient.conn.Do("SETEX", payload.client.hash, theConfig.redis_expire_seconds, payload.client.data)
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
			payload.client.mail_from)
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
	if allowed := allowedHosts[strings.ToLower(host)]; !allowed {
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
					str = strings.Replace(
						str,
						matched[i][0],
						mailTransportDecode(payload, "base64", charset),
						1)
				case "Q":
					str = strings.Replace(
						str,
						matched[i][0],
						mailTransportDecode(payload, "quoted-printable", charset),
						1)
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
		if cd, err := iconv.Open("UTF-8", charset); err == nil {
			defer func() {
				cd.Close()
				if r := recover(); r != nil {
					//logln(1, fmt.Sprintf("Recovered in %v", r))
				}
			}()
			// eg. charset can be "ISO-2022-JP"
			return cd.ConvString(str)
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
