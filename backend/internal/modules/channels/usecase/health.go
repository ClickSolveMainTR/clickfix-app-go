package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/email"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// FeatureChannels is the plan limit on connected instances.
const FeatureChannels = "channels"

// WithEntitlements limits the number of instances by plan (CF-61).
func (s *Service) WithEntitlements(e entitlement.Engine) *Service {
	s.entitlements = e
	return s
}

// WithPublicAPIURL sets the API origin providers post webhooks to, so
// instance views carry an absolute webhook_url.
func (s *Service) WithPublicAPIURL(u string) *Service {
	s.publicAPIURL = strings.TrimRight(u, "/")
	return s
}

// WithLogger sets the logger of the background jobs.
func (s *Service) WithLogger(l *slog.Logger) *Service {
	if l != nil {
		s.log = l
	}
	return s
}

func (s *Service) checkLimit(ctx context.Context, orgID int64) error {
	if s.entitlements == nil {
		return nil
	}
	used, err := s.q.CountChannelInstances(ctx, orgID)
	if err != nil {
		return err
	}
	_, err = s.entitlements.CheckLimit(ctx, orgID, FeatureChannels, used, 1)
	return err
}

// Disconnecter is implemented by channels that undo a provider side
// registration when an instance is removed (Telegram deleteWebhook).
type Disconnecter interface {
	Disconnect(ctx context.Context, inst channels.Instance) error
}

// Health is the health block of an instance view.
type Health struct {
	// State: ok, warning (token expiring, failures, silent webhook),
	// error (check failed: reconnect), paused, unknown (never checked).
	State          string     `json:"state"`
	Detail         string     `json:"detail"`
	CheckedAt      *time.Time `json:"checked_at"`
	TokenExpiresAt *time.Time `json:"token_expires_at"`
	LastWebhookAt  *time.Time `json:"last_webhook_at"`
	// Sent24h / Failed24h count outbound messages of the last day.
	Sent24h   int64   `json:"sent_24h"`
	Failed24h int64   `json:"failed_24h"`
	ErrorRate float64 `json:"error_rate"`
	// Reconnect: the credentials stopped working or expire within a week.
	Reconnect bool `json:"reconnect"`
}

// Thresholds of the health state.
const (
	tokenWarnBefore = 7 * 24 * time.Hour
	errorRateWarn   = 0.2
)

type outboundStats struct {
	sent, failed int64
	// identityRejection is the code of the widget's last rejected identify
	// call of the week (CF-176 record), "" without one.
	identityRejection string
}

