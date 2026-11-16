package remotesupport

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	orgusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/livekit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Public routes open nothing but the one session named by its code, its join
// token, or its uuid plus the client secret issued at resolve (§7 threat 3).

const (
	warningTR = "ClickFix çalışanları ve işletmeler sizden asla para transferi, banka girişi veya kart bilgisi istemez. Sizi telefonla arayıp bu uygulamayı kurdurmak isteyen birine kod vermeyin."
	warningEN = "ClickFix staff and businesses will never ask you for a money transfer, bank login or card details. Do not give the code to someone who called you and asked you to install this app."
)

func clip(v string, n int) string {
	v = strings.TrimSpace(v)
	for len(v) > n || !utf8.ValidString(v) {
		v = strings.ToValidUTF8(v, "")
		if len(v) > n {
			v = v[:n]
		}
	}
	return v
}

// Resolve redeems a code or a join link (single use) and returns what the
// customer app shows before asking for consent. Wrong codes are charged to
// the pending session that owns the code's 5-digit prefix; the fifth wrong
// attempt invalidates that session. Every failure looks the same.
func (s *Service) Resolve(ctx context.Context, in ResolveInput) (Resolved, error) {
	var row db.RemoteSupportSession
	var err error
	switch {
	case in.Token != "" && in.Code == "":
		if !validSecret(in.Token) {
			return Resolved{}, ErrCodeInvalid
		}
		row, err = s.Q.RemoteSupportSessionByJoinToken(ctx, s.Hasher.JoinToken(in.Token))
		if errors.Is(err, pgx.ErrNoRows) || err == nil && row.Status != StatusPending {
			return Resolved{}, ErrCodeInvalid
		}
	case in.Code != "" && in.Token == "":
		code, ok := NormalizeCode(in.Code)
		if !ok {
			return Resolved{}, ErrInvalid
		}
		row, err = s.Q.RemoteSupportPendingByCode(ctx, s.Hasher.CodeLookup(code))
		if errors.Is(err, pgx.ErrNoRows) {
			return Resolved{}, ErrCodeInvalid
		}
		if err == nil && !Equal(s.Hasher.Code(code), row.CodeHash) {
			return Resolved{}, s.wrongCode(ctx, row)
		}
	default:
		return Resolved{}, ErrInvalid
	}
	if err != nil {
		return Resolved{}, err
	}
	if !row.CodeExpiresAt.Time.After(s.now()) {
		if _, err := s.end(ctx, row, "code_expired", BySystem, 0); err != nil {
			return Resolved{}, err
		}
		return Resolved{}, ErrCodeInvalid
	}
	set, err := s.usable(ctx, row.OrganizationID)
	if err != nil {
		return Resolved{}, err
	}
	secret, err := NewSecret()
	if err != nil {
		return Resolved{}, err
	}
	row, err = s.Q.RemoteSupportResolve(ctx, db.RemoteSupportResolveParams{OrganizationID: row.OrganizationID, ID: row.ID, ClientSecretHash: s.Hasher.Secret(secret),
		ClientOs: clip(in.ClientOS, 32), ClientVersion: clip(in.ClientVersion, 32)})
	if errors.Is(err, pgx.ErrNoRows) {
		// Redeemed concurrently by someone else.
		return Resolved{}, ErrCodeInvalid
	}
	if err != nil {
		return Resolved{}, err
	}
	org, err := s.Q.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return Resolved{}, err
	}
	out := Resolved{
		SessionUUID: row.Uuid, ClientSecret: secret, ModeRequested: row.ModeRequested, MaxDurationMinutes: row.MaxDurationMinutes,
		RecordingRequested: row.RecordingConsent.Valid && set.Recording == "optional", ConsentDeadline: row.ResolvedAt.Time.UTC().Add(CodeTTL),
		Organization: Organization{UUID: org.Uuid, Name: org.Name, LogoURL: orgusecase.LogoURL(org.Uuid, org.LogoObjectKey), Verified: org.Status == "active"},
		Warning:      warningTR, Locale: "tr",
	}
	if strings.HasPrefix(org.Locale, "en") {
		out.Warning, out.Locale = warningEN, "en"
	}
	if p, err := s.person(ctx, row.AgentUserID); err != nil {
		return Resolved{}, err
	} else if p != nil {
		out.Agent = *p
	}
	method := "code"
	if in.Token != "" {
		method = "link"
	}
	s.audit(ctx, row.OrganizationID, nil, "remote_support.resolved", row.Uuid, map[string]any{"method": method, "client_os": row.ClientOs, "client_version": row.ClientVersion})
	s.publish(ctx, row, "resolved")
	return out, nil
}

