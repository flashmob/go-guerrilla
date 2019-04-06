package dashboard

import (
	"math/rand"
	"time"

	"github.com/gorilla/websocket"
)

const (
	maxMessageSize = 1024
	writeWait      = 5 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 50 * time.Second
)

var idCharset = []byte("qwertyuiopasdfghjklzxcvbnmQWERTYUIOPASDFGHJKLZXCVBNM1234567890")

// Represents an active session with a client
type session struct {
	id string
	ws *websocket.Conn
	// Messages to send over the websocket are received on this channel
	send <-chan *message
}

// Receives messages from the websocket connection associated with a session
func (s *session) receive() {
	defer s.ws.Close()
	s.ws.SetReadLimit(maxMessageSize)
	s.ws.SetReadDeadline(time.Now().Add(pongWait))
	s.ws.SetPongHandler(func(string) error {
		s.ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := s.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				mainlog().WithError(err).Error("Websocket closed unexpectedly")
			}
			break
		}
		mainlog().Infof("Message: %s", string(message))
	}
}

// Transmits messages to the websocket connection associated with a session
func (s *session) transmit() {
	ticker := time.NewTicker(pingPeriod)
	defer s.ws.Close()
	defer ticker.Stop()

	// Label for loop to allow breaking from within switch statement
transmit:
	for {
		select {
		case p, ok := <-s.send:
			s.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				s.ws.WriteMessage(websocket.CloseMessage, []byte{})
				break transmit
			}

			err := s.ws.WriteJSON(p)
			if err != nil {
				mainlog().WithError(err).Debug("Failed to write next websocket message. Closing connection")
				break transmit
			}
		case <-ticker.C:
			s.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := s.ws.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				mainlog().WithError(err).Debug("Failed to write next websocket message. Closing connection")
				break transmit
			}
		}
	}
}

// Returns a random alphanumeric 10-character ID
func newSessionID() string {
	mask := int64(63)
	gen := rand.Int63()
	out := []byte{}

	for i := 0; i < 10; i++ {
		out = append(out, idCharset[int(gen&mask)%58])
		gen = gen >> 6
	}

	return string(out)
}