func (s *Service) stats(ctx context.Context, orgID int64) (map[int64]outboundStats, error) {
	rows, err := s.q.ChannelInstanceOutboundStats(ctx, db.ChannelInstanceOutboundStatsParams{
		OrganizationID: orgID, Since: pgtype.Timestamptz{Time: s.now().Add(-24 * time.Hour), Valid: true},
	})
	if err != nil {
		return nil, err
	}
	out := map[int64]outboundStats{}
	for _, r := range rows {
		st := out[r.InstanceID]
		switch r.Status {
		case "failed":
			st.failed += r.Count
		case "sent", "delivered", "read":
			st.sent += r.Count
		}
		out[r.InstanceID] = st
	}
	// CF-195: rejected identify calls warn on the widget's health.
	rejections, err := s.q.ListWidgetIdentityRejections(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for _, r := range rejections {
		st := out[r.ChannelInstanceID]
		st.identityRejection = r.Code
		out[r.ChannelInstanceID] = st
	}
	return out, nil
}

// Health details of a widget whose signed-in users cannot be verified
// (CF-195). The panel translates them.
const (
	DetailIdentitySecretMissing = "identity_secret_missing"
	DetailIdentityRejected      = "identity_rejected"
)

// widgetIdentityHealth turns a working widget's health into a warning
// when no identity secret is set (every visitor stays anonymous, identify
// is refused) or identify calls were rejected recently (a wrong secret or
// user_hash on the customer's site). Errors and pauses keep their state.
func widgetIdentityHealth(h *Health, hasSecret bool, rejection string) {
	if h.State == "error" || h.State == "paused" || h.State == "warning" {
		return
	}
	switch {
	case !hasSecret:
		h.State, h.Detail = "warning", DetailIdentitySecretMissing
	case rejection != "":
		h.State, h.Detail = "warning", DetailIdentityRejected+":"+rejection
	}
}

func (s *Service) health(row db.ChannelInstance, st outboundStats) Health {
	h := Health{
		State: "unknown", Detail: row.HealthDetail, CheckedAt: timePtr(row.HealthCheckedAt),
		TokenExpiresAt: timePtr(row.TokenExpiresAt), LastWebhookAt: timePtr(row.LastWebhookAt),
		Sent24h: st.sent, Failed24h: st.failed,
	}
	if total := st.sent + st.failed; total > 0 {
		h.ErrorRate = float64(st.failed) / float64(total)
	}
	if row.HealthOk.Valid {
		h.State = "ok"
		if !row.HealthOk.Bool {
			h.State = "error"
			h.Reconnect = true
		}
	}
	if h.State != "error" {
		if h.TokenExpiresAt != nil && h.TokenExpiresAt.Sub(s.now()) < tokenWarnBefore {
			h.State, h.Reconnect = "warning", true
			if h.Detail == "" {
				h.Detail = "token_expiring"
			}
		} else if st.failed >= 3 && h.ErrorRate >= errorRateWarn {
			h.State = "warning"
			if h.Detail == "" {
				h.Detail = "send_failures"
			}
		}
	}
	switch row.Status {
	case StatusPaused:
		h.State = "paused"
	case StatusError:
		h.State, h.Reconnect = "error", true
		if h.Detail == "" {
			h.Detail = row.LastError
		}
	}
	return h
}

// CheckHealth asks the provider about an instance and stores the answer.
func (s *Service) CheckHealth(ctx context.Context, orgID int64, id uuid.UUID) (InstanceView, error) {
	row, err := s.q.GetChannelInstanceByUUID(ctx, db.GetChannelInstanceByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return InstanceView{}, notFound(err)
	}
	if _, err := s.checkRow(ctx, row); err != nil {
		return InstanceView{}, err
	}
	return s.Get(ctx, orgID, id)
}

// TestDelivery sends a test event to the instance's own endpoint (only
// channels implementing channels.DeliveryTester, e.g. the REST API
// channel). It changes no data; the result is not stored as health.
func (s *Service) TestDelivery(ctx context.Context, orgID int64, id uuid.UUID, to string) (channels.DeliveryTest, error) {
	row, err := s.q.GetChannelInstanceByUUID(ctx, db.GetChannelInstanceByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return channels.DeliveryTest{}, notFound(err)
	}
	inst, adapter, err := s.open(row)
	if err != nil {
		return channels.DeliveryTest{}, err
	}
	tester, ok := adapter.(channels.DeliveryTester)
	if !ok {
		return channels.DeliveryTest{}, fmt.Errorf("%w: this channel type has no test delivery", ErrInvalid)
	}
	out, err := tester.TestDelivery(ctx, inst, to)
	if err != nil {
		return channels.DeliveryTest{}, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	return out, nil
}

// Reconnect runs Connect again with the stored credentials (it re-registers
// provider webhooks, e.g. Telegram setWebhook) and checks the health.
func (s *Service) Reconnect(ctx context.Context, orgID int64, id uuid.UUID) (InstanceView, error) {
	row, err := s.q.GetChannelInstanceByUUID(ctx, db.GetChannelInstanceByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return InstanceView{}, notFound(err)
	}
	inst, adapter, err := s.open(row)
	if err != nil {
		return InstanceView{}, err
	}
	ext, err := adapter.Connect(ctx, inst)
	if err != nil {
		_, _ = s.q.SetChannelInstanceHealth(ctx, db.SetChannelInstanceHealthParams{Ok: false, Detail: truncate(err.Error(), 500), OrganizationID: orgID, ID: row.ID})
		return InstanceView{}, errors.Join(ErrConnectFailed, err)
	}
	if ext != "" && ext != row.ExternalID.String {
		// The provider account changed (a different number or page): that is
		// a new connection, not a reconnect.
		return InstanceView{}, errors.Join(ErrConnectFailed, errors.New("the credentials belong to another account"))
	}
	if _, err := s.checkRow(ctx, row); err != nil {
		return InstanceView{}, err
	}
	if inst.Type == channels.TypeEmail && email.IsIMAP(inst.Credentials) {
		if err := s.resetIMAPBackoff(ctx, orgID, row.ID); err != nil {
			return InstanceView{}, err
		}
	}
	return s.Get(ctx, orgID, id)
}

func (s *Service) checkRow(ctx context.Context, row db.ChannelInstance) (channels.Health, error) {
	inst, adapter, err := s.open(row)
	if err != nil {
		return channels.Health{}, err
	}
	var h channels.Health
	if hc, ok := adapter.(channels.HealthChecker); ok {
		h, err = hc.CheckHealth(ctx, inst)
		if err != nil {
			// The provider could not be asked (outage): keep the last state.
			return channels.Health{}, err
		}
	} else if _, err := adapter.Connect(ctx, inst); err != nil {
		h = channels.Health{Detail: err.Error()}
	} else {
		h = channels.Health{OK: true}
	}
	exp := pgtype.Timestamptz{}
	if h.TokenExpiresAt != nil {
		exp = pgtype.Timestamptz{Time: *h.TokenExpiresAt, Valid: true}
	}
	_, err = s.q.SetChannelInstanceHealth(ctx, db.SetChannelInstanceHealthParams{
		Ok: h.OK, Detail: truncate(h.Detail, 500), TokenExpiresAt: exp, OrganizationID: row.OrganizationID, ID: row.ID,
	})
	return h, notFound(err)
}

// CredentialRefresher is implemented by channels whose tokens must be
// renewed before they expire (Instagram long-lived tokens).
type CredentialRefresher interface {
	RefreshCredentials(ctx context.Context, inst channels.Instance) (channels.Credentials, bool, error)
}

// refresh renews and stores an instance's credentials when its channel
// asks for it, returning the row to check.
func (s *Service) refresh(ctx context.Context, row db.ChannelInstance) (db.ChannelInstance, error) {
	inst, adapter, err := s.open(row)
	if err != nil {
		return row, err
	}
	r, ok := adapter.(CredentialRefresher)
	if !ok {
		return row, nil
	}
	creds, changed, err := r.RefreshCredentials(ctx, inst)
	if err != nil || !changed {
		return row, err
	}
	sealed, err := s.vault.Seal(row.OrganizationID, row.Uuid, creds)
	if err != nil {
		return row, err
	}
	return s.q.UpdateChannelInstance(ctx, db.UpdateChannelInstanceParams{
		BrandID: row.BrandID, Name: row.Name, Status: row.Status, ExternalID: row.ExternalID, CredentialsEnc: sealed, Settings: row.Settings,
		OrganizationID: row.OrganizationID, ID: row.ID,
	})
}

// HealthSweep renews expiring tokens and checks every connected instance
// (worker job). Failures of one instance never stop the sweep.
func (s *Service) HealthSweep(ctx context.Context) (checked, failing int, err error) {
	rows, err := s.q.ListLiveChannelInstances(ctx, s.registry.Types())
	if err != nil {
		return 0, 0, err
	}
	for _, row := range rows {
		// IMAP polling owns background health, including import failures and backoff.
		if row.ChannelType == channels.TypeEmail {
			if inst, _, err := s.open(row); err == nil && email.IsIMAP(inst.Credentials) {
				continue
			}
		}
		if ctx.Err() != nil {
			return checked, failing, ctx.Err()
		}
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if refreshed, err := s.refresh(cctx, row); err != nil {
			s.log.Warn("channel_token_refresh_failed", "instance", row.Uuid, "type", row.ChannelType, "error", err)
		} else {
			row = refreshed
		}
		h, err := s.checkRow(cctx, row)
		cancel()
		if err != nil {
			s.log.Warn("channel_health_check_failed", "instance", row.Uuid, "type", row.ChannelType, "error", err)
			continue
		}
		checked++
		if !h.OK {
			failing++
		}
	}
	return checked, failing, nil
}

// Receiver stores a webhook result (the conversations inbound pipeline).
type Receiver func(ctx context.Context, inst channels.Instance, res channels.WebhookResult) error

// Reconcile reads the recent messages of every instance whose channel can
// list them (Meta conversations API) and stores the ones webhooks missed
// (ported from garanti-kulucka meta-reconcile.ts). Storing is idempotent
// on the provider message id, so overlapping windows are harmless.
func (s *Service) Reconcile(ctx context.Context, window time.Duration, receive Receiver) (stored int, err error) {
	var types []string
	for _, t := range s.registry.Types() {
		if a, _ := s.registry.Get(t); a != nil {
			if _, ok := a.(channels.Reconciler); ok {
				types = append(types, t)
			}
		}
	}
	if len(types) == 0 {
		return 0, nil
	}
	rows, err := s.q.ListLiveChannelInstances(ctx, types)
	if err != nil {
		return 0, err
	}
	since := s.now().Add(-window)
	for _, row := range rows {
		inst, adapter, err := s.open(row)
		if err != nil {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, time.Minute)
		res, err := adapter.(channels.Reconciler).Reconcile(cctx, inst, since)
		if err == nil && len(res.Messages) > 0 {
			err = receive(cctx, inst, channels.WebhookResult{Messages: res.Messages})
		}
		cancel()
		if err != nil {
			s.log.Warn("channel_reconcile_failed", "instance", row.Uuid, "type", row.ChannelType, "error", err)
			continue
		}
		stored += len(res.Messages)
	}
	return stored, nil
}

// TouchWebhook records that a verified webhook reached an instance.
func (s *Service) TouchWebhook(ctx context.Context, orgID, id int64) {
	if err := s.q.TouchChannelInstanceWebhook(ctx, db.TouchChannelInstanceWebhookParams{OrganizationID: orgID, ID: id}); err != nil {
		s.log.Warn("channel_webhook_touch_failed", "instance", id, "error", err)
	}
}

func truncate(v string, n int) string {
	if len(v) <= n {
		return v
	}
	r := []rune(v)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}
