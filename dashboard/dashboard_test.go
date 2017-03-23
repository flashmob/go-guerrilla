package dashboard

import (
	"sync"
	"testing"
)

func TestRunStop(t *testing.T) {

	config := &Config{
		Enabled:               true,
		ListenInterface:       ":8081",
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

	Stop()

	// Wait for Run() to exit
	wg.Wait()

}
