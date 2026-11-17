package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/google/uuid"
)

// desktopRepo records the session metadata written for each new refresh row
// and serves one active session for GetActiveSession.
type desktopRepo struct {
	*memRepo
	active model.RefreshSession
	saved  *[]model.SessionMeta
}

func (r desktopRepo) GetRefreshSession(context.Context, string) (model.RefreshSession, error) {
	return r.active, nil
}

func (r desktopRepo) GetActiveSession(_ context.Context, userID int64, id uuid.UUID) (model.RefreshSession, error) {
	if r.active.UUID != id || r.active.UserID != userID {
		return model.RefreshSession{}, repository.ErrNotFound
	}
	return r.active, nil
}

func (r desktopRepo) SaveRefresh(_ context.Context, _ int64, _ string, _ time.Time, meta model.SessionMeta) (uuid.UUID, error) {
	*r.saved = append(*r.saved, meta)
	return uuid.New(), nil
}

func newDesktopFixture(t *testing.T, client model.SessionClient) (*AuthUseCase, model.User, uuid.UUID, *[]model.SessionMeta) {
	t.Helper()
	mem := newMemRepo()
	user, err := mem.CreateUser(context.Background(), model.User{Email: "d@x.io", Status: "active"}, true)
	if err != nil {
		t.Fatal(err)
	}
	sid := uuid.New()
	saved := &[]model.SessionMeta{}
	tokens, _ := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	uc := New(desktopRepo{memRepo: mem, active: model.RefreshSession{UUID: sid, UserID: user.ID, Client: client}, saved: saved}, tokens)
	return uc, user, sid, saved
}

func TestIssueDesktopLoginSessionLabelsSession(t *testing.T) {
	desk := model.SessionClient{Kind: model.ClientDesktop, OS: "macos", DeviceName: "Mac"}
	uc, user, _, saved := newDesktopFixture(t, model.SessionClient{})
	if _, _, err := uc.IssueDesktopLoginSession(context.Background(), user.UUID, model.SessionMeta{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("non-desktop meta: %v", err)
	}
	_, got, err := uc.IssueDesktopLoginSession(context.Background(), user.UUID, model.SessionMeta{Client: desk})
	if err != nil || got.ID != user.ID || got.UUID != user.UUID {
		t.Fatalf("issue %+v %v", got, err)
	}
	if len(*saved) != 1 || (*saved)[0].Client != desk {
		t.Fatalf("saved %+v", *saved)
	}
}

func TestIssueDesktopWebSessionNeedsDesktopSession(t *testing.T) {
	desk := model.SessionClient{Kind: model.ClientDesktop, OS: "windows"}
	uc, user, sid, saved := newDesktopFixture(t, desk)
	if _, _, err := uc.IssueDesktopWebSession(context.Background(), user.ID, uuid.New(), model.SessionMeta{}); !errors.Is(err, ErrNotDesktopSession) {
		t.Fatalf("unknown session: %v", err)
	}
	if _, _, err := uc.IssueDesktopWebSession(context.Background(), user.ID, sid, model.SessionMeta{}); err != nil {
		t.Fatal(err)
	}
	if len(*saved) != 1 || (*saved)[0].Client != desk {
		t.Fatalf("web session must keep the desktop label: %+v", *saved)
	}

	web, webUser, webSID, _ := newDesktopFixture(t, model.SessionClient{})
	if _, _, err := web.IssueDesktopWebSession(context.Background(), webUser.ID, webSID, model.SessionMeta{}); !errors.Is(err, ErrNotDesktopSession) {
		t.Fatalf("web session must be refused: %v", err)
	}
}

func TestRefreshKeepsDesktopLabel(t *testing.T) {
	desk := model.SessionClient{Kind: model.ClientDesktop, OS: "linux"}
	uc, _, _, saved := newDesktopFixture(t, desk)
	if _, err := uc.Refresh(context.Background(), "raw", model.SessionMeta{UserAgent: "ua"}); err != nil {
		t.Fatal(err)
	}
	if len(*saved) != 1 || (*saved)[0].Client != desk || (*saved)[0].UserAgent != "ua" {
		t.Fatalf("rotated meta %+v", *saved)
	}
}
