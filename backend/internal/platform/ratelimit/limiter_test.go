package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ratelimit"
	"github.com/redis/go-redis/v9"
)

func TestLoginLimit(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	lim := ratelimit.New(rdb, "test")
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		ok, _ := lim.AllowLogin(ctx, "1.1.1.1", "a@example.com")
		if !ok {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	ok, retry := lim.AllowLogin(ctx, "1.1.1.1", "a@example.com")
	if ok {
		t.Fatal("expected lockout")
	}
	if retry <= 0 {
		t.Fatal("expected retry-after")
	}
	ok, _ = lim.AllowLogin(ctx, "1.1.1.1", "b@example.com")
	if !ok {
		t.Fatal("different email should not share the bucket")
	}
}

func TestNilRedisAllows(t *testing.T) {
	t.Parallel()
	lim := ratelimit.New(nil, "test")
	ok, _ := lim.AllowRegister(context.Background(), "10.0.0.1")
	if !ok {
		t.Fatal("nil redis must fail open")
	}
}

func TestGenericAllow(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	lim := ratelimit.New(rdb, "test")
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if ok, _ := lim.Allow(ctx, "ai_msg", "u1", 3, time.Minute); !ok {
			t.Fatalf("hit %d should be allowed", i+1)
		}
	}
	ok, retry := lim.Allow(ctx, "ai_msg", "u1", 3, time.Minute)
	if ok || retry <= 0 || retry > time.Minute {
		t.Fatalf("expected limit with retry <= window, got ok=%v retry=%v", ok, retry)
	}
	if ok, _ := lim.Allow(ctx, "ai_msg", "u2", 3, time.Minute); !ok {
		t.Fatal("other subject must have its own bucket")
	}
	if ok, _ := lim.Allow(ctx, "ai_voice", "u1", 3, time.Minute); !ok {
		t.Fatal("other action must have its own bucket")
	}
	mr.FastForward(61 * time.Second)
	if ok, _ := lim.Allow(ctx, "ai_msg", "u1", 3, time.Minute); !ok {
		t.Fatal("window must reset")
	}
}

// AllowStrict refuses when the limiter cannot count: an anonymous endpoint
// must not become unlimited because Redis is away.
func TestAllowStrictFailsClosed(t *testing.T) {
	t.Parallel()
	var none *ratelimit.Limiter
	if ok, retry := none.AllowStrict(context.Background(), "a", "s", 5, time.Hour); ok || retry != time.Hour {
		t.Fatalf("nil limiter allowed (ok=%v retry=%s)", ok, retry)
	}
	// The lenient form keeps failing open, as the other limits expect.
	if ok, _ := none.Allow(context.Background(), "a", "s", 5, time.Hour); !ok {
		t.Fatal("Allow changed its behaviour")
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := ratelimit.New(rdb, "test")
	for i := 0; i < 2; i++ {
		if ok, _ := l.AllowStrict(context.Background(), "a", "s", 2, time.Hour); !ok {
			t.Fatalf("hit %d refused under the limit", i+1)
		}
	}
	if ok, _ := l.AllowStrict(context.Background(), "a", "s", 2, time.Hour); ok {
		t.Fatal("third hit allowed over a limit of two")
	}
	mr.Close()
	if ok, _ := l.AllowStrict(context.Background(), "a", "other", 2, time.Hour); ok {
		t.Fatal("allowed while Redis is down")
	}
	if ok, _ := l.Allow(context.Background(), "a", "other", 2, time.Hour); !ok {
		t.Fatal("Allow no longer fails open while Redis is down")
	}
}
