package dedupe

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/cenkalti/backoff/v4"
	"github.com/rileyafox/solana-sentinel/internal/metrics"
)

type RedisDedupe struct{ rdb *redis.Client }

func New(url string) *RedisDedupe {
	opt, _ := redis.ParseURL(url)
	return &RedisDedupe{rdb: redis.NewClient(opt)}
}

// TryEmit returns true if id was not seen within ttl.
func (d *RedisDedupe) TryEmit(id string, ttl time.Duration) bool {
	var ok bool
	operation := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		res, err := d.rdb.SetNX(ctx, "dedupe:"+id, 1, ttl).Result()
		if err != nil {
			metrics.RedisErrors.Inc()
			return err
		}
		ok = res
		return nil
	}
	ebo := backoff.NewExponentialBackOff()
	ebo.MaxElapsedTime = 5 * time.Second
	if err := backoff.Retry(operation, ebo); err != nil {
		metrics.RedisReconnects.Inc()
		return false
	}
	return ok
}
