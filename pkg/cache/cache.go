package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrMiss = errors.New("cache miss")

type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error) // ErrMiss when absent
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	// SetNX stores val only if key is absent and reports whether it did.
	// Used for idempotency claims.
	SetNX(ctx context.Context, key string, val []byte, ttl time.Duration) (bool, error)
}

type Redis struct{ c *redis.Client }

func NewRedis(addr string) *Redis {
	return &Redis{c: redis.NewClient(&redis.Options{
		Addr: addr, DialTimeout: 2 * time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second,
	})}
}

func (r *Redis) Get(ctx context.Context, key string) ([]byte, error) {
	b, err := r.c.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrMiss
	}
	return b, err
}

func (r *Redis) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	return r.c.Set(ctx, key, val, ttl).Err()
}

func (r *Redis) SetNX(ctx context.Context, key string, val []byte, ttl time.Duration) (bool, error) {
	return r.c.SetNX(ctx, key, val, ttl).Result()
}

func (r *Redis) Delete(ctx context.Context, keys ...string) error { return r.c.Del(ctx, keys...).Err() }
