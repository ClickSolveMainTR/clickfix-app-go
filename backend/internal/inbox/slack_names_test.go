package inbox

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisSlackNames(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	c := NewRedisSlackNames(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test")
	ctx := context.Background()
	if _, ok := c.GetName(ctx, "1:T1:users.info:U1"); ok {
		t.Fatal("empty cache hit")
	}
	c.SetName(ctx, "1:T1:users.info:U1", "Ayşe", time.Hour)
	c.SetName(ctx, "1:T1:conversations.info:C1", "", time.Minute)
	if v, ok := c.GetName(ctx, "1:T1:users.info:U1"); !ok || v != "Ayşe" {
		t.Fatalf("got %q %v", v, ok)
	}
	if v, ok := c.GetName(ctx, "1:T1:conversations.info:C1"); !ok || v != "" {
		t.Fatalf("cached miss lost: %q %v", v, ok)
	}
	if ttl := mr.TTL("cf:test:slack:names:1:T1:users.info:U1"); ttl != time.Hour {
		t.Fatalf("ttl = %v", ttl)
	}
	mr.FastForward(2 * time.Hour)
	if _, ok := c.GetName(ctx, "1:T1:users.info:U1"); ok {
		t.Fatal("expired name served")
	}
}
