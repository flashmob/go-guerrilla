/**
Go-Guerrilla SMTPd

Project Lead: Flashmob, GuerrillaMail.com
Contributors: Reza Mohammadi reza@teeleh.ir
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

	evbus "github.com/asaskevich/EventBus"

	log "github.com/Sirupsen/logrus"

	guerrilla "github.com/flashmob/go-guerrilla"
)

// configure SSL using the values from guerrilla.ServerConfig
func (s *SmtpdServer) configureSSL() error {
	sConfig := s.configStore.Load().(guerrilla.ServerConfig)
	if sConfig.TLSAlwaysOn || sConfig.StartTLS {
		cert, err := tls.LoadX509KeyPair(sConfig.PublicKeyFile, sConfig.PrivateKeyFile)
		if err != nil {
			return fmt.Errorf("error while loading the certificate: %s", err)
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.VerifyClientCertIfGiven,
			ServerName:   sConfig.Hostname,
		}
		tlsConfig.Rand = rand.Reader
		s.tlsConfigStore.Store(tlsConfig)
	}
	return nil
}


func (server *SmtpdServer) configureTimeout(t int) {
	server.timeout = time.Duration(int64(t))
}

func RunServer(mainConfig guerrilla.Config, sConfig guerrilla.ServerConfig, backend guerrilla.Backend, bus *evbus.EventBus) (err error) {

	server := newSmtpdServer(mainConfig, sConfig);
	if err := server.configureSSL(); err != nil {
		return err
	}

	bus.Subscribe("config_change:"+sConfig.ListenInterface+":tls_config", func(changedValues guerrilla.ServerConfig) error {
		// TODO store the new changes
		//
		// reload changes for subsequent connections
		if err := server.configureSSL(); err != nil {
			return err
		}
		return nil
	})


	bus.Subscribe("config_change:allowed_hosts", func(mainConfig guerrilla.Config) {
		mainConfig.ResetAllowedHosts()
		server.mainConfigStore.Store(mainConfig)
		log.Infof("Allowed hosts config reloaded")
	})

	bus.Subscribe("config_change:"+sConfig.ListenInterface+":server_config", func(changedValues guerrilla.ServerConfig) {
		// change timeout

		// max size

		// start tls

		// max clients - resize server.sem
	})



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
