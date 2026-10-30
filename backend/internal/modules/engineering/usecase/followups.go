package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	conv "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/engineering/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ResolutionText is approved product copy. Agent-supplied arbitrary templates
// cannot turn a release into a bulk marketing send.
func resolutionGeneration(issue db.EngineeringIssue) string {
	return "done:" + issue.ResolvedAt.Time.UTC().Format(time.RFC3339Nano)
}

func ResolutionText(title string, release *db.EngineeringRelease) string {
	text := "Bildirdiğiniz sorun düzeltildi: " + title + ". Yardımınız için teşekkür ederiz."
	if release != nil {
		text += " Lütfen " + release.Version + " sürümüne güncelleyin."
		if release.ChangelogUrl != "" {
			text += " " + release.ChangelogUrl
		}
	}
	return text
}

func eligible(link db.EngineeringLinksRow) (bool, string) {
	if link.ContactBlocked {
		return false, "contact_blocked"
	}
	if !link.NotifyCustomer {
		return false, "conversation_opt_out"
	}
	var fields map[string]any
	if json.Unmarshal(link.ContactFields, &fields) != nil {
		return false, "invalid_preferences"
	}
	if fields["engineering_notifications"] == false || fields["notifications_opt_out"] == true {
		return false, "contact_opt_out"
	}
	if !link.ChannelInstanceID.Valid {
		return false, "no_channel"
	}
	return true, ""
}

