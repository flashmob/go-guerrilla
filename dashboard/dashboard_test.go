package dashboard

import (
	"sync"
	"testing"
	"time"
)

func TestRunStop(t *testing.T) {

	config := &Config{
		Enabled:               true,
		ListenInterface:       ":8082",
		TickInterval:          "5s",
		MaxWindow:             "24h",
		RankingUpdateInterval: "6h",
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		Run(config)
		wg.Done()
	}()
	// give Run some time to start
	time.Sleep(time.Second)

	Stop()

	// Wait for Run() to exit
	wg.Wait()

}

// Test if starting with a bad interface address
func TestRunStopBadAddress(t *testing.T) {

	config := &Config{
		Enabled:               true,
		ListenInterface:       "1.1.1.1:0",
		TickInterval:          "5s",
		MaxWindow:             "24h",
		RankingUpdateInterval: "6h",
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		Run(config)
		wg.Done()
	}()

	time.Sleep(time.Second * 2)

	Stop()

	// Wait for Run() to exit
	wg.Wait()

}
