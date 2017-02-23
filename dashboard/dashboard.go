package dashboard

import (
	"math/rand"
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"
	_ "github.com/flashmob/go-guerrilla/dashboard/statik"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rakyll/statik/fs"
)

const sessionTimeout = time.Hour * 24 // TODO replace with config

var (
	// Cache of HTML templates
	config   *Config
	sessions map[string]*session
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// TODO below for testing w/ webpack only, change before merging
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Config struct {
	ListenInterface string
}

// Begin collecting data and listening for dashboard clients
func Run(c *Config) {
	statikFS, _ := fs.New()
	config = c
	sessions = map[string]*session{}
	r := mux.NewRouter()
	r.HandleFunc("/ws", webSocketHandler)
	r.PathPrefix("/").Handler(http.FileServer(statikFS))

	rand.Seed(time.Now().UnixNano())

	go dataListener(tickInterval)
	err := http.ListenAndServe(c.ListenInterface, r)
	log.WithError(err).Error("Dashboard server failed to start")
}

func webSocketHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("SID")
	if err != nil {
		// TODO error
		w.WriteHeader(http.StatusInternalServerError)
	}
	sess, sidExists := sessions[cookie.Value]
	if !sidExists {
		// No SID cookie
		sess = startSession(w, r)
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		// TODO Internal error
		return
	}
	sess.ws = conn
	c := make(chan *message)
	sess.send = c

	store.subscribe(sess.id, c)
	go sess.receive()
	go sess.transmit()
	go store.initSession(sess)
}

func startSession(w http.ResponseWriter, r *http.Request) *session {
	sessionID := newSessionID()

	cookie := &http.Cookie{
		Name:  "SID",
		Value: sessionID,
		Path:  "/",
		// Secure: true, // TODO re-add this when TLS is set up
	}

	sess := &session{
		id: sessionID,
	}

	http.SetCookie(w, cookie)
	sessions[sessionID] = sess
	return sess
}
