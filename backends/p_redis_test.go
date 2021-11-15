package backends

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisGeneric(t *testing.T) {

	e := mail.NewEnvelope("127.0.0.1", 1)
	e.RcptTo = append(e.RcptTo, mail.Address{User: "test", Host: "grr.la"})

	l, _ := log.GetLogger("./test_redis.log", "debug")
	g, err := New(BackendConfig{
		"save_process":         "Hasher|Redis",
		"redis_interface":      "127.0.0.1:6379",
		"redis_expire_seconds": 7200,
	}, l)
	if err != nil {
		t.Error(err)
		return
	}
	err = g.Start()
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		err := g.Shutdown()
		if err != nil {
			t.Error(err)
		}
	}()
	if gateway, ok := g.(*BackendGateway); ok {
		r := gateway.Process(e)
		if !strings.Contains(r.String(), "250 2.0.0 OK") {
			t.Error("redis processor didn't result with expected result, it said", r)
		}
	}
	// check the log
	if _, err := os.Stat("./test_redis.log"); err != nil {
		t.Error(err)
		return
	}
	b, err := ioutil.ReadFile("./test_redis.log")
	require.NoError(t, err)
	assert.Contains(t, string(b), "SETEX")

	if err := os.Remove("./test_redis.log"); err != nil {
		t.Error(err)
	}

}
