package inbox

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisSlackNames shares channel display names (Slack channels and users,
// Google Chat spaces and users) across API processes; a Redis failure only
// means one more provider lookup.
type RedisSlackNames struct {
	rdb    *redis.Client
	prefix string
}

// NewRedisSlackNames builds the Slack cache; prefix separates environments.
func NewRedisSlackNames(rdb *redis.Client, env string) *RedisSlackNames {
	return NewRedisChannelNames(rdb, env, "slack")
}

// NewRedisChannelNames builds the name cache of one channel type.
func NewRedisChannelNames(rdb *redis.Client, env, channelType string) *RedisSlackNames {
	return &RedisSlackNames{rdb: rdb, prefix: "cf:" + env + ":" + channelType + ":names:"}
}

// GetName implements slack.NameCache and googlechat.NameCache.
func (c *RedisSlackNames) GetName(ctx context.Context, key string) (string, bool) {
	v, err := c.rdb.Get(ctx, c.prefix+key).Result()
	if err != nil {
		return "", false
	}
	return v, true
}

// SetName implements slack.NameCache and googlechat.NameCache.
func (c *RedisSlackNames) SetName(ctx context.Context, key, name string, ttl time.Duration) {
	_ = c.rdb.Set(ctx, c.prefix+key, name, ttl).Err()
}
