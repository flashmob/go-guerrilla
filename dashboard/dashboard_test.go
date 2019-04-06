package dashboard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/gorilla/websocket"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var testlog log.Logger

func init() {
	testlog, _ = log.GetLogger(log.OutputOff.String(), log.InfoLevel.String())
}

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
		Run(config, testlog)
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
		Run(config, testlog)
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
		Run(config, testlog)
		wg.Done()
	}()
	// give Run some time to start
	time.Sleep(time.Second)
	// run test
	simulateEvents(t)
	Stop()
	// Wait for Run() to exit
	wg.Wait()
}

func simulateEvents(t *testing.T) {
	file, err := os.OpenFile("simulation.log", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		panic(err.Error())
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
		if err := os.Remove("simulation.log"); err != nil {
			t.Error(err)
		}
	}()
	reader := bufio.NewReader(file)
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)
	testlog.AddHook(LogHook)
	// match with quotes or without, ie. time="..." or level=
	r := regexp.MustCompile(`(.+?)=("[^"]*"|\S*)\s*`)
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
		testlog.WithFields(fields).Info(msg)
	}
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

// Run a simulation from an already captured log
// Then open a websocket and validate that we are getting some data from it
func TestWebsocket(t *testing.T) {

	config := &Config{
		Enabled:               true,
		ListenInterface:       "127.0.0.1:8082",
		TickInterval:          "1s",
		MaxWindow:             "24h",
		RankingUpdateInterval: "6h",
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		Run(config, testlog)
		wg.Done()
	}()

	var simWg sync.WaitGroup
	go func() {
		simWg.Add(1)
		simulateEvents(t)
		simWg.Done()
	}()

	time.Sleep(time.Second)

	// lets talk to the websocket
	u := url.URL{Scheme: "ws", Host: "127.0.0.1:8082", Path: "/ws"}

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Error("cant connect':", err)
		return
	}

	simWg.Add(1)
	go func() {
		defer func() {
			simWg.Done()
		}()
		i := 0
		for {
			if err := c.SetReadDeadline(time.Now().Add(time.Second + 5)); err != nil {
				t.Error(err)
			}
			_, msg, err := c.ReadMessage()
			s := string(msg)
			_ = s
			if err != nil {
				fmt.Println("socket err:", err)
				t.Error("websocket failed to connect")
				return
			}
			var objmap map[string]*json.RawMessage
			if err := json.Unmarshal(msg, &objmap); err != nil {
				t.Error(err)
			}

			if pl, ok := objmap["payload"]; ok {
				if i == 0 {
					ifr := &initFrame{}
					if err := json.Unmarshal(*pl, &ifr); err != nil {
						t.Error(err, i)
					}

					// initial data frame
				} else {
					df := &dataFrame{}
					if err := json.Unmarshal(*pl, &df); err != nil {
						t.Error(err, i)
					}
					if df.NClients.Y > 10 && len(df.TopHelo) > 10 && len(df.TopDomain) > 10 && len(df.TopIP) > 10 {
						return
					}
				}
			}
			fmt.Println("recv:", string(msg))
			i++
			if i > 2 {
				//t.Error("websocket did get find expected result", i)
				return
			}
		}

	}()
	simWg.Wait() // wait for sim to exit, wait for websocket to finish reading
	Stop()
	// Wait for Run() to exit
	wg.Wait()
	if err := c.Close(); err != nil {
		t.Error(err)
	}

}
