package backends

import (
	"fmt"

	"github.com/flashmob/go-guerrilla/envelope"

	"github.com/flashmob/go-guerrilla/response"
	"github.com/garyburd/redigo/redis"
)

func init() {
	Processors["redis"] = func() Decorator {
		return Redis()
	}
}

type RedisProcessorConfig struct {
	RedisExpireSeconds int    `json:"redis_expire_seconds"`
	RedisInterface     string `json:"redis_interface"`
}

type RedisProcessor struct {
	isConnected bool
	conn        redis.Conn
}

func (r *RedisProcessor) redisConnection(redisInterface string) (err error) {
	if r.isConnected == false {
		r.conn, err = redis.Dial("tcp", redisInterface)
		if err != nil {
			// handle error
			return err
		}
		r.isConnected = true
	}
	return nil
}

// The redis decorator stores the email data in redis

func Redis() Decorator {

	var config *RedisProcessorConfig
	redisClient := &RedisProcessor{}
	// read the config into RedisProcessorConfig
	Service.AddInitializer(Initialize(func(backendConfig BackendConfig) error {
		configType := baseConfig(&RedisProcessorConfig{})
		bcfg, err := Service.extractConfig(backendConfig, configType)
		if err != nil {
			return err
		}
		config = bcfg.(*RedisProcessorConfig)
		if redisErr := redisClient.redisConnection(config.RedisInterface); redisErr != nil {
			err := fmt.Errorf("Redis cannot connect, check your settings: %s", redisErr)
			return err
		}
		return nil
	}))
	// When shutting down
	Service.AddShutdowner(Shutdown(func() error {
		if redisClient.isConnected {
			redisClient.conn.Close()
		}
		return nil
	}))

	var redisErr error

	return func(c Processor) Processor {
		return ProcessorFunc(func(e *envelope.Envelope) (BackendResult, error) {
			hash := ""
			if len(e.Hashes) > 0 {
				hash = e.Hashes[0]

				var cData *compressor
				// a compressor was set
				if c, ok := e.Info["zlib-compressor"]; ok {
					cData = c.(*compressor)
				}

				redisErr = redisClient.redisConnection(config.RedisInterface)
				if redisErr == nil {
					if cData != nil {
						// send data is using the compressor
						_, doErr := redisClient.conn.Do("SETEX", hash, config.RedisExpireSeconds, cData)
						if doErr != nil {
							redisErr = doErr
						}
					} else {
						// not using compressor
						_, doErr := redisClient.conn.Do("SETEX", hash, config.RedisExpireSeconds, e.Data.String())
						if doErr != nil {
							redisErr = doErr
						}
					}
				}
				if redisErr != nil {
					mainlog.WithError(redisErr).Warn("Error while talking to redis")
					result := NewBackendResult(response.Canned.FailBackendTransaction)
					return result, redisErr
				} else {
					e.Info["redis"] = "redis" // the backend system will know to look in redis for the message data
				}
			} else {
				mainlog.Error("Redis needs a Hash() process before it")
			}

			return c.Process(e)
		})
	}
}
