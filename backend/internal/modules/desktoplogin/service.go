// Package desktoplogin implements sign-in of the ClickFix desktop app through
// the system browser (CF-234, docs/design/masaustu.md §6).
//
// Google and Apple refuse OAuth inside an embedded web view, so the app opens
// {frontend}/desktop/login?state=...&challenge=... in the system browser. The
// signed-in browser creates a one-time, 60 second code bound to the app's
// state and PKCE S256 challenge (POST /v1/auth/desktop/codes) and hands it to
// the app through clickfix://auth/callback?code=...&state=.... The app trades
// code + code_verifier for its own session (POST /v1/auth/desktop/exchange);
// the session is labelled client=desktop in the device list. For the embedded
// web view the app asks for a second session (POST /v1/auth/desktop/web-session,
// called only by the web server's /desktop/session route).
package desktoplogin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/model"
	authusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/usecase"
	"github.com/google/uuid"
)

const (
	// CodeTTL is how long a code may be redeemed.
	CodeTTL = 60 * time.Second
	// maxVerifierAttempts burns the code after this many wrong verifiers.
	maxVerifierAttempts = 3
	codeBytes           = 32
	maxDeviceNameLen    = 64
	maxUserAgentLen     = 512
	// CallbackURL is the app's custom-scheme redirect (registered by the
	// desktop app, #218).
	CallbackURL = "clickfix://auth/callback"
)

var (
	// ErrInvalidRequest is a malformed state / challenge / verifier / OS.
	ErrInvalidRequest = errors.New("desktop login request is invalid")
	// ErrImpersonating blocks code creation from an impersonation session.
	ErrImpersonating = errors.New("desktop login cannot start while impersonating")
)

var (
	codePattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	challengePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	statePattern     = regexp.MustCompile(`^[A-Za-z0-9._~-]{16,128}$`)
	verifierPattern  = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)
)

// ValidOS lists the desktop platforms the app ships for.
var ValidOS = map[string]bool{"macos": true, "windows": true, "linux": true}

// Accounts is the slice of the auth use case desktop login needs.
type Accounts interface {
	CheckQRLoginApprover(ctx context.Context, userUUID uuid.UUID) error
	IssueDesktopLoginSession(ctx context.Context, userUUID uuid.UUID, meta model.SessionMeta) (model.Tokens, authusecase.QRLoginUser, error)
	IssueDesktopWebSession(ctx context.Context, userID int64, sessionUUID uuid.UUID, meta model.SessionMeta) (model.Tokens, authusecase.QRLoginUser, error)
}

// Service orchestrates desktop sign-in.
type Service struct {
	store    *Store
	accounts Accounts
	now      func() time.Time
}

// NewService wires the desktop login service.
func NewService(store *Store, accounts Accounts) *Service {
	return &Service{store: store, accounts: accounts, now: time.Now}
}

// S256 is the PKCE S256 code challenge of a verifier (RFC 7636 §4.2).
func S256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// Requester is the signed-in browser user asking for a code.
type Requester struct {
	UserUUID      uuid.UUID
	UserID        int64
	Impersonating bool
}

// CodeInput is the body of POST /v1/auth/desktop/codes.
type CodeInput struct {
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
}

// Created is the one-time code handed to the desktop app.
type Created struct {
	Code        string    `json:"code"`
	RedirectURL string    `json:"redirect_url"`
	ExpiresAt   time.Time `json:"expires_at"`
	ExpiresIn   int64     `json:"expires_in"`
}

// CreateCode issues a one-time code for the signed-in requester.
func (s *Service) CreateCode(ctx context.Context, req Requester, in CodeInput) (Created, error) {
	if !statePattern.MatchString(in.State) || !challengePattern.MatchString(in.CodeChallenge) ||
		in.CodeChallengeMethod != "S256" {
		return Created{}, ErrInvalidRequest
	}
	if req.Impersonating {
		return Created{}, ErrImpersonating
	}
	if err := s.accounts.CheckQRLoginApprover(ctx, req.UserUUID); err != nil {
		return Created{}, err
	}
	code, err := randomCode()
	if err != nil {
		return Created{}, err
	}
	now := s.now().UTC()
	expires := now.Add(CodeTTL)
	if err := s.store.Create(ctx, code, Code{
		UserUUID: req.UserUUID.String(), UserID: req.UserID,
		State: in.State, Challenge: in.CodeChallenge,
		CreatedAt: now, ExpiresAt: expires,
	}, CodeTTL); err != nil {
		return Created{}, err
	}
	q := url.Values{}
	q.Set("code", code)
	q.Set("state", in.State)
	return Created{
		Code: code, RedirectURL: CallbackURL + "?" + q.Encode(),
		ExpiresAt: expires, ExpiresIn: int64(CodeTTL.Seconds()),
	}, nil
}