func (s *Service) queueResolution(ctx context.Context, q *db.Queries, issue db.EngineeringIssue, actor int64, generation string, release *db.EngineeringRelease) error {
	links, err := q.EngineeringLinks(ctx, db.EngineeringLinksParams{OrganizationID: issue.OrganizationID, IssueID: issue.ID})
	if err != nil {
		return err
	}
	var rid pgtype.Int8
	if release != nil {
		rid = nullable(release.ID)
	}
	for _, link := range links {
		ok, _ := eligible(link)
		if !ok {
			continue
		}
		if err = q.EngineeringCreateDelivery(ctx, db.EngineeringCreateDeliveryParams{OrganizationID: issue.OrganizationID, IssueID: issue.ID, ConversationID: link.ConversationID, ReleaseID: rid, Generation: generation, Content: ResolutionText(issue.Title, release), ActorUserID: nullable(actor), ResolutionAt: issue.ResolvedAt}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Preview(ctx context.Context, org, actor int64, id uuid.UUID) ([]model.Preview, error) {
	issue, err := s.Store.Q.EngineeringIssue(ctx, db.EngineeringIssueParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return nil, missing(err)
	}
	links, err := s.Store.Q.EngineeringLinks(ctx, db.EngineeringLinksParams{OrganizationID: org, IssueID: issue.ID})
	if err != nil {
		return nil, err
	}
	out := []model.Preview{}
	for _, link := range links {
		if _, err = s.Inbox.Get(ctx, org, actor, link.ConversationUuid); errors.Is(err, conv.ErrNotFound) {
			continue
		} else if err != nil {
			return nil, err
		}
		ok, reason := eligible(link)
		out = append(out, model.Preview{Conversation: link.ConversationUuid, Channel: link.ChannelType, Eligible: ok, Reason: reason, Content: ResolutionText(issue.Title, nil)})
	}
	return out, nil
}

// Sweep durably retries customer follow-ups. Deterministic client IDs make a
// crash after Post safe. The inbox performs channel/window checks at delivery.
func (s *Service) Sweep(ctx context.Context) error {
	rows, err := s.Store.Q.EngineeringClaimDeliveries(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, d := range rows {
		if err = s.deliver(ctx, d); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *Service) deliver(ctx context.Context, d db.EngineeringDelivery) error {
	q := s.Store.Q
	finish := func(status string, message uuid.UUID, reason string) error {
		return q.EngineeringFinishDelivery(ctx, db.EngineeringFinishDeliveryParams{OrganizationID: d.OrganizationID, ID: d.ID, Status: status, MessageUuid: pgtype.UUID{Bytes: message, Valid: message != uuid.Nil}, LastError: reason})
	}
	allowed, err := q.EngineeringDeliveryAllowed(ctx, db.EngineeringDeliveryAllowedParams{OrganizationID: d.OrganizationID, Uuid: d.Uuid})
	if err != nil {
		return err
	}
	if !allowed {
		return finish("skipped", uuid.Nil, "opt_out_or_unlinked")
	}
	c, err := q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: d.OrganizationID, ID: d.ConversationID})
	if err != nil {
		return err
	}
	actor := conv.Actor{ID: d.ActorUserID.Int64, Name: "ClickFix"}
	if actor.ID > 0 {
		if u, e := q.GetUserByID(ctx, actor.ID); e == nil {
			actor.UUID = u.Uuid
			actor.Name = strings.TrimSpace(u.Name + " " + u.Surname)
		}
	}
	var template *channels.Template
	if c.ChannelType == channels.TypeWhatsApp && c.ChannelInstanceID.Valid {
		instance, e := q.GetChannelInstanceByID(ctx, db.GetChannelInstanceByIDParams{OrganizationID: d.OrganizationID, ID: c.ChannelInstanceID.Int64})
		if e != nil {
			return e
		}
		template, err = followupTemplate(instance.Settings, d.Generation, d.Content)
		if err != nil {
			return err
		}
	}
	posted, err := s.Inbox.Post(ctx, conv.PostInput{OrganizationID: d.OrganizationID, ConversationUUID: c.Uuid, AuthorID: actor.ID, AuthorUUID: actor.UUID, AuthorName: actor.Name, Content: d.Content, ClientID: d.Uuid, EngineeringDelivery: d.Uuid, Template: template})
	if err != nil {
		status := "pending"
		if d.Attempts >= 8 || errors.Is(err, conv.ErrNoChannel) || errors.Is(err, conv.ErrNotFound) {
			status = "failed"
		}
		if e := finish(status, uuid.Nil, "follow_up_failed"); e != nil {
			return e
		}
		return err
	}
	if c.Status == "resolved" || c.Status == "closed" {
		status := "open"
		if _, err = s.Inbox.Update(ctx, conv.UpdateInput{OrganizationID: d.OrganizationID, ConversationUUID: c.Uuid, Actor: actor, Status: &status}); err != nil {
			return err
		}
	}
	return finish("queued", posted.Message.UUID, "")
}

func validURL(raw string) bool {
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

func (s *Service) CreateRelease(ctx context.Context, org int64, version, changelog string) (db.EngineeringRelease, error) {
	if strings.TrimSpace(version) == "" || len(version) > 120 || len(changelog) > 2000 || !validURL(changelog) {
		return db.EngineeringRelease{}, invalid("invalid version or HTTPS changelog URL")
	}
	return s.Store.Q.EngineeringCreateRelease(ctx, db.EngineeringCreateReleaseParams{OrganizationID: org, Version: strings.TrimSpace(version), ChangelogUrl: changelog})
}

func (s *Service) PublishRelease(ctx context.Context, org, actor int64, id uuid.UUID) (db.EngineeringRelease, error) {
	var release db.EngineeringRelease
	err := s.Store.InTx(ctx, func(ctx context.Context, q *db.Queries) error {
		var err error
		release, err = q.EngineeringPublishRelease(ctx, db.EngineeringPublishReleaseParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return missing(err)
		}
		issues, err := q.EngineeringReleaseIssues(ctx, db.EngineeringReleaseIssuesParams{OrganizationID: org, ReleaseID: nullable(release.ID)})
		if err != nil {
			return err
		}
		for _, issue := range issues {
			if issue.Status == "canceled" {
				continue
			}
			if err = s.queueResolution(ctx, q, issue, actor, "release:"+id.String(), &release); err != nil {
				return err
			}
			if err = emit(ctx, q, issue, actor, "released"); err != nil {
				return err
			}
		}
		return nil
	})
	return release, err
}

func (s *Service) Escalate(ctx context.Context, org int64, actor conv.Actor, conversation uuid.UUID, in model.EscalationInput) (db.EngineeringEscalation, error) {
	if _, err := s.Inbox.Get(ctx, org, actor.ID, conversation); err != nil {
		return db.EngineeringEscalation{}, missing(err)
	}
	if strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 2000 || in.Team == uuid.Nil || in.SLASeconds < 60 || in.SLASeconds > 30*86400 {
		return db.EngineeringEscalation{}, invalid("team, reason and SLA seconds (60..2592000) are required")
	}
	var result db.EngineeringEscalation
	err := s.Store.InTx(ctx, func(ctx context.Context, q *db.Queries) error {
		c, err := q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: conversation})
		if err != nil {
			return missing(err)
		}
		c, err = q.LockConversation(ctx, db.LockConversationParams{OrganizationID: org, ID: c.ID})
		if err != nil {
			return err
		}
		history, err := q.EngineeringEscalations(ctx, db.EngineeringEscalationsParams{OrganizationID: org, ConversationID: c.ID})
		if err != nil {
			return err
		}
		previous := "l1"
		if len(history) > 0 {
			previous = history[0].Tier
		}
		if (previous != "l1" || in.Tier != "l2") && (previous != "l2" || in.Tier != "engineering") {
			return invalid("handoff must advance l1 → l2 → engineering")
		}
		team, err := q.GetTeamRefByUUID(ctx, db.GetTeamRefByUUIDParams{OrganizationID: org, Uuid: in.Team})
		if err != nil {
			return missing(err)
		}
		due := pgtype.Timestamptz{Time: time.Now().UTC().Add(time.Duration(in.SLASeconds) * time.Second), Valid: true}
		result, err = q.EngineeringCreateEscalation(ctx, db.EngineeringCreateEscalationParams{OrganizationID: org, ConversationID: c.ID, Tier: in.Tier, TeamID: nullable(team.ID), Reason: in.Reason, DueAt: due, ActorUserID: nullable(actor.ID)})
		if err != nil {
			return err
		}
		if err = q.EngineeringHandoffConversation(ctx, db.EngineeringHandoffConversationParams{OrganizationID: org, ID: c.ID, AssigneeTeamID: nullable(team.ID), ResolutionDueAt: due}); err != nil {
			return err
		}
		now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		warn := pgtype.Timestamptz{Time: due.Time.Add(-time.Duration(in.SLASeconds) * time.Second / 5), Valid: true}
		if err = q.UpsertConversationSLAMetric(ctx, db.UpsertConversationSLAMetricParams{OrganizationID: org, ConversationID: c.ID, Metric: "resolution", TargetSeconds: int64(in.SLASeconds), Cycle: safeconv.ToInt32(len(history) + 1), StartedAt: now, ResumedAt: now, WarnAt: warn, DueAt: due}); err != nil {
			return err
		}
		ev := events.New("engineering.escalated").WithOrganization(org).WithActor(actor.ID).WithEntity("conversations", &c.ID, &c.Uuid).WithPayload(map[string]any{"conversation_uuid": c.Uuid, "tier": in.Tier, "team_uuid": in.Team, "due_at": due.Time, "reason": in.Reason})
		return outbox.EnqueueQueries(ctx, q, ev)
	})
	return result, err
}

func (s *Service) Comment(ctx context.Context, org, actor int64, id uuid.UUID, body string) (db.EngineeringComment, error) {
	if strings.TrimSpace(body) == "" || len([]rune(body)) > 10000 {
		return db.EngineeringComment{}, invalid("comment must be 1..10000 characters")
	}
	var result db.EngineeringComment
	err := s.Store.InTx(ctx, func(ctx context.Context, q *db.Queries) error {
		issue, err := q.EngineeringLockIssue(ctx, db.EngineeringLockIssueParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return missing(err)
		}
		result, err = q.EngineeringAddComment(ctx, db.EngineeringAddCommentParams{OrganizationID: org, IssueID: issue.ID, AuthorUserID: nullable(actor), Body: strings.TrimSpace(body), Source: "native"})
		if err != nil {
			return err
		}
		return emit(ctx, q, issue, actor, "commented")
	})
	return result, err
}

var (
	followupTemplateName     = regexp.MustCompile(`^[a-z0-9_]{1,512}$`)
	followupTemplateLanguage = regexp.MustCompile(`^[a-z]{2,3}(_[A-Z]{2})?$`)
)

// Channel settings select an already approved WhatsApp template with one body
// parameter: the fixed resolution/release copy. Provider approval stays in Meta.
func followupTemplate(settings []byte, generation, content string) (*channels.Template, error) {
	var cfg struct {
		Followup struct {
			Resolution string `json:"resolution_template"`
			Release    string `json:"release_template"`
			Language   string `json:"language"`
		} `json:"engineering_followup"`
	}
	if err := json.Unmarshal(settings, &cfg); err != nil {
		return nil, invalid("invalid channel follow-up settings")
	}
	name := cfg.Followup.Resolution
	if strings.HasPrefix(generation, "release:") && cfg.Followup.Release != "" {
		name = cfg.Followup.Release
	}
	if name == "" {
		return nil, nil
	}
	language := cfg.Followup.Language
	if language == "" {
		language = "tr"
	}
	if !followupTemplateName.MatchString(name) || !followupTemplateLanguage.MatchString(language) {
		return nil, invalid("invalid approved template name or language")
	}
	return &channels.Template{Name: name, Language: language, Parameters: []string{content}}, nil
}
