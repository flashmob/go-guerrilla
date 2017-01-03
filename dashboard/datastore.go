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
	subs []chan<- *point
}

func newDataStore() *dataStore {
	return &dataStore{
		ram: make([]*point, 0, maxTicks),
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

func (ds *dataStore) subscribe(c chan<- *point) {
	ds.subs = append(ds.subs, c)
}

func (ds *dataStore) notify(p *point) {
	var toUnsubscribe []int
	for i, c := range ds.subs {
		select {
		case c <- p:
		default:
			close(c)
			toUnsubscribe = append(toUnsubscribe, i)
		}
	}

	if len(toUnsubscribe) > 0 {
		newSubs := ds.subs[:0]
		for i, c := range ds.subs {
			if i != toUnsubscribe[0] {
				newSubs = append(newSubs, c)
			} else {
				toUnsubscribe = toUnsubscribe[1:]
				if len(toUnsubscribe) == 0 {
					break
				}
			}
		}
		ds.subs = newSubs
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
