// --- File: internal/platform/redis/client.go ---
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps go-redis to satisfy the internal cache.CacheClient interface.
type RedisClient struct {
	rdb *redis.Client
}

func NewRedisClient(addr, password string, db int) (*RedisClient, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Fail fast if connection is bad
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &RedisClient{rdb: rdb}, nil
}

func (c *RedisClient) Get(ctx context.Context, key string, dest interface{}) error {
	val, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return err // redis.Nil is returned as error, matching our interface expectation
	}
	return json.Unmarshal(val, dest)
}

func (c *RedisClient) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	bytes, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, bytes, ttl).Err()
}

func (c *RedisClient) Del(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

func (c *RedisClient) Close() error {
	return c.rdb.Close()
}
