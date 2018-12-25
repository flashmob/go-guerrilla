package backends

import (
	"net"
	"time"
)

func init() {
	RedisDialer = func(network, address string, options ...RedisDialOption) (RedisConn, error) {
		return new(RedisMockConn), nil
	}
}

// RedisConn interface provides a generic way to access Redis via drivers
type RedisConn interface {
	Close() error
	Do(commandName string, args ...interface{}) (reply interface{}, err error)
}

type RedisMockConn struct{}

func (m *RedisMockConn) Close() error {
	return nil
}

func (m *RedisMockConn) Do(commandName string, args ...interface{}) (reply interface{}, err error) {
	Log().Info("redis mock driver command: ", commandName)
	return nil, nil
}

type dialOptions struct {
	readTimeout  time.Duration
	writeTimeout time.Duration
	dial         func(network, addr string) (net.Conn, error)
	db           int
	password     string
}

type RedisDialOption struct {
	f func(*dialOptions)
}

type redisDial func(network, address string, options ...RedisDialOption) (RedisConn, error)

var RedisDialer redisDial
