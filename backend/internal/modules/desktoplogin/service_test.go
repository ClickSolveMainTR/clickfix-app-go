package desktoplogin

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/model"
	authusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/usecase"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type fakeAccounts struct {
	deactivated map[uuid.UUID]bool
	issued      atomic.Int32
	mu          sync.Mutex
	lastMeta    model.SessionMeta
}

func (f *fakeAccounts) CheckQRLoginApprover(_ context.Context, id uuid.UUID) error {
	if f.deactivated[id] {
		return authusecase.ErrAccountDeactivated
	}
	return nil
}

func (f *fakeAccounts) IssueDesktopLoginSession(_ context.Context, id uuid.UUID, meta model.SessionMeta) (model.Tokens, authusecase.QRLoginUser, error) {
	if f.deactivated[id] {
		return model.Tokens{}, authusecase.QRLoginUser{}, authusecase.ErrAccountDeactivated
	}
	f.issued.Add(1)
	f.mu.Lock()
	f.lastMeta = meta
	f.mu.Unlock()
	return model.Tokens{AccessToken: "at", RefreshToken: "rt", TokenType: "Bearer"},
		authusecase.QRLoginUser{ID: 7, UUID: id, Email: "u@example.com", Name: "U"}, nil
}

func (f *fakeAccounts) IssueDesktopWebSession(_ context.Context, userID int64, _ uuid.UUID, meta model.SessionMeta) (model.Tokens, authusecase.QRLoginUser, error) {
	f.mu.Lock()
	f.lastMeta = meta
	f.mu.Unlock()
	return model.Tokens{AccessToken: "web-at", RefreshToken: "web-rt", TokenType: "Bearer"},
		authusecase.QRLoginUser{ID: userID, UUID: uuid.New(), Email: "u@example.com"}, nil
}

type fixture struct {
	svc  *Service
	mr   *miniredis.Miniredis
	acc  *fakeAccounts
	user uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	acc := &fakeAccounts{deactivated: map[uuid.UUID]bool{}}
	return &fixture{svc: NewService(NewStore(rdb, "test"), acc), mr: mr, acc: acc, user: uuid.New()}
}

const (
	testVerifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	testState    = "state-0123456789abcdef"
)

func (f *fixture) requester() Requester { return Requester{UserUUID: f.user, UserID: 7} }

