package dashboard

import (
	"html/template"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
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
	config = c
	sessions = map[string]*session{}
	r := mux.NewRouter()
	r.HandleFunc("/", indexHandler)
	r.HandleFunc("/ws", webSocketHandler)

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

// func loginHandler(w http.ResponseWriter, r *http.Request) {
// 	switch r.Method {
// 	case "GET":
// 		if isLoggedIn(r) {
// 			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
// 		} else {
// 			templates.ExecuteTemplate(w, login, nil)
// 		}
//
// 	case "POST":
// 		user := r.FormValue("username")
// 		pass := r.FormValue("password")
//
// 		if user == config.Username && pass == config.Password {
// 			err := startSession(w, r)
// 			if err != nil {
// 				w.WriteHeader(http.StatusInternalServerError)
// 				// TODO Internal error
// 				return
// 			}
// 			http.Redirect(w, r, "/", http.StatusSeeOther)
// 		} else {
// 			templates.ExecuteTemplate(w, login, nil) // TODO info about failed login
// 		}
//
// 	default:
// 		w.WriteHeader(http.StatusMethodNotAllowed)
// 	}
// }
//
// func logoutHandler(w http.ResponseWriter, r *http.Request) {
// 	switch r.Method {
// 	case "POST":
// 		sess := getSession(r)
// 		if sess == nil {
// 			w.WriteHeader(http.StatusForbidden)
// 			return
// 		}
//
// 		store.unsubscribe(sess.id)
// 		sess.expires = time.Now()
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
//
// 	default:
// 		w.WriteHeader(http.StatusMethodNotAllowed)
// 	}
// }

func webSocketHandler(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	if sess == nil {
		w.WriteHeader(http.StatusInternalServerError)
		// TODO Internal error
		return
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

func startSession(w http.ResponseWriter, r *http.Request) {
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
