package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis wraps a Redis client connection.
type Redis struct {
	client *redis.Client
}

// New creates a new Redis client from a connection URL.
func New(redisURL string) (*Redis, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("unable to ping redis: %w", err)
	}

	return &Redis{client: client}, nil
}

// Close closes the Redis connection.
func (r *Redis) Close() error {
	return r.client.Close()
}

// Ping checks the Redis connection.
func (r *Redis) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Set stores a value in Redis with a TTL.
func (r *Redis) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal cache value: %w", err)
	}
	return r.client.Set(ctx, key, data, ttl).Err()
}

// Get retrieves a value from Redis and unmarshals it into dest.
// Returns false if the key does not exist.
func (r *Redis) Get(ctx context.Context, key string, dest interface{}) (bool, error) {
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get cache value: %w", err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return false, fmt.Errorf("failed to unmarshal cache value: %w", err)
	}
	return true, nil
}

// Delete removes a key from Redis.
func (r *Redis) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

// DeleteByPattern removes all keys matching a glob pattern.
func (r *Redis) DeleteByPattern(ctx context.Context, pattern string) error {
	iter := r.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		if err := r.client.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}
