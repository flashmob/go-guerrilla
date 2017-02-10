package backends

import (
	"github.com/flashmob/go-guerrilla/envelope"

	"github.com/flashmob/go-guerrilla/response"
)

type RedisProcessorConfig struct {
	RedisExpireSeconds int    `json:"redis_expire_seconds"`
	RedisInterface     string `json:"redis_interface"`
}

// The redis decorator stores the email data in redis

func Redis(dc *DecoratorCallbacks) Decorator {

	var config *RedisProcessorConfig
	redisClient := &redisClient{}
	dc.loader = func(backendConfig BackendConfig) error {
		configType := baseConfig(&RedisProcessorConfig{})
		bcfg, err := ab.extractConfig(backendConfig, configType)
		if err != nil {
			return err
		}
		config = bcfg.(*RedisProcessorConfig)

		return nil
	}
	var redisErr error

	return func(c Processor) Processor {
		return ProcessorFunc(func(e *envelope.Envelope) (BackendResult, error) {
			hash := ""
			if len(e.Hashes) > 0 {
				hash = e.Hashes[0]

				var cData *compressor
				// a compressor was set
				if e.Meta != nil {
					if c, ok := e.Meta["zlib-compressor"]; ok {
						cData = c.(*compressor)
					}
				} else {
					e.Meta = make(map[string]interface{})
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
					e.Meta["redis"] = "redis" // the backend system will know to look in redis for the message data
				}
			} else {
				mainlog.Error("Redis needs a Hash() process before it")
			}

			return c.Process(e)
		})
	}
}
