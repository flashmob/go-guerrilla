package dashboard

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"
	// "github.com/flashmob/go-guerrilla/dashboard/statik"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rakyll/statik/fs"
	"sync"
)

var (
	config   *Config
	sessions map[string]*session

	stopRankingManager chan bool = make(chan bool)
	stopDataListener   chan bool = make(chan bool)
	stopHttp           chan bool = make(chan bool)

	wg      sync.WaitGroup
	started sync.WaitGroup

	s state
)

type state int

const (
	stateStopped state = iota
	stateRunning
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// TODO below for testing w/ webpack only, change before merging
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Config struct {
	Enabled         bool   `json:"is_enabled"`
	ListenInterface string `json:"listen_interface"`
	// Interval at which we send measure and send dataframe to frontend
	TickInterval string `json:"tick_interval"`
	// Maximum interval for which we store data
	MaxWindow string `json:"max_window"`
	// Granularity for which rankings are aggregated
	RankingUpdateInterval string `json:"ranking_aggregation_interval"`
	// Determines at which ratio of unique HELOs to unique connections we
	// will stop collecting data to prevent memory exhaustion attack.
	// Number between 0-1, set to >1 if you never want to stop collecting data.
	// Default is 0.8
	UniqueHeloRatioMax float64 `json:"unique_helo_ratio"`
}

// Begin collecting data and listening for dashboard clients
func Run(c *Config) {
	statikFS, _ := fs.New()

	applyConfig(c)
	sessions = map[string]*session{}

	r := mux.NewRouter()
	r.HandleFunc("/ws", webSocketHandler)
	r.PathPrefix("/").Handler(http.FileServer(statikFS))

	rand.Seed(time.Now().UnixNano())

	started.Add(1)
	defer func() {
		s = stateStopped

	}()

	closer, err := ListenAndServeWithClose(c.ListenInterface, r)
	if err != nil {
		log.WithError(err).Error("Dashboard server failed to start")
		started.Done()
		return
	}
	log.Infof("started dashboard, listening on http [%s]", c.ListenInterface)
	wg.Add(1)

	go func() {
		wg.Add(1)
		dataListener(tickInterval)
		wg.Done()
	}()
	go func() {
		wg.Add(1)
		store.rankingManager()
		wg.Done()
	}()

	s = stateRunning
	started.Done()

	select {
	case <-stopHttp:
		closer.Close()
		wg.Done()
		return
	}
}

func Stop() {
	started.Wait()
	if s == stateRunning {
		stopDataListener <- true
		stopRankingManager <- true
		stopHttp <- true
		wg.Wait()
	}

}

// Parses options in config and applies to global variables
func applyConfig(c *Config) {
	config = c

	if len(config.MaxWindow) > 0 {
		mw, err := time.ParseDuration(config.MaxWindow)
		if err == nil {
			maxWindow = mw
		}
	}
	if len(config.RankingUpdateInterval) > 0 {
		rui, err := time.ParseDuration(config.RankingUpdateInterval)
		if err == nil {
			rankingUpdateInterval = rui
		}
	}
	if len(config.TickInterval) > 0 {
		ti, err := time.ParseDuration(config.TickInterval)
		if err == nil {
			tickInterval = ti
		}
	}
	if config.UniqueHeloRatioMax > 0 {
		uniqueHeloRatioMax = config.UniqueHeloRatioMax
	}

	maxTicks = int(maxWindow * tickInterval)
	nRankingBuffers = int(maxWindow / rankingUpdateInterval)
}

func webSocketHandler(w http.ResponseWriter, r *http.Request) {
	var sess *session
	cookie, err := r.Cookie("SID")
	fmt.Println("cookie", cookie, err.Error())
	if err != nil {
		// Haven't set this cookie yet.
		sess = startSession(w, r)
	} else {
		var sidExists bool
		sess, sidExists = sessions[cookie.Value]
		if !sidExists {
			// No SID cookie in our store, start a new session
			sess = startSession(w, r)
		}
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
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
