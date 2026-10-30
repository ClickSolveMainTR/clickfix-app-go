package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type deletionFixture struct {
	p   *pgxpool.Pool
	r   *repository.Postgres
	u   model.User
	org int64
}

func deletionSetup(t *testing.T) deletionFixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	p, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(p.Close)
	f := deletionFixture{p: p, r: repository.NewPostgres(p, db.New(p))}
	f.u, e = f.r.CreateUser(ctx, model.User{Email: uuid.NewString() + "@example.test", Name: "Private", Surname: "Person", PasswordHash: "hash", Status: "active"}, true)
	if e != nil {
		t.Fatal(e)
	}
	if e = p.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'Deletion') RETURNING id", "deletion-"+uuid.NewString()[:8]).Scan(&f.org); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_, _ = p.Exec(ctx, "DELETE FROM organizations WHERE id=$1", f.org)
		_, _ = p.Exec(ctx, "DELETE FROM activity_events WHERE resource_uuid=$1 OR actor_user_id=$2", f.u.UUID, f.u.ID)
		_, _ = p.Exec(ctx, "DELETE FROM users WHERE id=$1", f.u.ID)
	})
	f.exec(t, "INSERT INTO organization_members(organization_id,user_id,role_id) SELECT $1,$2,id FROM roles WHERE slug='admin' AND scope='tenant' AND organization_id IS NULL", f.org, f.u.ID)
	return f
}
func (f deletionFixture) exec(t *testing.T, s string, args ...any) {
	t.Helper()
	if _, e := f.p.Exec(context.Background(), s, args...); e != nil {
		t.Fatal(e)
	}
}
func (f deletionFixture) count(t *testing.T, s string, args ...any) int {
	t.Helper()
	var n int
	if e := f.p.QueryRow(context.Background(), s, args...).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func TestAccountErasureGraceAndIdempotency(t *testing.T) {
	f := deletionSetup(t)
	ctx := context.Background()
	seed := []string{
		"INSERT INTO oauth_accounts(user_id,provider,provider_account_id,refresh_token_enc) VALUES($1,'apple',gen_random_uuid()::text,'encrypted')",
		"INSERT INTO webauthn_credentials(user_id,credential_id,public_key) VALUES($1,gen_random_uuid()::text,'key')",
		"INSERT INTO user_totp(user_id,secret_enc) VALUES($1,'secret')",
		"INSERT INTO refresh_tokens(user_id,token_hash,expires_at) VALUES($1,gen_random_uuid()::text,now()+interval '1 day')",
		"INSERT INTO otp_codes(user_id,email,code_hash,type,expires_at) VALUES($1,'private@example.test','hash','password_reset',now()+interval '1 day')",
		"INSERT INTO push_devices(user_id,token,platform) VALUES($1,gen_random_uuid()::text,'ios')",
		"INSERT INTO push_subscriptions(user_id,endpoint,key_p256dh,key_auth) VALUES($1,'https://example.test','key','auth')",
		"INSERT INTO notification_preferences(user_id) VALUES($1)",
		"INSERT INTO notifications(user_id,channel,recipient,body) VALUES($1,'email','private@example.test','private')",
		"INSERT INTO notification_digest_items(user_id,event_type,title) VALUES($1,'test','Private')",
		"INSERT INTO activity_events(actor_user_id,action,payload,ip_address,user_agent) VALUES($1,'test','{\"email\":\"private@example.test\"}','127.0.0.1','private agent')",
	}
	for _, s := range seed {
		f.exec(t, s, f.u.ID)
	}
	var contact, conv int64
	if e := f.p.QueryRow(ctx, "INSERT INTO contacts(organization_id,name) VALUES($1,'Customer') RETURNING id", f.org).Scan(&contact); e != nil {
		t.Fatal(e)
	}
	if e := f.p.QueryRow(ctx, "INSERT INTO conversations(organization_id,contact_id,number,channel_type) VALUES($1,$2,1,'test') RETURNING id", f.org, contact).Scan(&conv); e != nil {
		t.Fatal(e)
	}
	f.exec(t, "INSERT INTO messages(organization_id,conversation_id,direction,sender_type,sender_user_id,status,content) VALUES($1,$2,'out','agent',$3,'sent','Organization note')", f.org, conv, f.u.ID)
	f.r.SetDeletionGrace(48 * time.Hour)
	disabled, e := f.r.DeactivateUser(ctx, f.u.ID)
	if e != nil {
		t.Fatal(e)
	}
	if disabled.Status != "disabled" || disabled.DeletionDueAt == nil || time.Until(*disabled.DeletionDueAt) < 47*time.Hour {
		t.Fatalf("bad deadline: %+v", disabled)
	}
	if n := f.count(t, "SELECT count(*) FROM refresh_tokens WHERE user_id=$1 AND revoked_at IS NULL", f.u.ID); n != 0 {
		t.Fatal("live session")
	}
	if e = f.r.PurgeDueAccounts(ctx); e != nil {
		t.Fatal(e)
	}
	if n := f.count(t, "SELECT count(*) FROM oauth_accounts WHERE user_id=$1", f.u.ID); n != 1 {
		t.Fatal("erased before deadline")
	}
	// Signing in does not restore the account. A support reactivation explicitly cancels it.
	active := "active"
	if _, e = f.r.UpdateUserPlatform(ctx, f.u.UUID, nil, nil, &active); e != nil {
		t.Fatal(e)
	}
	restored, e := f.r.FindUserByUUID(ctx, f.u.UUID)
	if e != nil || restored.DeletionDueAt != nil || restored.DeactivatedAt != nil {
		t.Fatalf("support cancellation failed: %+v %v", restored, e)
	}
	if _, e = f.r.DeactivateUser(ctx, f.u.ID); e != nil {
		t.Fatal(e)
	}
	f.exec(t, "UPDATE users SET deletion_due_at=now()-interval '1 second' WHERE id=$1", f.u.ID)
	search := &deletionSearch{fail: true}
	f.r.SetDeletionSearch(search)
	if e = f.r.PurgeDueAccounts(ctx); e == nil {
		t.Fatal("search failure must preserve the durable deletion for retry")
	}
	if f.count(t, "SELECT count(*) FROM oauth_accounts WHERE user_id=$1", f.u.ID) != 1 {
		t.Fatal("failed erasure lost credentials before retry")
	}
	search.fail = false
	for range 2 {
		if e = f.r.PurgeDueAccounts(ctx); e != nil {
			t.Fatal(e)
		}
	}
	if len(search.docs) != 3 {
		t.Fatalf("profile/message search purge missing: %v", search.docs)
	}
	for _, table := range []string{"oauth_accounts", "webauthn_credentials", "user_totp", "refresh_tokens", "otp_codes", "push_devices", "push_subscriptions", "notification_preferences", "notifications", "notification_digest_items", "organization_members"} {
		if n := f.count(t, "SELECT count(*) FROM "+table+" WHERE user_id=$1", f.u.ID); n != 0 {
			t.Fatalf("%s retains user data", table)
		}
	}
	var name, email, hash string
	var deleted bool
	if e = f.p.QueryRow(ctx, "SELECT name,email,password_hash,deleted_at IS NOT NULL FROM users WHERE id=$1", f.u.ID).Scan(&name, &email, &hash, &deleted); e != nil {
		t.Fatal(e)
	}
	if name != "Silinmiş kullanıcı" || email != "deleted-"+f.u.UUID.String()+"@deleted.invalid" || hash != "" || !deleted {
		t.Fatalf("not erased: %s %s", name, email)
	}
	if n := f.count(t, "SELECT count(*) FROM messages m JOIN users u ON u.id=m.sender_user_id WHERE m.organization_id=$1 AND m.content='Organization note' AND u.name='Silinmiş kullanıcı'", f.org); n != 1 {
		t.Fatal("authored history lost")
	}
	if n := f.count(t, "SELECT count(*) FROM activity_events WHERE actor_user_id=$1 AND (payload<>'{}' OR ip_address IS NOT NULL OR user_agent IS NOT NULL)", f.u.ID); n != 0 {
		t.Fatal("audit retains PII")
	}
	if n := f.count(t, "SELECT count(*) FROM activity_events WHERE resource_uuid=$1 AND payload->>'stage'='erased'", f.u.UUID); n != 1 {
		t.Fatal("purge audit missing or duplicated")
	}
}
func TestAccountDeletionBlocksSoleOwner(t *testing.T) {
	f := deletionSetup(t)
	ctx := context.Background()
	f.exec(t, "INSERT INTO organization_member_roles(organization_id,member_id,role_id) SELECT m.organization_id,m.id,r.id FROM organization_members m CROSS JOIN roles r WHERE m.organization_id=$1 AND m.user_id=$2 AND r.slug='owner' AND r.scope='tenant' AND r.organization_id IS NULL", f.org, f.u.ID)
	if _, e := f.r.DeactivateUser(ctx, f.u.ID); !errors.Is(e, repository.ErrSoleOwner) {
		t.Fatalf("sole owner: %v", e)
	}
	u, e := f.r.FindUserByUUID(ctx, f.u.UUID)
	if e != nil || u.Status != "active" || u.DeletionDueAt != nil {
		t.Fatal("blocked request changed account")
	}
	// The existing organization closure flow permits deletion afterwards.
	f.exec(t, "UPDATE organizations SET deleted_at=now() WHERE id=$1", f.org)
	if _, e = f.r.DeactivateUser(ctx, f.u.ID); e != nil {
		t.Fatal(e)
	}
}

func TestAccountDeletionRequiresAnotherActiveOwner(t *testing.T) {
	f := deletionSetup(t)
	ctx := context.Background()
	other, e := f.r.CreateUser(ctx, model.User{Email: uuid.NewString() + "@example.test", Name: "Other", Surname: "Owner", PasswordHash: "hash", Status: "disabled"}, true)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_, _ = f.p.Exec(ctx, "DELETE FROM organization_members WHERE organization_id=$1 AND user_id=$2", f.org, other.ID)
		_, _ = f.p.Exec(ctx, "DELETE FROM users WHERE id=$1", other.ID)
	})
	f.exec(t, "UPDATE organization_members SET role_id=(SELECT id FROM roles WHERE slug='owner' AND scope='tenant' AND organization_id IS NULL) WHERE organization_id=$1 AND user_id=$2", f.org, f.u.ID)
	// UPDATE primary role does not add the permission role; set it just as the use case does.
	f.exec(t, "INSERT INTO organization_member_roles(organization_id,member_id,role_id) SELECT organization_id,id,role_id FROM organization_members WHERE organization_id=$1 AND user_id=$2 ON CONFLICT DO NOTHING", f.org, f.u.ID)
	f.exec(t, "INSERT INTO organization_members(organization_id,user_id,role_id) SELECT $1,$2,id FROM roles WHERE slug='owner' AND scope='tenant' AND organization_id IS NULL", f.org, other.ID)
	if _, e = f.r.DeactivateUser(ctx, f.u.ID); !errors.Is(e, repository.ErrSoleOwner) {
		t.Fatalf("disabled co-owner must not bypass guard: %v", e)
	}
	f.exec(t, "UPDATE users SET status='active' WHERE id=$1", other.ID)
	if _, e = f.r.DeactivateUser(ctx, f.u.ID); e != nil {
		t.Fatalf("active co-owner should permit deletion: %v", e)
	}
}

type deletionSearch struct {
	fail bool
	docs []string
}

func (s *deletionSearch) DeleteDocuments(_ context.Context, spec string, ids []string) error {
	for _, id := range ids {
		s.docs = append(s.docs, spec+":"+id)
	}
	if s.fail {
		return errors.New("search unavailable")
	}
	return nil
}
