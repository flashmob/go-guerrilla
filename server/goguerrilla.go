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

package server

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net"
	"runtime"
	"time"

	log "github.com/Sirupsen/logrus"

	guerrilla "github.com/flashmob/go-guerrilla"
)

func RunServer(sConfig guerrilla.ServerConfig, backend guerrilla.Backend, allowedHostsStr string) (err error) {
	server := SmtpdServer{
		Config:          sConfig,
		sem:             make(chan int, sConfig.MaxClients),
		allowedHostsStr: allowedHostsStr,
	}

	// configure ssl
	if sConfig.TLSAlwaysOn || sConfig.StartTLS {
		cert, err := tls.LoadX509KeyPair(sConfig.PublicKeyFile, sConfig.PrivateKeyFile)
		if err != nil {
			return fmt.Errorf("error while loading the certificate: %s", err)
		}
		server.tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.VerifyClientCertIfGiven,
			ServerName:   sConfig.Hostname,
		}
		server.tlsConfig.Rand = rand.Reader
	}

	// configure timeout
	server.timeout = time.Duration(sConfig.Timeout)

	// Start listening for SMTP connections
	listener, err := net.Listen("tcp", sConfig.ListenInterface)
	if err != nil {
		return fmt.Errorf("cannot listen on port, %v", err)
	}

	log.Infof("Listening on tcp %s", sConfig.ListenInterface)

	var clientID int64
	clientID = 1
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.WithError(err).Infof("Accept error")
			continue
		}
		log.Debugf("Number of serving goroutines: %d", runtime.NumGoroutine())
		server.sem <- 1 // Wait for active queue to drain.
		go server.handleClient(&guerrilla.Client{
			Conn:        conn,
			Address:     conn.RemoteAddr().String(),
			Time:        time.Now().Unix(),
			Bufin:       guerrilla.NewSMTPBufferedReader(conn),
			Bufout:      bufio.NewWriter(conn),
			ClientID:    clientID,
			SavedNotify: make(chan int),
		}, backend)
		clientID++
	}
}
