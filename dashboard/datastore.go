package dashboard

import (
	"runtime"
	"time"
)

const (
	tickInterval = time.Second
	maxWindow    = time.Hour * 24
	maxTicks     = int(maxWindow / tickInterval)
)

type dataStore struct {
	ram  []*point
	subs map[string]chan<- *point
}

func newDataStore() *dataStore {
	return &dataStore{
		ram:  make([]*point, 0, maxTicks),
		subs: make(map[string]chan<- *point),
	}
}

func (ds *dataStore) addPoint(p *point) {
	if len(ds.ram) == int(maxTicks) {
		ds.ram = append(ds.ram[1:], p)
	} else {
		ds.ram = append(ds.ram, p)
	}
	ds.notify(p)
}

func (ds *dataStore) subscribe(id string, c chan<- *point) {
	ds.subs[id] = c
}

func (ds *dataStore) unsubscribe(id string) {
	delete(ds.subs, id)
}

func (ds *dataStore) notify(p *point) {
	for _, c := range ds.subs {
		select {
		case c <- p:
		default:
		}
	}
}

type point struct {
	T time.Time `json:"t"`
	Y uint64    `json:"y"`
}

func ramListener(interval time.Duration, store *dataStore) {
	ticker := time.Tick(interval)
	memStats := &runtime.MemStats{}

	for {
		t := <-ticker
		runtime.ReadMemStats(memStats)
		store.addPoint(&point{t, memStats.Alloc})
	}
}
