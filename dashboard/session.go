package dashboard

import (
	"math/rand"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/websocket"
)

const (
	maxMessageSize = 1024
	writeWait      = 5 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 50 * time.Second
)

var idCharset = []byte("qwertyuiopasdfghjklzxcvbnmQWERTYUIOPASDFGHJKLZXCVBNM1234567890")

type session struct {
	start, expires time.Time
	id             string
	// Whether we have a valid
	alive bool
	ws    *websocket.Conn
	// Messages to send over the websocket are received on this channel
	send <-chan *point
}

func (s *session) valid() bool {
	return s.expires.After(time.Now())
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
				log.WithError(err).Error("Websocket closed unexpectedly")
			}
			break
		}
		log.Infof("Message: %s", string(message))
	}
}

// Transmits messages to the websocket connection associated with a session
func (s *session) transmit() {
	ticker := time.NewTicker(pingPeriod)
	defer s.ws.Close()
	defer ticker.Stop()

	for {
		select {
		case p, ok := <-s.send:
			s.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok || !s.valid() {
				s.ws.WriteMessage(websocket.CloseMessage, []byte{})
				break
			}

			err := s.ws.WriteJSON(p)
			if err != nil {
				log.WithError(err).Debug("Failed to write next websocket message. Closing connection")
				break
			}
		case <-ticker.C:
			s.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := s.ws.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				log.WithError(err).Debug("Failed to write next websocket message. Closing connection")
				break
			}
		}
	}
}

type sessionStore map[string]*session

// Remove expired sessions
func (ss sessionStore) clean() {
	now := time.Now()
	for id, sess := range ss {
		if sess.expires.Before(now) {
			delete(ss, id)
		}
	}
}

// Cleans the store on each tick
func (ss sessionStore) cleaner(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		<-ticker.C
		ss.clean()
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
