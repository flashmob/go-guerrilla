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
	readTimeout  time.Duration                                //nolint:unused
	writeTimeout time.Duration                                //nolint:unused
	dial         func(network, addr string) (net.Conn, error) //nolint:unused
	db           int                                          //nolint:unused
	password     string                                       //nolint:unused
}

type RedisDialOption struct {
	f func(*dialOptions) //nolint:unused
}

type redisDial func(network, address string, options ...RedisDialOption) (RedisConn, error)

var RedisDialer redisDial
