package dashboard

import (
	"html/template"
	"math/rand"
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"
	_ "github.com/flashmob/go-guerrilla/dashboard/statik"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rakyll/statik/fs"
)

const (
	dashboard      = "index.html"
	dashboardPath  = "dashboard/html/index.html"
	sessionTimeout = time.Hour * 24 // TODO replace with config
)

var (
	// Cache of HTML templates
	templates = template.Must(template.ParseFiles(dashboardPath))
	config    *Config
	sessions  map[string]*session
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Config struct {
	ListenInterface string
}

func Run(c *Config) {
	statikFS, _ := fs.New()
	config = c
	sessions = map[string]*session{}
	r := mux.NewRouter()
	r.HandleFunc("/ws", webSocketHandler)
	r.PathPrefix("/").Handler(http.FileServer(statikFS))

	rand.Seed(time.Now().UnixNano())

	go dataListener(tickInterval)

	http.ListenAndServe(c.ListenInterface, r)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("SID")
	_, sidExists := sessions[c.Value]
	if err != nil || !sidExists {
		// No SID cookie
		startSession(w, r)
	}
	w.WriteHeader(http.StatusOK)
	templates.ExecuteTemplate(w, dashboard, nil)
}

func webSocketHandler(w http.ResponseWriter, r *http.Request) {
	log.Info("dashboard:112")
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
	c := make(chan *dataFrame)
	sess.send = c
	// TODO send store contents at connection time
	store.subscribe(sess.id, c)
	go sess.receive()
	go sess.transmit()
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

func getSession(r *http.Request) *session {
	c, err := r.Cookie("SID")
	if err != nil {
		return nil
	}

	sid := c.Value
	sess, ok := sessions[sid]
	if !ok {
		return nil
	}

	return sess
}
