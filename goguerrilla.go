/**
Go-Guerrilla SMTPd

Version: 1.5
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
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

var allowedHosts = make(map[string]bool, 15)


var signalChannel = make(chan os.Signal, 1) // for trapping SIG_HUB

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
	SaveMailChan = make(chan *savePayload, mainConfig.Save_workers_size)

	// write out our PID
	if f, err := os.Create(mainConfig.Pid_file); err == nil {
		defer f.Close()
		if _, err := f.WriteString(strconv.Itoa(os.Getpid())); err == nil {
			f.Sync()
		}
	}
	// handle SIGHUP for reloading the configuration while running
	signal.Notify(signalChannel, syscall.SIGHUP)

	return
}

func runServer(sConfig ServerConfig) {
	server := SmtpdServer{Config: sConfig, sem: make(chan int, sConfig.Max_clients)}

	// setup logging
	server.openLog()

	// configure ssl
	if (sConfig.Tls_always_on || sConfig.Start_tls_on) {
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
	}


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
			bufin:       newSmtpBufferedReader(conn),
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
	if err := testDbConnections(); err != nil {
		fmt.Println(err)
		os.Exit(1);
	}
	// start some savemail workers
	for i := 0; i < mainConfig.Save_workers_size; i++ {
		go saveMail()
	}
	// run our servers
	for serverId := 0; serverId < len(mainConfig.Servers); serverId++ {
		if mainConfig.Servers[serverId].Is_enabled {
			go runServer(mainConfig.Servers[serverId])
		}
	}
	sigHandler()
}
