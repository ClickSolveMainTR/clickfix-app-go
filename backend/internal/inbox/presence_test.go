package inbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/inbox"
	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestRedisPresence(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	p := inbox.NewRedisPresence(rdb, "test")
	ctx := context.Background()
	conv := uuid.New()
	a, b := uuid.New(), uuid.New()
	now := time.Now()
	if err := p.Touch(ctx, 1, conv, convmodel.Viewer{UUID: a, Name: "Ayşe", At: now.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := p.Touch(ctx, 1, conv, convmodel.Viewer{UUID: b, Name: "Mert", Typing: true, At: now}); err != nil {
		t.Fatal(err)
	}
	got, err := p.List(ctx, 1, conv)
	if err != nil || len(got) != 1 || got[0].UUID != b || !got[0].Typing {
		t.Fatalf("stale viewer must be dropped: %+v (%v)", got, err)
	}
	if other, _ := p.List(ctx, 2, conv); len(other) != 0 {
		t.Fatal("presence is organization scoped")
	}
	if err := p.Leave(ctx, 1, conv, b); err != nil {
		t.Fatal(err)
	}
	if got, _ := p.List(ctx, 1, conv); len(got) != 0 {
		t.Fatalf("after leave: %+v", got)
	}
}
