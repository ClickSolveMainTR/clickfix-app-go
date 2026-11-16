package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/repository"
	"github.com/google/uuid"
)

// ErrNotDesktopSession is returned when a desktop-only call is made with a
// session that the desktop app did not open.
var ErrNotDesktopSession = errors.New("session is not a desktop app session")

// IssueDesktopLoginSession issues the desktop app's own session after a
// one-time code exchange (CF-234). The browser that created the code was
// already signed in (password, OAuth, passkey, SSO and its second factor), so
// no factor is asked again. client must be a desktop client.
func (u *AuthUseCase) IssueDesktopLoginSession(
	ctx context.Context,
	userUUID uuid.UUID,
	meta model.SessionMeta,
) (model.Tokens, QRLoginUser, error) {
	if meta.Client.Kind != model.ClientDesktop {
		return model.Tokens{}, QRLoginUser{}, fmt.Errorf("%w: desktop client is required", ErrInvalidRequest)
	}
	meta.ImpersonatorUserID = nil
	return u.issueSessionForUUID(ctx, userUUID, meta)
}

// IssueDesktopWebSession issues a second, separate session for the desktop
// app's embedded web view (CF-234). The app keeps its own refresh token in the
// OS keychain; sharing it with the web view would break rotation (each side
// would revoke the other's token). The caller's session (access token "sid")
// must be an active desktop session of the user; the new session carries the
// same client label.
func (u *AuthUseCase) IssueDesktopWebSession(
	ctx context.Context,
	userID int64,
	sessionUUID uuid.UUID,
	meta model.SessionMeta,
) (model.Tokens, QRLoginUser, error) {
	if sessionUUID == uuid.Nil {
		return model.Tokens{}, QRLoginUser{}, ErrNotDesktopSession
	}
	session, err := u.repo.GetActiveSession(ctx, userID, sessionUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.Tokens{}, QRLoginUser{}, ErrNotDesktopSession
		}
		return model.Tokens{}, QRLoginUser{}, err
	}
	if session.Client.Kind != model.ClientDesktop || session.ImpersonatorUserID != nil {
		return model.Tokens{}, QRLoginUser{}, ErrNotDesktopSession
	}
	user, err := u.repo.FindUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.Tokens{}, QRLoginUser{}, ErrInvalidCredentials
		}
		return model.Tokens{}, QRLoginUser{}, err
	}
	if err := userStatusError(user); err != nil {
		return model.Tokens{}, QRLoginUser{}, err
	}
	meta.ImpersonatorUserID = nil
	meta.Client = session.Client
	tokens, err := u.issueTokensForUser(ctx, user, meta)
	if err != nil {
		return model.Tokens{}, QRLoginUser{}, err
	}
	return tokens, QRLoginUser{ID: user.ID, UUID: user.UUID, Email: user.Email, Name: user.Name}, nil
}
