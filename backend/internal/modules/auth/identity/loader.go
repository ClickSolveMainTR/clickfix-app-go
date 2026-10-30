package identity

import (
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
)

// Loader hydrates Principal from JWT claims via AuthUseCase.
type Loader struct {
	UC *usecase.AuthUseCase
}

// LoadPrincipal implements middleware.IdentityLoader.
func (l Loader) LoadPrincipal(r *http.Request, claims jwt.Claims) (authctx.Principal, error) {
	ctx := r.Context()
	userUUID, err := claims.UserUUID()
	if err != nil {
		return authctx.Principal{
			SSOOrganizationID: claims.SSOOrganizationID}, err
	}
	user, err := l.UC.LoadUserByUUID(ctx, userUUID)
	if err != nil {
		return authctx.Principal{
			SSOOrganizationID: claims.SSOOrganizationID}, err
	}
	if user.Status == "disabled" {
		return authctx.Principal{
			SSOOrganizationID: claims.SSOOrganizationID}, usecase.ErrUserDisabled
	}
	isSuperAdmin := claims.IsSuperAdmin
	perms, err := l.UC.ResolvePermissions(ctx, claims.Roles, isSuperAdmin)
	if err != nil {
		return authctx.Principal{
			SSOOrganizationID: claims.SSOOrganizationID}, err
	}
	impersonatorUUID, err := claims.ImpersonatorUUID()
	if err != nil {
		return authctx.Principal{
			SSOOrganizationID: claims.SSOOrganizationID}, err
	}
	return authctx.Principal{
		SSOOrganizationID:  claims.SSOOrganizationID,
		UserID:             user.UUID,
		UserInternal:       user.ID,
		Email:              user.Email,
		Roles:              claims.Roles,
		Permissions:        perms,
		IsSuperAdmin:       isSuperAdmin,
		ImpersonatorUserID: impersonatorUUID,
		SessionID:          claims.SessionUUID(),
	}, nil
}
