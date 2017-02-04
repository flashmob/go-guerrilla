package dashboard

import (
	"runtime"
	"time"

	log "github.com/Sirupsen/logrus"
)

const (
	tickInterval = time.Second * 5
	maxWindow    = time.Hour * 24
	maxTicks     = int(maxWindow / tickInterval)
)

// Log for sending client events from the server to the dashboard.
var (
	LogHook = logHook(1)
	store   = newDataStore()
)

type dataStore struct {
	// List of samples of RAM usage
	ramTicks []point
	// List of samples of number of connected clients
	nClientTicks []point
	// Up-to-date number of clients
	nClients uint64
	subs     map[string]chan<- *dataFrame
}

func newDataStore() *dataStore {
	subs := make(map[string]chan<- *dataFrame)
	return &dataStore{
		ramTicks:     make([]point, 0, maxTicks),
		nClientTicks: make([]point, 0, maxTicks),
		subs:         subs,
	}
}

func (ds *dataStore) addRAMPoint(p point) {
	if len(ds.ramTicks) == int(maxTicks) {
		ds.ramTicks = append(ds.ramTicks[1:], p)
	} else {
		ds.ramTicks = append(ds.ramTicks, p)
	}
}

func (ds *dataStore) addNClientPoint(p point) {
	if len(ds.nClientTicks) == int(maxTicks) {
		ds.nClientTicks = append(ds.nClientTicks[1:], p)
	} else {
		ds.nClientTicks = append(ds.nClientTicks, p)
	}
}

func (ds *dataStore) subscribe(id string, c chan<- *dataFrame) {
	ds.subs[id] = c
}

func (ds *dataStore) unsubscribe(id string) {
	delete(ds.subs, id)
}

func (ds *dataStore) notify(p *dataFrame) {
	for _, c := range ds.subs {
		select {
		case c <- p:
		default:
		}
	}
}

type point struct {
	X time.Time `json:"x"`
	Y uint64    `json:"y"`
}

func dataListener(interval time.Duration) {
	ticker := time.Tick(interval)
	memStats := &runtime.MemStats{}

	for {
		t := <-ticker
		runtime.ReadMemStats(memStats)
		ramPoint := point{t, memStats.Alloc}
		nClientPoint := point{t, store.nClients}
		log.Info("datastore:89", ramPoint, nClientPoint)
		store.addRAMPoint(ramPoint)
		store.addNClientPoint(nClientPoint)
		store.notify(&dataFrame{
			Ram:      ramPoint,
			NClients: nClientPoint,
		})
	}
}

type dataFrame struct {
	Ram      point `json:"ram"`
	NClients point `json:"n_clients"`
	// top5Helo []string // TODO add for aggregation
	// top5IP   []string
}

type logHook int

func (h logHook) Levels() []log.Level {
	return []log.Level{log.InfoLevel}
}

func (h logHook) Fire(e *log.Entry) error {
	event, ok := e.Data["event"]
	if !ok {
		return nil
	}
	event, ok = event.(string)
	if !ok {
		return nil
	}

	switch event {
	case "connect":
		store.nClients++
	case "disconnect":
		store.nClients--
	}
	return nil
}
