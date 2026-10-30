package presence

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

type denyAll struct{ called bool }

func (d *denyAll) CanSubscribeChannel(context.Context, uuid.UUID, bool, string) (bool, error) {
	d.called = true
	return false, nil
}

type noResolver struct{}

func (noResolver) Resolve(context.Context, string, int64) (orgctx.Scope, error) {
	return orgctx.Scope{}, errors.New("not a member")
}

func TestAuthorizerFailsClosedAndDelegates(t *testing.T) {
	next := &denyAll{}
	a := Authorizer{Next: next, Resolver: noResolver{}}
	ctx := context.Background()
	user := uuid.New()
	for _, ch := range []string{"org:" + uuid.NewString(), "conversation:" + uuid.NewString()} {
		ok, err := a.CanSubscribeChannel(ctx, user, true, ch)
		if err != nil || ok {
			t.Fatalf("%s granted without membership", ch)
		}
	}
	if next.called {
		t.Fatal("org/conversation channels must not fall through")
	}
	if ok, _ := a.CanSubscribeChannel(ctx, user, false, "widget:visitor-1"); ok || !next.called {
		t.Fatal("other channels go to next (which denies widget)")
	}
}
