package usecase

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
)

// A Unicode host can imitate a known one on the consent screen; only ASCII
// hosts (punycode for internationalized names) may be registered.
func TestRedirectHostMustBeASCII(t *testing.T) {
	for _, u := range []string{
		"https://cl\u0430ude.ai/cb",            // Cyrillic \u0430
		"https://ex\u0430mple.com:8443/cb",     // with a port
		"https://\u4f8b\u3048.jp/cb",           // Japanese
		"https://example.com\u200b.evil.io/cb", // zero width space
		"https://exa mple.com/cb",
		"http://loc\u0430lhost:7777/cb",
	} {
		if validRedirect(u) {
			t.Fatalf("accepted %q", u)
		}
	}
	for _, u := range []string{
		"https://claude.ai/api/mcp/auth_callback",
		"https://xn--cl-ude-4ve.ai/cb", // the punycode form shows itself for what it is
		"https://sub.example.co.uk:8443/cb?x=1",
		"http://localhost:7777/callback",
		"http://127.0.0.1:53682/cb",
		"http://[::1]:9000/cb",
	} {
		if !validRedirect(u) {
			t.Fatalf("rejected %q", u)
		}
	}
}

func TestCleanClientName(t *testing.T) {
	cases := map[string]string{
		"  Claude  ": "Claude",
		"Claude\nClickFix tarafından doğrulandı": "Claude ClickFix tarafından doğrulandı",
		"Cla\u202eude":           "Claude", // right-to-left override
		"Cl\u200baude\u0000":     "Claude", // zero width space, NUL
		"a\t\tb\r\nc":            "a b c",
		"":                       "MCP client",
		"\u200b\n":               "MCP client",
		"Çağrı Merkezi Asistanı": "Çağrı Merkezi Asistanı",
	}
	for in, want := range cases {
		if got := cleanClientName(in); got != want {
			t.Errorf("cleanClientName(%q) = %q, want %q", in, got, want)
		}
	}
	long := cleanClientName(strings.Repeat("ş", 500))
	if n := len([]rune(long)); n != maxClientName {
		t.Fatalf("long name kept %d runes", n)
	}
}

type fakeLimiter struct {
	deny  map[string]bool
	calls []string
}

func (l *fakeLimiter) Allow(_ context.Context, action, subject string, _ int, _ time.Duration) (bool, time.Duration) {
	l.calls = append(l.calls, "allow:"+action+":"+subject)
	return !l.deny[action], time.Minute
}

type fakeStrictLimiter struct{ fakeLimiter }

func (l *fakeStrictLimiter) AllowStrict(_ context.Context, action, subject string, _ int, _ time.Duration) (bool, time.Duration) {
	l.calls = append(l.calls, "strict:"+action+":"+subject)
	return !l.deny[action], time.Minute
}

func tooManyRequests(err error) bool {
	var oe *model.Error
	return errors.As(err, &oe) && oe.Status == http.StatusTooManyRequests
}

// Anonymous registration is capped per address and across all addresses, and
// asks the limiter in its fail-closed form. Both refusals happen before
// anything is stored (the service here has no database at all).
func TestRegistrationLimits(t *testing.T) {
	in := model.RegisterInput{ClientName: "x", RedirectURIs: []string{"https://client.example/cb"}}

	perIP := &fakeStrictLimiter{fakeLimiter{deny: map[string]bool{"mcp_oauth_register": true}}}
	if _, err := New(nil, perIP, issuer, nil).Register(context.Background(), "203.0.113.7", in); !tooManyRequests(err) {
		t.Fatalf("per-address limit: %v", err)
	}

	global := &fakeStrictLimiter{fakeLimiter{deny: map[string]bool{"mcp_oauth_register_all": true}}}
	if _, err := New(nil, global, issuer, nil).Register(context.Background(), "203.0.113.7", in); !tooManyRequests(err) {
		t.Fatalf("global limit: %v", err)
	}
	want := []string{"strict:mcp_oauth_register:203.0.113.7", "strict:mcp_oauth_register_all:all"}
	if strings.Join(global.calls, ",") != strings.Join(want, ",") {
		t.Fatalf("limiter calls %v, want %v", global.calls, want)
	}

	// A limiter without the strict form still applies both limits.
	plain := &fakeLimiter{deny: map[string]bool{"mcp_oauth_register_all": true}}
	if _, err := New(nil, plain, issuer, nil).Register(context.Background(), "203.0.113.7", in); !tooManyRequests(err) {
		t.Fatalf("global limit (plain limiter): %v", err)
	}
}

