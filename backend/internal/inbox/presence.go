package inbox

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	convmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisPresence keeps conversation viewers in one Redis hash per
// conversation (field = agent uuid); entries older than PresenceTTL are
// dropped on read, the hash expires when nobody heartbeats.
type RedisPresence struct {
	rdb    *redis.Client
	prefix string
	now    func() time.Time
}

// NewRedisPresence builds the store; prefix separates environments.
func NewRedisPresence(rdb *redis.Client, env string) *RedisPresence {
	return &RedisPresence{rdb: rdb, prefix: "cf:" + env + ":viewers:", now: time.Now}
}

func (p *RedisPresence) key(orgID int64, conv uuid.UUID) string {
	return fmt.Sprintf("%s%d:%s", p.prefix, orgID, conv)
}

// Touch implements usecase.PresenceStore.
func (p *RedisPresence) Touch(ctx context.Context, orgID int64, conv uuid.UUID, v convmodel.Viewer) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	k := p.key(orgID, conv)
	pipe := p.rdb.TxPipeline()
	pipe.HSet(ctx, k, v.UUID.String(), raw)
	pipe.Expire(ctx, k, 2*convusecase.PresenceTTL)
	_, err = pipe.Exec(ctx)
	return err
}

// Leave implements usecase.PresenceStore.
func (p *RedisPresence) Leave(ctx context.Context, orgID int64, conv uuid.UUID, user uuid.UUID) error {
	return p.rdb.HDel(ctx, p.key(orgID, conv), user.String()).Err()
}

// List implements usecase.PresenceStore (fresh viewers, oldest first).
func (p *RedisPresence) List(ctx context.Context, orgID int64, conv uuid.UUID) ([]convmodel.Viewer, error) {
	k := p.key(orgID, conv)
	all, err := p.rdb.HGetAll(ctx, k).Result()
	if err != nil {
		return nil, err
	}
	cutoff := p.now().Add(-convusecase.PresenceTTL)
	out := make([]convmodel.Viewer, 0, len(all))
	var stale []string
	for field, raw := range all {
		var v convmodel.Viewer
		if json.Unmarshal([]byte(raw), &v) != nil || v.At.Before(cutoff) {
			stale = append(stale, field)
			continue
		}
		out = append(out, v)
	}
	if len(stale) > 0 {
		_ = p.rdb.HDel(ctx, k, stale...).Err()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}