func (f *fixture) create(t *testing.T) Created {
	t.Helper()
	c, err := f.svc.CreateCode(context.Background(), f.requester(), CodeInput{
		State: testState, CodeChallenge: S256(testVerifier), CodeChallengeMethod: "S256",
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *fixture) exchange(code, verifier string) (Exchanged, Redeemed, error) {
	return f.svc.Exchange(context.Background(), ExchangeInput{
		Code: code, CodeVerifier: verifier, State: testState, ClientOS: "macos", DeviceName: " Furkan's Mac ",
	}, "ClickFix-Desktop/1.0", "203.0.113.7")
}

func TestS256MatchesRFC7636(t *testing.T) {
	// RFC 7636 Appendix B.
	if S256(testVerifier) != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatal("S256 mismatch")
	}
}

func TestCreateCodeShape(t *testing.T) {
	f := newFixture(t)
	c := f.create(t)
	if !codePattern.MatchString(c.Code) || c.ExpiresIn != 60 {
		t.Fatalf("bad code %+v", c)
	}
	u, err := url.Parse(c.RedirectURL)
	if err != nil || u.Scheme != "clickfix" || u.Host != "auth" || u.Path != "/callback" ||
		u.Query().Get("code") != c.Code || u.Query().Get("state") != testState {
		t.Fatalf("redirect %s", c.RedirectURL)
	}
	// Redis holds only a hash of the code, never the code itself.
	for _, k := range f.mr.Keys() {
		if strings.Contains(k, c.Code) {
			t.Fatalf("raw code in key %s", k)
		}
	}
	if ttl := f.mr.TTL(f.mr.Keys()[0]); ttl <= 0 || ttl > CodeTTL {
		t.Fatalf("ttl %s", ttl)
	}
}

func TestCreateCodeValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	good := CodeInput{State: testState, CodeChallenge: S256(testVerifier), CodeChallengeMethod: "S256"}
	cases := map[string]CodeInput{
		"plain method":    {State: good.State, CodeChallenge: good.CodeChallenge, CodeChallengeMethod: "plain"},
		"short state":     {State: "abc", CodeChallenge: good.CodeChallenge, CodeChallengeMethod: "S256"},
		"state with &":    {State: "aaaaaaaaaaaaaaaa&x=1", CodeChallenge: good.CodeChallenge, CodeChallengeMethod: "S256"},
		"short challenge": {State: good.State, CodeChallenge: "abc", CodeChallengeMethod: "S256"},
	}
	for name, in := range cases {
		if _, err := f.svc.CreateCode(ctx, f.requester(), in); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := f.svc.CreateCode(ctx, Requester{UserUUID: f.user, Impersonating: true}, good); !errors.Is(err, ErrImpersonating) {
		t.Fatalf("impersonating: %v", err)
	}
	f.acc.deactivated[f.user] = true
	if _, err := f.svc.CreateCode(ctx, f.requester(), good); !errors.Is(err, authusecase.ErrAccountDeactivated) {
		t.Fatalf("deactivated: %v", err)
	}
	if len(f.mr.Keys()) != 0 {
		t.Fatal("refused requests must not store a code")
	}
}

func TestExchangeIssuesDesktopSession(t *testing.T) {
	f := newFixture(t)
	c := f.create(t)
	out, redeemed, err := f.exchange(c.Code, testVerifier)
	if err != nil {
		t.Fatal(err)
	}
	if out.RefreshToken != "rt" || out.User.UUID != f.user.String() || redeemed.UserID != 7 {
		t.Fatalf("exchange %+v %+v", out, redeemed)
	}
	m := f.acc.lastMeta
	if m.Client != (model.SessionClient{Kind: model.ClientDesktop, OS: "macos", DeviceName: "Furkan's Mac"}) ||
		m.UserAgent != "ClickFix-Desktop/1.0" || m.IP != "203.0.113.7" {
		t.Fatalf("meta %+v", m)
	}
}

func TestExchangeReuseIsRefused(t *testing.T) {
	f := newFixture(t)
	c := f.create(t)
	if _, _, err := f.exchange(c.Code, testVerifier); err != nil {
		t.Fatal(err)
	}
	_, redeemed, err := f.exchange(c.Code, testVerifier)
	if !errors.Is(err, ErrCodeUsed) || redeemed.UserID != 7 {
		t.Fatalf("reuse: %v %+v", err, redeemed)
	}
	if f.acc.issued.Load() != 1 {
		t.Fatalf("issued %d sessions", f.acc.issued.Load())
	}
}

func TestExchangeConcurrentSingleWinner(t *testing.T) {
	f := newFixture(t)
	c := f.create(t)
	var wg sync.WaitGroup
	var ok atomic.Int32
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := f.exchange(c.Code, testVerifier); err == nil {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || f.acc.issued.Load() != 1 {
		t.Fatalf("winners %d issued %d", ok.Load(), f.acc.issued.Load())
	}
}

func TestExchangeExpired(t *testing.T) {
	f := newFixture(t)
	c := f.create(t)
	f.mr.FastForward(CodeTTL + time.Second)
	if _, _, err := f.exchange(c.Code, testVerifier); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expired: %v", err)
	}
	if f.acc.issued.Load() != 0 {
		t.Fatal("no session for an expired code")
	}
}

func TestExchangeWrongVerifierBurnsCode(t *testing.T) {
	f := newFixture(t)
	c := f.create(t)
	wrong := strings.Repeat("x", 43)
	for i := 0; i < maxVerifierAttempts; i++ {
		_, redeemed, err := f.exchange(c.Code, wrong)
		if !errors.Is(err, ErrVerifierMismatch) || redeemed.UserID != 7 {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	// Burned: even the right verifier fails now.
	if _, _, err := f.exchange(c.Code, testVerifier); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("after burn: %v", err)
	}
	if f.acc.issued.Load() != 0 {
		t.Fatal("no session after wrong verifiers")
	}
}

func TestExchangeWrongStateAndInput(t *testing.T) {
	f := newFixture(t)
	c := f.create(t)
	ctx := context.Background()
	_, _, err := f.svc.Exchange(ctx, ExchangeInput{
		Code: c.Code, CodeVerifier: testVerifier, State: "other-state-0123456789", ClientOS: "macos",
	}, "", "")
	if !errors.Is(err, ErrVerifierMismatch) {
		t.Fatalf("wrong state: %v", err)
	}
	bad := []ExchangeInput{
		{Code: c.Code, CodeVerifier: testVerifier, State: testState, ClientOS: "ios"},
		{Code: c.Code, CodeVerifier: "short", State: testState, ClientOS: "macos"},
		{Code: c.Code, CodeVerifier: testVerifier, State: testState, ClientOS: "macos", DeviceName: "a\nb"},
		{Code: c.Code, CodeVerifier: testVerifier, State: testState, ClientOS: "macos", DeviceName: strings.Repeat("a", 65)},
	}
	for i, in := range bad {
		if _, _, err := f.svc.Exchange(ctx, in, "", ""); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	if _, _, err := f.svc.Exchange(ctx, ExchangeInput{
		Code: "nope", CodeVerifier: testVerifier, State: testState, ClientOS: "macos",
	}, "", ""); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("malformed code: %v", err)
	}
	// The right verifier still works: one wrong state used one attempt only.
	if _, _, err := f.exchange(c.Code, testVerifier); err != nil {
		t.Fatalf("valid after one miss: %v", err)
	}
}

func TestExchangeDeactivatedAccount(t *testing.T) {
	f := newFixture(t)
	c := f.create(t)
	f.acc.deactivated[f.user] = true
	if _, _, err := f.exchange(c.Code, testVerifier); !errors.Is(err, authusecase.ErrAccountDeactivated) {
		t.Fatalf("deactivated: %v", err)
	}
}
