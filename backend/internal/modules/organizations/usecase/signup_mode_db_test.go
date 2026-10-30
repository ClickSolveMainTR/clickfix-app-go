package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	orgmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/repository"
	"github.com/google/uuid"
)

type fixedSignup string

func (f fixedSignup) SignupMode(context.Context) (string, error) { return string(f), nil }

type stubAccounts struct{ q *db.Queries }

func (a stubAccounts) RegisterInvited(ctx context.Context, email, name, surname, _ string) (int64, error) {
	u, err := a.q.CreateUser(ctx, db.CreateUserParams{Email: email, PasswordHash: "x", Name: name, Surname: surname, Status: "active"})
	return u.ID, err
}

// CF-143: with sign-up closed, only invitations a platform admin sent open
// new accounts; team invitations need an existing account. Platform admins
// can still create organizations with an owner invitation.
func TestClosedSignupAdmitsOnlyPlatformInvitations(t *testing.T) {
	pool, q := testPool(t)
	ctx := context.Background()
	repo := repository.NewPostgres(pool, q)
	svc := New(repo, nil)
	m := NewManagement(svc, repo, ManagementConfig{FrontendURL: "https://example.test"})
	m.SetAccounts(stubAccounts{q: q})
	m.SetSignupPolicy(fixedSignup("closed"))
	var tokens []string
	m.newToken = func() (string, error) {
		tok, err := NewInvitationToken()
		tokens = append(tokens, tok)
		return tok, err
	}
	suffix := uuid.NewString()[:8]
	org, ownerInv, err := m.CreateOrganizationWithOwnerInvitation(ctx, orgmodel.CreateOrganizationInput{Name: "Kapalı kayıt " + suffix}, "owner-"+suffix+"@example.test", Actor{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id=$1", org.ID) })
	if ownerInv.Email == "" {
		t.Fatal("owner invitation missing")
	}
	if _, err = m.RegisterAndAccept(ctx, tokens[0], orgmodel.RegisterInvitedInput{Name: "Ayşe", Surname: "Yılmaz", Password: "password-123"}); err != nil {
		t.Fatalf("platform owner invitation refused: %v", err)
	}
	var role uuid.UUID
	if err = pool.QueryRow(ctx, "SELECT uuid FROM roles WHERE scope='tenant' AND organization_id IS NULL AND slug<>'owner' ORDER BY id LIMIT 1").Scan(&role); err != nil {
		t.Fatal(err)
	}
	scope := OperatorScope(org)
	if _, err = m.CreateInvitation(ctx, scope, Actor{}, orgmodel.CreateInvitationInput{Email: "team-" + suffix + "@example.test", RoleUUIDs: []uuid.UUID{role}}); err != nil {
		t.Fatal(err)
	}
	if _, err = m.RegisterAndAccept(ctx, tokens[1], orgmodel.RegisterInvitedInput{Name: "Can", Surname: "Demir", Password: "password-123"}); !errors.Is(err, ErrRegistrationClosed) {
		t.Fatalf("team invitation opened an account while closed: %v", err)
	}
	m.SetSignupPolicy(fixedSignup("invite"))
	if _, err = m.RegisterAndAccept(ctx, tokens[1], orgmodel.RegisterInvitedInput{Name: "Can", Surname: "Demir", Password: "password-123"}); err != nil {
		t.Fatalf("invite mode refused a team invitation: %v", err)
	}
}
