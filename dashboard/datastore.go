package dashboard

import (
	"runtime"
	"sync"
	"time"
)

const (
	// Number of entries to show in top N charts
	topClientsSize = 5
	// Redux action type names
	initMessageType = "INIT"
	tickMessageType = "TICK"
)

var (
	tickInterval          = time.Second * 5
	maxWindow             = time.Hour * 24
	rankingUpdateInterval = time.Hour * 6
	uniqueHeloRatioMax    = 0.8
	maxTicks              = int(maxWindow / tickInterval)
	nRankingBuffers       = int(maxWindow / rankingUpdateInterval)
	LogHook               = logHook(1)
	store                 = newDataStore()
)

// Keeps track of connection data that is buffered in the topClients
// so the data can be removed after `maxWindow` interval has occurred.
type conn struct {
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
	// Total number of clients in the current aggregation buffer
	nClientsInBuffer uint64
	topDomain        bufferedRanking
	topHelo          bufferedRanking
	topIP            bufferedRanking
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
		topDomain:    newBufferedRanking(nRankingBuffers),
		topHelo:      newBufferedRanking(nRankingBuffers),
		topIP:        newBufferedRanking(nRankingBuffers),
		newConns:     newConns,
		subs:         subs,
	}

	return ds
}

// Keeps track of top domain/helo/ip rankings, but buffered into multiple
// maps so that old records can be efficiently kept track of and quickly removed
type bufferedRanking []map[string]int

func newBufferedRanking(nBuffers int) bufferedRanking {
	br := make([]map[string]int, nBuffers)
	for i := 0; i < nBuffers; i++ {
		br[i] = make(map[string]int)
	}
	return br
}

// Manages the list of top clients by domain, helo, and IP by updating buffered
// record maps. At each `rankingUpdateInterval` we shift the maps and remove the
// oldest, so rankings are always at most as old as `maxWindow`
func (ds *dataStore) rankingManager() {
	ticker := time.NewTicker(rankingUpdateInterval)

	for {
		select {
		case c := <-ds.newConns:
			nHelos := len(ds.topHelo)
			if nHelos > 5 &&
				float64(nHelos)/float64(ds.nClientsInBuffer) > uniqueHeloRatioMax {
				// If too many unique HELO messages are detected as a ratio to the total
				// number of clients, quit collecting data until we roll over into the next
				// aggregation buffer.
				continue
			}
			ds.lock.Lock()
			ds.nClientsInBuffer++
			ds.topDomain[0][c.domain]++
			ds.topHelo[0][c.helo]++
			ds.topIP[0][c.ip]++
			ds.lock.Unlock()

		case <-ticker.C:
			ds.lock.Lock()
			// Add empty map at index 0 and shift other maps one down
			ds.nClientsInBuffer = 0
			ds.topDomain = append(
				[]map[string]int{map[string]int{}},
				ds.topDomain[:len(ds.topDomain)-1]...)
			ds.topHelo = append(
				[]map[string]int{map[string]int{}},
				ds.topHelo[:len(ds.topHelo)-1]...)
			ds.topIP = append(
				[]map[string]int{map[string]int{}},
				ds.topHelo[:len(ds.topIP)-1]...)
			ds.lock.Unlock()

		case <-stopRankingManager:
			return
		}
	}
}

// Aggregates the rankings from the ranking buffer into a single map
// for each of domain, helo, ip. This is what we send to the frontend.
func (ds *dataStore) aggregateRankings() ranking {
	topDomain := make(map[string]int, len(ds.topDomain[0]))
	topHelo := make(map[string]int, len(ds.topHelo[0]))
	topIP := make(map[string]int, len(ds.topIP[0]))

	ds.lock.Lock()
	// Aggregate buffers
	for i := 0; i < nRankingBuffers; i++ {
		for domain, count := range ds.topDomain[i] {
			topDomain[domain] += count
		}
		for helo, count := range ds.topHelo[i] {
			topHelo[helo] += count
		}
		for ip, count := range ds.topIP[i] {
			topIP[ip] += count
		}
	}
	ds.lock.Unlock()

	return ranking{
		TopDomain: topDomain,
		TopHelo:   topHelo,
		TopIP:     topIP,
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
		select {
		case t := <-ticker:
			runtime.ReadMemStats(memStats)
			ramPoint := point{t, memStats.Alloc}
			nClientPoint := point{t, store.nClients}
			mainlog().WithFields(map[string]interface{}{
				"ram":     ramPoint.Y,
				"clients": nClientPoint.Y,
			}).Info("Logging analytics data")

			store.addRAMPoint(ramPoint)
			store.addNClientPoint(nClientPoint)
			store.notify(&message{tickMessageType, dataFrame{
				Ram:      ramPoint,
				NClients: nClientPoint,
				ranking:  store.aggregateRankings(),
			}})
		case <-stopDataListener:
			return
		}

	}
}

// Keeps track of top clients by helo, ip, and domain
type ranking struct {
	TopHelo   map[string]int `json:"topHelo"`
	TopIP     map[string]int `json:"topIP"`
	TopDomain map[string]int `json:"topDomain"`
}

type dataFrame struct {
	Ram      point `json:"ram"`
	NClients point `json:"nClients"`
	ranking
}

type initFrame struct {
	Ram      []point `json:"ram"`
	NClients []point `json:"nClients"`
	ranking
}

// Format of messages to be sent over WebSocket
type message struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}