func (s *Service) wrongCode(ctx context.Context, row db.RemoteSupportSession) error {
	n, err := s.Q.RemoteSupportCodeAttempt(ctx, db.RemoteSupportCodeAttemptParams{OrganizationID: row.OrganizationID, ID: row.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCodeInvalid
	}
	if err != nil {
		return err
	}
	if n >= MaxCodeAttempts {
		s.audit(ctx, row.OrganizationID, nil, "remote_support.code_invalidated", row.Uuid, map[string]any{"attempts": n})
		if _, err := s.end(ctx, row, "attempts", BySystem, 0); err != nil {
			return err
		}
	}
	return ErrCodeInvalid
}

// client loads the session a customer call names and checks its secret.
func (s *Service) client(ctx context.Context, id uuid.UUID, secret string) (db.RemoteSupportSession, error) {
	if id == uuid.Nil || !validSecret(secret) {
		return db.RemoteSupportSession{}, ErrUnauthorized
	}
	row, err := s.Q.RemoteSupportSessionByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrUnauthorized
	}
	if err != nil {
		return row, err
	}
	if !Equal(s.Hasher.Secret(secret), row.ClientSecretHash) {
		return db.RemoteSupportSession{}, ErrUnauthorized
	}
	return row, nil
}

func customerView(r db.RemoteSupportSession) CustomerView {
	return CustomerView{SessionUUID: r.Uuid, Status: r.Status, ModeRequested: r.ModeRequested, ViewConsentAt: opt(r.ViewConsentAt),
		ControlActive: controlActive(r), RecordingConsent: boolPtr(r.RecordingConsent), EndsAt: endsAt(r), EndedAt: opt(r.EndedAt),
		EndedReason: r.EndedReason, Reported: r.ReportedAt.Valid}
}

// Consent records the customer's separate answers. Viewing must be given
// first (or together); control only when the agent asked for it and the
// organization allows it; recording only when the organization records.
// Withdrawing control takes data publishing away from the connected agent
// immediately.
func (s *Service) Consent(ctx context.Context, id uuid.UUID, in ConsentInput) (CustomerView, error) {
	row, err := s.client(ctx, id, in.ClientSecret)
	if err != nil {
		return CustomerView{}, err
	}
	if !open(row) || row.Status == StatusPending {
		return CustomerView{}, ErrEnded
	}
	if in.View == nil && in.Control == nil && in.Recording == nil || in.View != nil && !*in.View {
		return CustomerView{}, ErrInvalid
	}
	set, err := s.usable(ctx, row.OrganizationID)
	if err != nil {
		return CustomerView{}, err
	}
	now := s.now()
	viewing := row.ViewConsentAt.Valid || in.View != nil
	if !viewing {
		return CustomerView{}, ErrConsent
	}
	p := db.RemoteSupportSetConsentParams{OrganizationID: row.OrganizationID, ID: row.ID}
	if in.View != nil {
		p.ViewConsentAt = ts(now)
	}
	var actions []string
	if in.View != nil && !row.ViewConsentAt.Valid {
		actions = append(actions, "remote_support.view_consented")
	}
	if in.Control != nil {
		if row.ModeRequested != ModeControl || !set.AllowControl {
			return CustomerView{}, ErrInvalid
		}
		p.SetControl = true
		if *in.Control {
			p.ControlConsentAt = ts(now)
			actions = append(actions, "remote_support.control_consented")
		} else {
			p.ControlConsentAt, p.ControlRevokedAt = row.ControlConsentAt, ts(now)
			if !row.ControlConsentAt.Valid {
				// Declining before ever granting: recorded as consent never given.
				p.ControlConsentAt = pgtype.Timestamptz{}
				p.ControlRevokedAt = pgtype.Timestamptz{}
				actions = append(actions, "remote_support.control_declined")
			} else {
				actions = append(actions, "remote_support.control_revoked")
			}
		}
	}
	if in.Recording != nil {
		if !row.RecordingConsent.Valid || set.Recording != "optional" {
			return CustomerView{}, ErrInvalid
		}
		p.SetRecording, p.RecordingConsent = true, pgtype.Bool{Bool: *in.Recording, Valid: true}
		if *in.Recording {
			actions = append(actions, "remote_support.recording_consented")
		} else {
			actions = append(actions, "remote_support.recording_declined")
		}
	}
	updated, err := s.Q.RemoteSupportSetConsent(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return CustomerView{}, ErrEnded
	}
	if err != nil {
		return CustomerView{}, err
	}
	for _, a := range actions {
		s.audit(ctx, row.OrganizationID, nil, a, row.Uuid, map[string]any{"mode": row.ModeRequested})
	}
	if in.Control != nil {
		s.applyControl(ctx, updated, set)
	}
	if in.Recording != nil {
		if *in.Recording && updated.Status == StatusActive {
			s.startRecording(ctx, updated, set)
		} else if !*in.Recording {
			s.stopRecording(ctx, updated)
		}
	}
	s.publish(ctx, updated, "consent")
	return customerView(updated), nil
}