// ExchangeInput is the body of POST /v1/auth/desktop/exchange.
type ExchangeInput struct {
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
	State        string `json:"state"`
	ClientOS     string `json:"client_os"`
	DeviceName   string `json:"device_name,omitempty"`
}

// Exchanged is the desktop app's session.
type Exchanged struct {
	model.Tokens
	User ExchangedUser `json:"user"`
}

// ExchangedUser identifies the signed-in account.
type ExchangedUser struct {
	UUID  string `json:"uuid"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// Exchange redeems a code exactly once. On failure Redeemed carries the
// account when it is known (reuse, wrong verifier) so the handler can audit.
func (s *Service) Exchange(ctx context.Context, in ExchangeInput, userAgent, ip string) (Exchanged, Redeemed, error) {
	if !ValidOS[in.ClientOS] || !verifierPattern.MatchString(in.CodeVerifier) ||
		!statePattern.MatchString(in.State) {
		return Exchanged{}, Redeemed{}, ErrInvalidRequest
	}
	name, ok := cleanDeviceName(in.DeviceName)
	if !ok {
		return Exchanged{}, Redeemed{}, ErrInvalidRequest
	}
	if !codePattern.MatchString(in.Code) {
		return Exchanged{}, Redeemed{}, ErrInvalidCode
	}
	redeemed, err := s.store.Redeem(ctx, in.Code, S256(in.CodeVerifier), in.State, maxVerifierAttempts)
	if err != nil {
		return Exchanged{}, redeemed, err
	}
	userUUID, err := uuid.Parse(redeemed.UserUUID)
	if err != nil {
		return Exchanged{}, Redeemed{}, ErrInvalidCode
	}
	if len(userAgent) > maxUserAgentLen {
		userAgent = userAgent[:maxUserAgentLen]
	}
	tokens, user, err := s.accounts.IssueDesktopLoginSession(ctx, userUUID, model.SessionMeta{
		UserAgent: userAgent, IP: ip,
		Client: model.SessionClient{Kind: model.ClientDesktop, OS: in.ClientOS, DeviceName: name},
	})
	if err != nil {
		return Exchanged{}, redeemed, err
	}
	redeemed.UserID = user.ID
	return Exchanged{
		Tokens: tokens,
		User:   ExchangedUser{UUID: user.UUID.String(), Email: user.Email, Name: user.Name},
	}, redeemed, nil
}

// WebSession issues the embedded web view's own session for a desktop app
// session (see authusecase.IssueDesktopWebSession).
func (s *Service) WebSession(ctx context.Context, userID int64, sessionUUID uuid.UUID, userAgent, ip string) (Exchanged, error) {
	if len(userAgent) > maxUserAgentLen {
		userAgent = userAgent[:maxUserAgentLen]
	}
	tokens, user, err := s.accounts.IssueDesktopWebSession(ctx, userID, sessionUUID, model.SessionMeta{
		UserAgent: userAgent, IP: ip,
	})
	if err != nil {
		return Exchanged{}, err
	}
	return Exchanged{
		Tokens: tokens,
		User:   ExchangedUser{UUID: user.UUID.String(), Email: user.Email, Name: user.Name},
	}, nil
}

// cleanDeviceName trims the optional computer name; control characters are
// refused and the length is bounded.
func cleanDeviceName(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if len([]rune(v)) > maxDeviceNameLen {
		return "", false
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return v, true
}

func randomCode() (string, error) {
	buf := make([]byte, codeBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("desktoplogin: random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
