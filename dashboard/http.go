package dashboard

import (
	"io"
	"net"
	"net/http"
	"time"
)

// ListenAndServeWithClose is a non-blocking listen and serve returning a closer
func ListenAndServeWithClose(addr string, handler http.Handler) (io.Closer, error) {

	var (
		listener  net.Listener
		srvCloser io.Closer
		err       error
	)

	srv := &http.Server{Addr: addr, Handler: handler}

	if addr == "" {
		addr = ":http"
	}

	listener, err = net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	go func() {
		err := srv.Serve(tcpKeepAliveListener{listener.(*net.TCPListener)})
		if err != nil {
			mainlog().Error("HTTP Server Error - ", err)
		}
	}()

	srvCloser = listener
	return srvCloser, nil
}

type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (c net.Conn, err error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}
