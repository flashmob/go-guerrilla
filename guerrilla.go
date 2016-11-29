package guerrilla

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	log "github.com/Sirupsen/logrus"
)

func Run(ac *AppConfig) {
	for _, sc := range ac.Servers {
		sc.AllowedHosts = ac.AllowedHosts
		go runServer(ac, &sc)
	}
}

func runServer(ac *AppConfig, sc *ServerConfig) error {
	server := Server{
		config: sc,
		sem:    make(chan int, sc.MaxClients),
	}

	if server.config.RequireTLS || server.config.AdvertiseTLS {
		cert, err := tls.LoadX509KeyPair(server.config.PublicKeyFile, server.config.PrivateKeyFile)
		if err != nil {
			return fmt.Errorf("Error loading TLS certificate: %s", err.Error())
		}

		server.tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.VerifyClientCertIfGiven,
			ServerName:   server.config.Hostname,
			Rand:         rand.Reader,
		}
	}

	server.timeout = time.Duration(server.config.Timeout) * time.Second
	listener, err := net.Listen("tcp", server.config.ListenInterface)
	if err != nil {
		return fmt.Errorf("Cannot listen on port: %s", err.Error())
	}

	log.Infof("Listening on TCP %s", server.config.ListenInterface)
	var clientID int64
	clientID = 1
	for {
		log.Debugf("Waiting for a new client. Client ID: %d", clientID)
		conn, err := listener.Accept()
		if err != nil {
			log.WithError(err).Info("Error accepting client")
			continue
		}

		client := &Client{
			conn:        conn,
			address:     conn.RemoteAddr().String(),
			connectedAt: time.Now(),
			bufin:       NewSMTPBufferedReader(conn),
			bufout:      bufio.NewWriter(conn),
			id:          clientID,
		}
		server.sem <- 1
		go server.handleClient(client)
		clientID++
	}
}
