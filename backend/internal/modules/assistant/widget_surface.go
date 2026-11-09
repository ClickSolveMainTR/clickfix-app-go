package assistant

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
)

// Surfaces are the assistant's chat surfaces: the guide surfaces plus
// ClickFix's own support widget embedded in the panel (CF-193).
var Surfaces = []string{capability.SurfaceWeb, capability.SurfaceMobile, capability.SurfaceWidget}

// WidgetWrites are the small, confirmed set-ups the support widget may
// propose besides reads. Everything else stays in the panel drawer.
var WidgetWrites = []string{"navigation.open_route", "channels.connect", "ai.enable", "channels.test", "webhooks.test"}

// WidgetAllowed is the widget surface's allowlist: panel reads and the
// WidgetWrites. Destructive capabilities and those holding secrets never
// qualify; MCP-only capabilities stay off.
func WidgetAllowed(c *capability.Capability) bool {
	if !c.OnSurface(capability.SurfaceWeb) || len(c.Secrets) > 0 || c.Kind == capability.Destructive {
		return false
	}
	return c.Kind == capability.Read || slices.Contains(WidgetWrites, c.Name)
}

// guideSurface maps a chat surface to the guide catalog (the widget runs
// in the web panel).
func guideSurface(s string) string {
	if s == capability.SurfaceWidget {
		return capability.SurfaceWeb
	}
	return s
}

// SupportWidget identifies a member to ClickFix's own support widget.
type SupportWidget struct {
	Available bool   `json:"available"`
	WidgetID  string `json:"widget_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	UserHash  string `json:"user_hash,omitempty"`
}

// SupportWidgetFor returns the support organization's widget and the
// member's identity signature. The identity secret never leaves the
// server; without a support organization, an active widget or identity
// verification the widget is unavailable. The user id is scoped to the
// organization so one person in two organizations is two contacts.
func (s *Service) SupportWidgetFor(ctx context.Context, scope orgctx.Scope, user uuid.UUID) (SupportWidget, error) {
	st, err := s.GetSettings(ctx)
	if err != nil || st.SupportOrgUUID == nil || s.Instances == nil {
		return SupportWidget{}, err
	}
	row, err := s.Q.AssistantSupportWidget(ctx, *st.SupportOrgUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return SupportWidget{}, nil
	}
	if err != nil {
		return SupportWidget{}, err
	}
	inst, _, err := s.Instances.Instance(ctx, row.OrganizationID, row.ID)
	if err != nil {
		return SupportWidget{}, err
	}
	secret := inst.Credentials[widget.IdentitySecretField]
	if secret == "" {
		return SupportWidget{}, nil
	}
	id := "member:" + scope.UUID.String() + ":" + user.String()
	return SupportWidget{Available: true, WidgetID: row.PublicUuid.String(), UserID: id, UserHash: widget.UserHash(secret, id)}, nil
}
