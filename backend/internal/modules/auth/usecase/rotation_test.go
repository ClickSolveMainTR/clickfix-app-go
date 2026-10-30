package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/google/uuid"
)

type rotatingRepo struct {
	*memRepo
	session model.RefreshSession
}

func (r rotatingRepo) GetRefreshSession(context.Context, string) (model.RefreshSession, error) {
	return r.session, nil
}

type rotationRecorder struct {
	userID   int64
	from, to uuid.UUID
}

func (h *rotationRecorder) SessionRotated(_ context.Context, userID int64, from, to uuid.UUID) error {
	h.userID, h.from, h.to = userID, from, to
	return nil
}

func TestRefreshNotifiesSessionRotation(t *testing.T) {
	mem := newMemRepo()
	user, err := mem.CreateUser(context.Background(), model.User{Email: "r@x.io", Status: "active"}, true)
	if err != nil {
		t.Fatal(err)
	}
	old := uuid.New()
	tokens, _ := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	uc := New(rotatingRepo{memRepo: mem, session: model.RefreshSession{UUID: old, UserID: user.ID}}, tokens)
	hook := &rotationRecorder{}
	uc.SetSessionRotationHook(hook)

	out, err := uc.Refresh(context.Background(), "raw", model.SessionMeta{})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := tokens.ParseAccess(out.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if hook.userID != user.ID || hook.from != old || hook.to != claims.SessionUUID() || hook.to == uuid.Nil {
		t.Fatalf("hook %+v, new session %s", hook, claims.SessionUUID())
	}
}
