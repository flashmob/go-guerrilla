package dashboard

import (
	"runtime"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

const (
	// Frequency at which we measure stats and send messages to clients
	tickInterval = time.Second * 5
	// Maximum history of stats kept in store and displayed on frontend
	maxWindow = time.Hour * 24
	maxTicks  = int(maxWindow / tickInterval)
	// Number of entries to show in top N charts
	topClientsSize = 5
	// Frequency at which we update top client rankings
	topClientsUpdateInterval = time.Minute * 5
	// Redux action type names
	initMessageType = "INIT"
	tickMessageType = "TICK"
)

var (
	// Log for sending client events from the server to the dashboard.
	LogHook = logHook(1)
	store   = newDataStore()
)

// Keeps track of connection data that is buffered in the topClients
// so the data can be removed after `maxWindow` interval has occurred.
type conn struct {
	addedTime        int64
	helo, domain, ip string
}

type dataStore struct {
	lock sync.Mutex
	// List of samples of RAM usage
	ramTicks []point
	// List of samples of number of connected clients
	nClientTicks []point
	// Up-to-date number of clients
	nClients uint64
	topClients
	// For notifying the store about new connections
	newConns chan conn
	subs     map[string]chan<- *message
}

func newDataStore() *dataStore {
	newConns := make(chan conn, 64)
	subs := make(map[string]chan<- *message)
	ds := &dataStore{
		ramTicks:     make([]point, 0, maxTicks),
		nClientTicks: make([]point, 0, maxTicks),
		topClients: topClients{
			TopDomain: make(map[string]int),
			TopHelo:   make(map[string]int),
			TopIP:     make(map[string]int),
		},
		newConns: newConns,
		subs:     subs,
	}
	go ds.topClientsManager()
	return ds
}

// Manages the list of top clients by domain, helo, and IP by incrementing
// records upon a new connection and scheduling a decrement after the `maxWindow`
// interval has passed.
func (ds *dataStore) topClientsManager() {
	bufferedConns := []conn{}
	ticker := time.NewTicker(topClientsUpdateInterval)

	for {
		select {
		case c := <-ds.newConns:
			bufferedConns = append(bufferedConns, c)

			ds.lock.Lock()
			ds.TopDomain[c.domain]++
			ds.TopHelo[c.helo]++
			ds.TopIP[c.ip]++
			ds.lock.Unlock()

		case <-ticker.C:
			cutoff := time.Now().Add(-maxWindow).Unix()
			cutoffI := 0

			ds.lock.Lock()
			for i, bc := range bufferedConns {
				// We make an assumption here that conns come in in-order, which probably
				// isn't exactly true, but close enough to not make much of a difference
				if bc.addedTime > cutoff {
					cutoffI = i
					break
				}
				ds.TopDomain[bc.domain]--
				ds.TopHelo[bc.helo]--
				ds.TopIP[bc.ip]--
			}
			ds.lock.Unlock()

			bufferedConns = bufferedConns[cutoffI:]
		}
	}
}

// Adds a new ram point, removing old points if necessary
func (ds *dataStore) addRAMPoint(p point) {
	if len(ds.ramTicks) == int(maxTicks) {
		ds.ramTicks = append(ds.ramTicks[1:], p)
	} else {
		ds.ramTicks = append(ds.ramTicks, p)
	}
}

// Adds a new nClients point, removing old points if necessary
func (ds *dataStore) addNClientPoint(p point) {
	if len(ds.nClientTicks) == int(maxTicks) {
		ds.nClientTicks = append(ds.nClientTicks[1:], p)
	} else {
		ds.nClientTicks = append(ds.nClientTicks, p)
	}
}

func (ds *dataStore) subscribe(id string, c chan<- *message) {
	ds.subs[id] = c
}

func (ds *dataStore) unsubscribe(id string) {
	delete(ds.subs, id)
}

func (ds *dataStore) notify(m *message) {
	// Prevent concurrent read/write to maps in the store
	ds.lock.Lock()
	defer ds.lock.Unlock()
	for _, c := range ds.subs {
		select {
		case c <- m:
		default:
		}
	}
}

// Initiates a session with all historic data in the store
func (ds *dataStore) initSession(sess *session) {
	store.subs[sess.id] <- &message{initMessageType, initFrame{
		Ram:      store.ramTicks,
		NClients: store.nClientTicks,
	}}
}

type point struct {
	X time.Time `json:"x"`
	Y uint64    `json:"y"`
}

// Measures RAM and number of connected clients and sends a tick
// message to all connected clients on the given interval
func dataListener(interval time.Duration) {
	ticker := time.Tick(interval)
	memStats := &runtime.MemStats{}

	for {
		t := <-ticker
		runtime.ReadMemStats(memStats)
		ramPoint := point{t, memStats.Alloc}
		nClientPoint := point{t, store.nClients}
		log.WithFields(map[string]interface{}{
			"ram":     ramPoint.Y,
			"clients": nClientPoint.Y,
		}).Info("Logging analytics data")

		store.addRAMPoint(ramPoint)
		store.addNClientPoint(nClientPoint)
		store.notify(&message{tickMessageType, dataFrame{
			Ram:        ramPoint,
			NClients:   nClientPoint,
			topClients: store.topClients,
		}})
	}
}

// Keeps track of top clients by helo, ip, and domain
type topClients struct {
	TopHelo   map[string]int `json:"topHelo"`
	TopIP     map[string]int `json:"topIP"`
	TopDomain map[string]int `json:"topDomain"`
}

type dataFrame struct {
	Ram      point `json:"ram"`
	NClients point `json:"nClients"`
	topClients
}

type initFrame struct {
	Ram      []point `json:"ram"`
	NClients []point `json:"nClients"`
	topClients
}

// Format of messages to be sent over WebSocket
type message struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type logHook int

func (h logHook) Levels() []log.Level {
	return log.AllLevels
}

// Checks fired logs for information that is relevant to the dashboard
func (h logHook) Fire(e *log.Entry) error {
	event, ok := e.Data["event"].(string)
	if !ok {
		return nil
	}

	var helo, ip, domain string
	if event == "mailfrom" {
		helo, ok = e.Data["helo"].(string)
		if !ok {
			return nil
		}
		ip, ok = e.Data["address"].(string)
		if !ok {
			return nil
		}
		domain, ok = e.Data["domain"].(string)
		if !ok {
			return nil
		}
	}

	switch event {
	case "connect":
		store.lock.Lock()
		store.nClients++
		store.lock.Unlock()
	case "mailfrom":
		store.newConns <- conn{
			addedTime: time.Now().Unix(),
			domain:    domain,
			helo:      helo,
			ip:        ip,
		}
	case "disconnect":
		store.lock.Lock()
		log.Info("datastore:251, disconnecting", store.nClients)
		store.nClients--
		store.lock.Unlock()
	}
	return nil
}
