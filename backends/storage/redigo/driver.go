package redigo_driver

import "github.com/flashmob/go-guerrilla/backends"
import redigo "github.com/gomodule/redigo/redis"

func init() {
	backends.RedisDialer = func(network, address string, options ...backends.RedisDialOption) (backends.RedisConn, error) {
		return redigo.Dial(network, address)
	}
}
