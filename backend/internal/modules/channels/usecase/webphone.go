package usecase

// Per-member SIP configuration ported from Garanti webphone/config.ts; tenant binding uses the channel vault.
import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/voice"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type WebphoneConfig struct {
	Enabled      bool      `json:"enabled"`
	InstanceUUID uuid.UUID `json:"instance_uuid"`
	WebsocketURL string    `json:"sip_websocket_url"`
	Domain       string    `json:"sip_domain"`
	Username     string    `json:"sip_username"`
	Password     string    `json:"sip_password"`
	ICEServers   []any     `json:"ice_servers"`
}
type SIPSettings struct {
	Extension     string `json:"sip_extension"`
	Username      string `json:"sip_username"`
	Password      string `json:"sip_password,omitempty"`
	ClearPassword bool   `json:"clear_password,omitempty"`
	HasPassword   bool   `json:"has_password"`
}

func (s *Service) MemberSIP(ctx context.Context, org int64, user uuid.UUID) (SIPSettings, error) {
	m, e := s.q.GetMemberSIP(ctx, db.GetMemberSIPParams{OrganizationID: org, UserUuid: user})
	if e != nil {
		return SIPSettings{}, notFound(e)
	}
	return SIPSettings{Extension: m.SipExtension,
			Username:    m.SipUsername,
			HasPassword: m.SipCredentialsEncrypted != ""},
		nil
}
func (s *Service) SetMemberSIP(ctx context.Context, org int64, user uuid.UUID, in SIPSettings) error {
	if in.Extension != "" && !regexp.MustCompile(`^[0-9]{1,6}$`).MatchString(in.Extension) || len(in.Username) > 128 || len(in.Password) > 512 || strings.ContainsAny(in.Username,
		"\r\n@; ") {
		return ErrInvalid
	}
	ciphertext := ""
	var err error
	if in.Password != "" {
		ciphertext, err = s.vault.Seal(org, user, channels.Credentials{"password": in.Password})
		if err != nil {
			return err
		}
	}
	n, err := s.q.SetMemberSIP(ctx,
		db.SetMemberSIPParams{OrganizationID: org,
			UserUuid:      user,
			SipExtension:  in.Extension,
			SipUsername:   in.Username,
			Ciphertext:    ciphertext,
			ClearPassword: in.ClearPassword})
	if err != nil {
		return ErrInvalid
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *Service) Webphone(ctx context.Context, org int64, user uuid.UUID) (WebphoneConfig, error) {
	out := WebphoneConfig{ICEServers: []any{}}
	member, err := s.q.GetMemberSIP(ctx, db.GetMemberSIPParams{OrganizationID: org, UserUuid: user})
	if err != nil {
		return out, notFound(err)
	}
	if member.SipUsername == "" || member.SipCredentialsEncrypted == "" {
		return out, nil
	}
	rows, err := s.q.ListChannelInstances(ctx, db.ListChannelInstancesParams{OrganizationID: org})
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		if row.ChannelType != channels.TypeVoice || row.Status != StatusActive {
			continue
		}
		inst, _, e := s.open(row)
		if e != nil || inst.Credentials["provider"] != "netgsm" {
			continue
		}
		ws, _ := inst.Settings["ws_url"].(string)
		domain, _ := inst.Settings["domain"].(string)
		u, e := url.Parse(ws)
		if e != nil || u.Scheme != "wss" || u.Hostname() == "" || u.User != nil || domain == "" || strings.ContainsAny(domain,
			"/\r\n@; ") {
			continue
		}
		c, e := s.vault.Open(org, user, member.SipCredentialsEncrypted)
		if e != nil {
			return out, errors.New("SIP credentials unavailable")
		}
		out = WebphoneConfig{Enabled: true,
			InstanceUUID: inst.UUID,
			WebsocketURL: ws,
			Domain:       domain,
			Username:     member.SipUsername,
			Password:     c["password"],
			ICEServers:   []any{}}
		if ice, ok := inst.Settings["ice_servers"].([]any); ok {
			out.ICEServers = ice
		}
		return out, nil
	}
	return out, nil
}

type NetgsmStore interface {
	InTx(context.Context, int64, int64, func(*db.Queries) error) error
}

func (s *Service) WithNetgsmStore(store NetgsmStore) *Service { s.netgsmStore = store; return s }

type WebphoneEvent struct {
	EventID     uuid.UUID `json:"event_id"`
	Direction   string    `json:"direction"`
	Phone       string    `json:"phone"`
	StartedAt   time.Time `json:"started_at"`
	EndedAt     time.Time `json:"ended_at"`
	Status      string    `json:"client_status"`
	TalkSeconds int32     `json:"talk_seconds"`
}

func (s *Service) WebphoneEvent(ctx context.Context, org int64, user uuid.UUID, in WebphoneEvent) error {
	window := in.EndedAt.Sub(in.StartedAt)
	invalidWindow := in.StartedAt.IsZero() || window < 0 || window > 24*time.Hour || in.EndedAt.After(time.Now().Add(time.Minute))
	invalidDuration := in.TalkSeconds < 0 || int64(in.TalkSeconds) > int64(window.Seconds())
	invalidIdentity := in.EventID == uuid.Nil || len(in.Phone) > 32
	invalidDirection := in.Direction != "inbound" && in.Direction != "outbound"
	if invalidWindow || invalidDuration || invalidIdentity || invalidDirection {
		return ErrInvalid
	}

	status := map[string]string{"answered": "completed",
		"failed":    "failed",
		"busy":      "busy",
		"no_answer": "no-answer",
		"cancelled": "canceled"}[in.Status]
	if status == "" {
		return ErrInvalid
	}
	c, err := s.Webphone(ctx, org, user)
	if err != nil {
		return err
	}
	if !c.Enabled {
		return ErrNotFound
	}
	member, err := s.q.GetMemberSIP(ctx, db.GetMemberSIPParams{OrganizationID: org, UserUuid: user})
	if err != nil {
		return notFound(err)
	}
	inst, _, err := s.Open(ctx, org, c.InstanceUUID)
	if err != nil {
		return err
	}
	phone := voice.PhoneLast10(in.Phone)
	if phone == "" {
		return ErrInvalid
	}
	from, to := in.Phone, member.SipExtension
	if in.Direction == "outbound" {
		from, to = to, from
	}
	if s.netgsmStore == nil {
		return errors.New("netgsm persistence unavailable")
	}
	return s.netgsmStore.InTx(ctx, org, inst.ID, func(q *db.Queries) error {
		return q.UpsertWebphoneEvent(ctx,
			db.UpsertWebphoneEventParams{OrganizationID: org,
				ChannelInstanceID: inst.ID,
				UserID: pgtype.Int8{Int64: member.UserID,
					Valid: true},
				Direction: in.Direction,
				Phone:     phone,
				EventID: pgtype.UUID{Bytes: in.EventID,
					Valid: true},
				Status:          status,
				FromNumber:      from,
				ToNumber:        to,
				InternalNumber:  member.SipExtension,
				DurationSeconds: in.TalkSeconds,
				StartedAt: pgtype.Timestamptz{Time: in.StartedAt,
					Valid: true},
				EndedAt: pgtype.Timestamptz{Time: in.EndedAt,
					Valid: true}})
	})
}
