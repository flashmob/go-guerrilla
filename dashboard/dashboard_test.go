package dashboard

import (
	"bufio"
	"os"
	"sync"
	"testing"
	"time"
	//"fmt"
	"github.com/flashmob/go-guerrilla/log"
	"regexp"
	"strings"
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

// Run a simulation from an already captured log
func TestSimulationRun(t *testing.T) {

	config := &Config{
		Enabled:               true,
		ListenInterface:       ":8082",
		TickInterval:          "1s",
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

	file, err := os.Open("simulation.log")

	if err != nil {
		panic(err.Error())
	}

	defer file.Close()

	reader := bufio.NewReader(file)
	scanner := bufio.NewScanner(reader)

	scanner.Split(bufio.ScanLines)

	l, _ := log.GetLogger("stderr", "info")
	l.AddHook(LogHook)

	// match with quotes or without, ie. time="..." or level=
	r := regexp.MustCompile(`(.+?)=("[^"]*"|\S*)\s*`)
	c := 0
	simStart := time.Now()
	var start time.Time
	for scanner.Scan() {
		fields := map[string]interface{}{}
		line := scanner.Text()
		items := r.FindAllString(line, -1)
		msg := ""
		var logElapsed time.Duration
		for i := range items {
			key, val := parseItem(items[i])
			//fmt.Println(key, val)
			if key != "time" && key != "level" && key != "msg" {
				fields[key] = val
			}
			if key == "msg" {
				msg = val
			}
			if key == "time" {
				tv, err := time.Parse(time.RFC3339, val)
				if err != nil {
					t.Error("invalid time", tv)
				}
				if start.IsZero() {
					start = tv
				}
				fields["start"] = start
				logElapsed = tv.Sub(start)
			}

		}

		diff := time.Now().Sub(simStart) - logElapsed
		time.Sleep(diff)              // wait so that we don't go too fast
		simStart = simStart.Add(diff) // catch up

		l.WithFields(fields).Info(msg)

		c++
		if c > 5000 {
			break
		}

	}

	Stop()

	// Wait for Run() to exit
	wg.Wait()

}

// parseItem parses a log item, eg time="2017-03-24T11:55:44+11:00" will be:
// key = time and val will be 2017-03-24T11:55:44+11:00
func parseItem(item string) (key string, val string) {
	arr := strings.Split(item, "=")
	if len(arr) == 2 {
		key = arr[0]
		if arr[1][0:1] == "\"" {
			pos := len(arr[1]) - 2
			val = arr[1][1:pos]
		} else {
			val = arr[1]
		}
	}
	val = strings.TrimSpace(val)
	return
}