// CustomerToken: the customer publishes only the screen-share track and
// receives data; it publishes no data and no camera or microphone.
func (s *Service) CustomerToken(ctx context.Context, id uuid.UUID, in ClientInput) (Grant, error) {
	row, err := s.client(ctx, id, in.ClientSecret)
	if err != nil {
		return Grant{}, err
	}
	if !open(row) || row.Status == StatusPending {
		return Grant{}, ErrEnded
	}
	if !row.ViewConsentAt.Valid {
		return Grant{}, ErrConsent
	}
	if _, err = s.usable(ctx, row.OrganizationID); err != nil {
		return Grant{}, err
	}
	if err = s.ensureRoom(ctx, row); err != nil {
		return Grant{}, err
	}
	no := false
	grant := livekit.VideoGrant{Room: row.RoomName, CanPublish: ptr(true), CanSubscribe: ptr(true), CanPublishData: &no,
		CanPublishSources: []string{livekit.SourceScreenShare}, CanUpdateOwnMetadata: &no}
	ttl := s.tokenTTL(row)
	tok, err := s.LiveKit.JoinToken(customerIdentity(row), "", `{"role":"customer"}`, grant, ttl)
	if err != nil {
		return Grant{}, err
	}
	s.audit(ctx, row.OrganizationID, nil, "remote_support.customer_token_issued", row.Uuid, nil)
	return Grant{ServerURL: s.LiveKit.URL(), Token: tok, Identity: customerIdentity(row), Room: row.RoomName, ExpiresAt: s.now().Add(ttl)}, nil
}

// EndByCustomer: "Durdur" in the app, or declining before sharing.
func (s *Service) EndByCustomer(ctx context.Context, id uuid.UUID, in ClientInput) (CustomerView, error) {
	row, err := s.client(ctx, id, in.ClientSecret)
	if err != nil {
		return CustomerView{}, err
	}
	if open(row) {
		reason := "customer"
		if !row.ViewConsentAt.Valid {
			reason = "declined"
		}
		if row, err = s.end(ctx, row, reason, ByCustomer, 0); err != nil {
			return CustomerView{}, err
		}
	}
	return customerView(row), nil
}

// Report flags the session as a scam attempt, ends it and suspends the
// organization's remote support once reports pile up (ReportThreshold in
// ReportWindow) until the platform reviews it.
func (s *Service) Report(ctx context.Context, id uuid.UUID, in ReportInput) (CustomerView, error) {
	row, err := s.client(ctx, id, in.ClientSecret)
	if err != nil {
		return CustomerView{}, err
	}
	reason := clip(in.Reason, 1000)
	first := !row.ReportedAt.Valid
	if row, err = s.Q.RemoteSupportReport(ctx, db.RemoteSupportReportParams{OrganizationID: row.OrganizationID, ID: row.ID, ReportReason: reason}); err != nil {
		return CustomerView{}, err
	}
	s.audit(ctx, row.OrganizationID, nil, "remote_support.reported", row.Uuid, map[string]any{"has_reason": reason != ""})
	if open(row) {
		if row, err = s.end(ctx, row, "reported", ByCustomer, 0); err != nil {
			return CustomerView{}, err
		}
	} else {
		s.publish(ctx, row, "reported")
	}
	if first {
		if err = s.autoSuspend(ctx, row.OrganizationID); err != nil {
			s.log().Error("remote_support_auto_suspend_failed", "organization_id", row.OrganizationID, "error", err)
		}
	}
	return customerView(row), nil
}

func (s *Service) autoSuspend(ctx context.Context, org int64) error {
	n, err := s.Q.RemoteSupportRecentReports(ctx, db.RemoteSupportRecentReportsParams{OrganizationID: org, Since: ts(s.now().Add(-ReportWindow))})
	if err != nil || n < ReportThreshold {
		return err
	}
	set, err := s.settings(ctx, org)
	if err != nil || set.PlatformState != PlatformActive {
		return err
	}
	if _, err = s.Q.RemoteSupportSetPlatformState(ctx, db.RemoteSupportSetPlatformStateParams{OrganizationID: org, PlatformState: PlatformSuspended,
		PlatformReason: "Otomatik askıya alma: son 24 saatte dolandırıcılık bildirimi eşiği aşıldı"}); err != nil {
		return err
	}
	o, err := s.Q.GetOrganizationByID(ctx, org)
	if err != nil {
		return err
	}
	s.auditOrg(ctx, org, nil, "remote_support.auto_suspended", o.Uuid, map[string]any{"reports": n, "window_hours": int(ReportWindow.Hours())})
	s.log().Warn("remote_support_auto_suspended", "organization_id", org, "reports", n)
	return s.endAll(ctx, org, "platform", ByPlatform, 0)
}
