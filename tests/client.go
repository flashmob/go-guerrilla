package test

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/flashmob/go-guerrilla"
	"net"
	"time"
)

func Connect(serverConfig guerrilla.ServerConfig, deadline time.Duration) (net.Conn, *bufio.Reader, error) {
	var bufin *bufio.Reader
	var conn net.Conn
	var err error
	if serverConfig.TLSAlwaysOn {
		// start tls automatically
		conn, err = tls.Dial("tcp", serverConfig.ListenInterface, &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         "127.0.0.1",
		})
	} else {
		conn, err = net.Dial("tcp", serverConfig.ListenInterface)
	}

	if err != nil {
		// handle error
		//t.Error("Cannot dial server", config.Servers[0].ListenInterface)
		return conn, bufin, errors.New("Cannot dial server: " + serverConfig.ListenInterface + "," + err.Error())
	}
	bufin = bufio.NewReader(conn)

	// should be ample time to complete the test
	conn.SetDeadline(time.Now().Add(time.Duration(time.Second * deadline)))
	// read greeting, ignore it
	_, err = bufin.ReadString('\n')
	return conn, bufin, err
}

func Command(conn net.Conn, bufin *bufio.Reader, command string) (reply string, err error) {
	_, err = fmt.Fprintln(conn, command+"\r")
	if err == nil {
		return bufin.ReadString('\n')
	}
	return "", err
}
