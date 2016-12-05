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
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	evbus "github.com/asaskevich/EventBus"

	log "github.com/Sirupsen/logrus"

	guerrilla "github.com/flashmob/go-guerrilla"
	"reflect"
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

// Set the timeout for the server and all clients
func (server *SmtpdServer) setTimeout(seconds int) {
	duration := time.Duration(int64(seconds))
	server.clientPool.SetTimeout(duration)
	server.timeout.Store(duration)
}


func RunServer(mainConfig guerrilla.Config,
	sConfig guerrilla.ServerConfig,
	backend guerrilla.Backend,
	bus *evbus.EventBus) (err error) {

	server := newSmtpdServer(mainConfig, sConfig, bus)

	handlerAllowedHosts := func(newConfig guerrilla.Config) {
		newConfig.ResetAllowedHosts()
		server.mainConfigStore.Store(newConfig)
		log.Infof("Allowed hosts config reloaded")
	}
	bus.Subscribe("config_change:allowed_hosts", handlerAllowedHosts)

	handlerTimeout := func(newServerConfig *guerrilla.ServerConfig) {
		server.setTimeout(newServerConfig.Timeout)
		log.Infof("timeout changed")
	}
	bus.Subscribe("config_change:"+sConfig.ListenInterface+":timeout", handlerTimeout)
	// Configure TLS
	if err := server.configureSSL(); err != nil {
		return err
	}
	// Re-configure TLS
	handlerTLS := func(newServerConfig guerrilla.ServerConfig) error {
		// reload changes for subsequent connections
		server.configStore.Store(newServerConfig)
		if err := server.configureSSL(); err != nil {
			return err
		}
		log.Infof("SSL reconfigured for [%s]", sConfig.ListenInterface)
		return nil
	}
	bus.Subscribe("config_change:"+sConfig.ListenInterface+":tls_config", handlerTLS)

	var (
		listener     net.Listener
		listener_err error
	)

	// Start listening for SMTP connections
	listener, err = net.Listen("tcp", sConfig.ListenInterface)
	if listener_err != nil {
		return fmt.Errorf("cannot listen on port, %v (%s)", listener_err, reflect.TypeOf(listener_err))
	}
	log.Infof("Listening on tcp %s", sConfig.ListenInterface)

	// Stop listening for new connections on a stop_server event
	handlerStopServer := func() {
		if listener != nil {
			log.Infof("Close listening on interface: %s", sConfig.ListenInterface)
			listener.Close()
			// subsequent calls to listener.Accept() will produce a
			// non-temporary error, then this will function will return.
			// Not that each individual connections will also receive this event and close themselves
		}
	}
	bus.Subscribe("config_change:"+sConfig.ListenInterface+":stop_server", handlerStopServer)
	defer func() {
		if err := bus.Unsubscribe("config_change:"+sConfig.ListenInterface+":stop_server", handlerStopServer); err != nil {
			log.WithError(err).Debug("bus unsub error")
		}
		if err := bus.Unsubscribe("config_change:"+sConfig.ListenInterface+":tls_config", handlerTLS);err != nil {
			log.WithError(err).Debug("bus unsub error")
		}
		if err := bus.Unsubscribe("config_change:"+sConfig.ListenInterface+":timeout", handlerTimeout); err != nil {
			log.WithError(err).Debug("bus unsub error")
		}
		if err := bus.Unsubscribe("config_change:allowed_hosts", handlerAllowedHosts); err != nil {
			log.WithError(err).Debug("bus unsub error")
		}
	}()

	var (
		clientID uint64
		client *guerrilla.Client
		poolErr error
	)
	clientID = 1
	for {
		conn, err := listener.Accept()
		if err != nil {
			if e, ok := err.(net.Error); ok && !e.Temporary() {
				// most likely the socket has been closed (stop_server event caught)
				// send an event to wait for existing connections to close
				server.Shutdown()
				log.Infof("Server [%s] has stopped", sConfig.ListenInterface)
				return nil
			}
			log.WithError(err).Infof("Accept error (%s) ", reflect.TypeOf(err))
			continue
		}
		// grab a client form the pool, will block if full
		client, poolErr = server.clientPool.Borrow(conn, clientID);
		if poolErr == ErrPoolShuttingDown {
			continue
		}
		go func () {
			server.handleClient(client, backend)
			server.clientPool.Return(client)
		}()
		clientID++
	}
	return nil
}