func TestUnusedRegistrationsAreKeptForADay(t *testing.T) {
	if DefaultUnusedClientAge != 24*time.Hour {
		t.Fatalf("unused client age is %s", DefaultUnusedClientAge)
	}
}

// TestAuthorizeNeverRedirectsToTheClient: registration is anonymous and takes
// any https address, so an authorize error must never send the browser to
// the registered redirect_uri (open redirect). Every invalid request is an
// error to show; only a valid one continues, and only to this origin's
// consent screen.
func TestAuthorizeNeverRedirectsToTheClient(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	svc := New(tx, nil, issuer, nil)
	svc.SetGate(&fakeGate{allowed: true})
	q := db.New(tx)
	var orgID int64
	var orgUUID uuid.UUID
	if err = tx.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES('MCP sec test',$1) RETURNING id,uuid", uuid.NewString()).Scan(&orgID, &orgUUID); err != nil {
		t.Fatal(err)
	}
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		Email: "mcpsec-" + uuid.NewString()[:8] + "@example.test", PasswordHash: "x", Name: "M", Surname: "S", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "INSERT INTO organization_members(organization_id,user_id,role_id) SELECT $1,$2,id FROM roles WHERE scope='tenant' AND organization_id IS NULL AND slug='owner'", orgID, user.ID); err != nil {
		t.Fatal(err)
	}
	evil := "https://evil.example/landing"
	client, err := svc.Register(ctx, "203.0.113.9", model.RegisterInput{ClientName: "Claude\nverified", RedirectURIs: []string{evil, "http://127.0.0.1:7777/cb"}})
	if err != nil {
		t.Fatal(err)
	}
	if client.ClientName != "Claude verified" {
		t.Fatalf("stored name %q", client.ClientName)
	}
	valid := model.AuthorizeInput{
		ClientID: client.ClientID, RedirectURI: evil, ResponseType: "code",
		CodeChallenge: strings.Repeat("a", 43), CodeChallengeMethod: "S256",
		State: "s", Scope: "mcp:read", Resource: issuer + model.ResourceBusiness,
	}
	broken := map[string]func(*model.AuthorizeInput){
		"response_type":    func(in *model.AuthorizeInput) { in.ResponseType = "x" },
		"challenge method": func(in *model.AuthorizeInput) { in.CodeChallengeMethod = "plain" },
		"challenge":        func(in *model.AuthorizeInput) { in.CodeChallenge = "short" },
		"resource":         func(in *model.AuthorizeInput) { in.Resource = "https://elsewhere.example/mcp/business" },
		"scope":            func(in *model.AuthorizeInput) { in.Scope = "mcp:admin" },
		"redirect_uri":     func(in *model.AuthorizeInput) { in.RedirectURI = "https://other.example/" },
		"client":           func(in *model.AuthorizeInput) { in.ClientID = "mcp_unknown" },
	}
	for name, mutate := range broken {
		in := valid
		mutate(&in)
		to, err := svc.Authorize(ctx, "203.0.113.9", in)
		var oe *model.Error
		if to != "" || !errors.As(err, &oe) || oe.Status != http.StatusBadRequest {
			t.Fatalf("%s: got redirect %q, error %v; want an error page and no redirect", name, to, err)
		}
	}

	// A loopback client (a program on the user's computer) does get the RFC
	// error redirect for a request error, since nothing else can show it the
	// error; an unknown client or an unregistered redirect_uri still never does.
	loopbackBad := valid
	loopbackBad.RedirectURI, loopbackBad.Scope = "http://127.0.0.1:7777/cb", "mcp:admin"
	to, err := svc.Authorize(ctx, "203.0.113.9", loopbackBad)
	if err != nil {
		t.Fatalf("loopback request error: %v", err)
	}
	lu, _ := url.Parse(to)
	if lu.Host != "127.0.0.1:7777" || lu.Query().Get("error") != model.ErrInvalidScope || lu.Query().Get("state") != "s" || lu.Query().Get("iss") != issuer {
		t.Fatalf("loopback error redirect %s", to)
	}
	unregistered := loopbackBad
	unregistered.RedirectURI = "http://127.0.0.1:9999/other"
	if to, err = svc.Authorize(ctx, "203.0.113.9", unregistered); to != "" || err == nil {
		t.Fatalf("unregistered loopback redirect_uri: redirect %q, err %v", to, err)
	}

	to, err = svc.Authorize(ctx, "203.0.113.9", valid)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(to)
	if u.Scheme+"://"+u.Host != issuer || u.Path != model.ConsentPath {
		t.Fatalf("a valid request must go to this origin's consent screen, got %s", to)
	}

	// The consent screen data: ASCII host, loopback told apart, lifetimes.
	id, _ := uuid.Parse(u.Query().Get("request"))
	info, err := svc.Consent(ctx, Actor{UserID: user.ID}, id)
	if err != nil {
		t.Fatal(err)
	}
	if info.RedirectHost != "evil.example" || info.RedirectLoopback {
		t.Fatalf("remote redirect shown as %q loopback=%v", info.RedirectHost, info.RedirectLoopback)
	}
	if info.AccessTTLSeconds != 3600 || info.RefreshTTLSeconds != 30*24*3600 {
		t.Fatalf("lifetimes %d / %d", info.AccessTTLSeconds, info.RefreshTTLSeconds)
	}
	local := valid
	local.RedirectURI = "http://127.0.0.1:7777/cb"
	to, err = svc.Authorize(ctx, "203.0.113.9", local)
	if err != nil {
		t.Fatal(err)
	}
	u, _ = url.Parse(to)
	id, _ = uuid.Parse(u.Query().Get("request"))
	if info, err = svc.Consent(ctx, Actor{UserID: user.ID}, id); err != nil || !info.RedirectLoopback || info.RedirectHost != "127.0.0.1:7777" {
		t.Fatalf("loopback redirect: %+v %v", info, err)
	}

	// Connected apps list the cleaned name too.
	approved, err := svc.Authorize(ctx, "203.0.113.9", local)
	if err != nil {
		t.Fatal(err)
	}
	au, _ := url.Parse(approved)
	aid, _ := uuid.Parse(au.Query().Get("request"))
	if _, err = svc.Decide(ctx, Actor{UserID: user.ID}, aid, model.DecideInput{Decision: model.DecisionApprove, OrganizationUUID: &orgUUID}); err != nil {
		t.Fatal(err)
	}
	grants, _, err := svc.ListGrants(ctx, Actor{UserID: user.ID}, ListFilter{Limit: 10})
	if err != nil || len(grants) != 1 || grants[0].ClientName != "Claude verified" {
		t.Fatalf("grants %+v %v", grants, err)
	}

	// A denial on the consent screen is the one error that goes to the client.
	dec, err := svc.Decide(ctx, Actor{UserID: user.ID}, id, model.DecideInput{Decision: model.DecisionDeny})
	if err != nil {
		t.Fatal(err)
	}
	back, _ := url.Parse(dec.RedirectURL)
	if back.Host != "127.0.0.1:7777" || back.Query().Get("error") != "access_denied" {
		t.Fatalf("denial redirect %s", dec.RedirectURL)
	}
}
