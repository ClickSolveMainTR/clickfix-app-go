package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrSignature = errors.New("engineering: invalid webhook signature")

// VerifySignature signs exact bytes and uses constant-time comparison. Linear's
// timestamp is inside those signed bytes; unsigned headers cannot extend its age.
func VerifySignature(provider, secret string, headers http.Header, raw []byte, now time.Time) error {
	if len(secret) < 16 {
		return ErrSignature
	}
	var signature string
	switch provider {
	case "sentry":
		signature = headers.Get("Sentry-Hook-Signature")
	case "linear":
		signature = headers.Get("Linear-Signature")
	case "github":
		signature = strings.TrimPrefix(headers.Get("X-Hub-Signature-256"), "sha256=")
		if !strings.HasPrefix(headers.Get("X-Hub-Signature-256"), "sha256=") {
			return ErrSignature
		}
	case "jira":
		signature = strings.TrimPrefix(headers.Get("X-Hub-Signature"), "sha256=")
		if !strings.HasPrefix(headers.Get("X-Hub-Signature"), "sha256=") {
			return ErrSignature
		}
	default:
		return ErrSignature
	}
	digest, err := hex.DecodeString(signature)
	if err != nil {
		return ErrSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, err = mac.Write(raw)
	if err != nil {
		return err
	}
	if !hmac.Equal(mac.Sum(nil), digest) {
		return ErrSignature
	}
	if provider == "linear" {
		var data struct {
			Timestamp int64 `json:"webhookTimestamp"`
		}
		if json.Unmarshal(raw, &data) != nil || data.Timestamp == 0 {
			return ErrSignature
		}
		age := now.Sub(time.UnixMilli(data.Timestamp))
		if age > time.Minute || age < -time.Minute {
			return ErrSignature
		}
	}
	return nil
}

type webhookChange struct {
	ExternalID, CommentID, Body string
	Deleted                     bool
	Ignore                      bool
}

func webhookPayload(provider string, cfg IntegrationConfig, headers http.Header, raw []byte) (webhookChange, error) {
	var change webhookChange
	switch provider {
	case "linear":
		var p struct {
			Type, Action string
			Data         struct{ ID, IssueID, Body string }
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return change, invalid("invalid Linear payload")
		}
		switch p.Type {
		case "Issue":
			change.ExternalID = p.Data.ID
			change.Deleted = p.Action == "remove"
		case "Comment":
			if p.Action == "remove" {
				change.Ignore = true
				break
			}
			change.ExternalID = p.Data.IssueID
			change.CommentID = p.Data.ID
			change.Body = p.Data.Body
		default:
			change.Ignore = true
		}
	case "github":
		var p struct {
			Action     string
			Repository struct {
				FullName string `json:"full_name"`
			}
			Issue struct {
				Number      int64
				PullRequest json.RawMessage `json:"pull_request"`
			}
			Comment struct {
				ID   int64
				Body string
			}
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return change, invalid("invalid GitHub payload")
		}
		if p.Repository.FullName != cfg.Repository || len(p.Issue.PullRequest) > 0 {
			change.Ignore = true
			return change, nil
		}
		switch headers.Get("X-GitHub-Event") {
		case "issues":
			change.ExternalID = strconv.FormatInt(p.Issue.Number, 10)
			change.Deleted = p.Action == "deleted"
		case "issue_comment":
			if p.Action == "deleted" {
				change.Ignore = true
				break
			}
			change.ExternalID = strconv.FormatInt(p.Issue.Number, 10)
			change.CommentID = strconv.FormatInt(p.Comment.ID, 10)
			change.Body = p.Comment.Body
		default:
			change.Ignore = true
		}
	case "sentry":
		var p struct {
			Action string
			Data   struct{ Issue struct{ ID string } }
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return change, invalid("invalid Sentry payload")
		}
		if headers.Get("Sentry-Hook-Resource") != "issue" {
			change.Ignore = true
			return change, nil
		}
		switch p.Action {
		case "resolved", "unresolved", "created", "ignored", "archived":
			change.ExternalID = p.Data.Issue.ID
		default:
			change.Ignore = true
		}
	case "jira":
		var p struct {
			Event string `json:"webhookEvent"`
			Issue struct {
				ID     string
				Fields struct{ Project struct{ Key string } }
			}
			Comment struct {
				ID   string
				Body any
			}
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return change, invalid("invalid Jira payload")
		}
		if p.Issue.Fields.Project.Key != "" && p.Issue.Fields.Project.Key != cfg.ProjectID {
			change.Ignore = true
			return change, nil
		}
		switch p.Event {
		case "jira:issue_created", "jira:issue_updated", "jira:issue_deleted":
			change.ExternalID = p.Issue.ID
			change.Deleted = p.Event == "jira:issue_deleted"
		case "comment_created", "comment_updated":
			change.ExternalID = p.Issue.ID
			change.CommentID = p.Comment.ID
			if text, ok := p.Comment.Body.(string); ok {
				change.Body = text
			} else {
				change.Body = strings.TrimSpace(adfText(p.Comment.Body))
			}
		default:
			change.Ignore = true
		}
	default:
		return change, invalid("unknown provider")
	}
	if !change.Ignore && !externalIDPattern.MatchString(change.ExternalID) {
		return change, invalid("missing external issue ID")
	}
	return change, nil
}
func (s *Service) Webhook(ctx context.Context, integrationUUID uuid.UUID, headers http.Header, raw []byte) error {
	row, err := s.Store.Q.EngineeringIntegrationByUUID(ctx, integrationUUID)
	if err != nil {
		return missing(err)
	}
	creds, cfg, err := s.open(row)
	if err != nil {
		return err
	}
	if err = VerifySignature(row.Provider, creds.WebhookSecret, headers, raw, time.Now()); err != nil {
		return err
	}
	change, err := webhookPayload(row.Provider, cfg, headers, raw)
	if err != nil {
		return err
	}
	if change.Ignore {
		return nil
	}
	// Body digest (not attacker-supplied delivery headers) also deduplicates a
	// valid signed body replayed with a different delivery ID.
	if change.CommentID == "" && !change.Deleted {
		if err = s.refresh(ctx, row.OrganizationID, row.Provider); err != nil {
			return err
		}
	}
	sum := sha256.Sum256(raw)
	key := hex.EncodeToString(sum[:])
	return s.Store.InTx(ctx, func(ctx context.Context, q *db.Queries) error {
		integration, err := q.EngineeringLockIntegration(ctx, db.EngineeringLockIntegrationParams{OrganizationID: row.OrganizationID, Provider: row.Provider})
		if err != nil {
			return err
		}
		if !integration.Enabled {
			return ErrUnavailable
		}
		n, err := q.EngineeringReceipt(ctx, db.EngineeringReceiptParams{OrganizationID: row.OrganizationID, IntegrationID: row.ID, DeliveryKey: key})
		if err != nil || n == 0 {
			return err
		}
		mapping, err := q.EngineeringExternalByID(ctx, db.EngineeringExternalByIDParams{OrganizationID: row.OrganizationID, IntegrationID: row.ID, ExternalID: change.ExternalID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		issue, err := q.EngineeringLockIssue(ctx, db.EngineeringLockIssueParams{OrganizationID: row.OrganizationID, Uuid: mapping.IssueUuid})
		if err != nil {
			return err
		}
		if change.CommentID != "" {
			if strings.Contains(change.Body, "ClickFix-Comment: ") {
				return nil
			}
			if strings.TrimSpace(change.Body) == "" || len([]rune(change.Body)) > 10000 {
				return invalid("invalid external comment")
			}
			if _, err = q.EngineeringAddComment(ctx, db.EngineeringAddCommentParams{OrganizationID: row.OrganizationID, IssueID: issue.ID, Source: row.Provider, ExternalID: change.CommentID, Body: change.Body}); err != nil {
				return err
			}
			return emit(ctx, q, issue, 0, "commented")
		}
		remote := RemoteIssue{Title: issue.Title, Description: issue.Description, Status: "canceled"}
		if !change.Deleted {
			// Refetch current provider state: delayed/out-of-order webhooks cannot
			// roll the issue back to the stale state carried in their payload.
			creds, cfg, err = s.open(integration)
			if err != nil {
				return err
			}
			if row.Provider == "sentry" {
				remote, err = s.sentryIssue(ctx, creds, change.ExternalID)
			} else {
				remote, err = s.Adapters[row.Provider].Fetch(ctx, creds, cfg, change.ExternalID)
			}
			if err != nil {
				return err
			}
		}
		if row.Provider == "sentry" {
			remote.Description = issue.Description
		}
		issue, err = s.applyRemote(ctx, q, issue, remote, 0)
		if err != nil {
			return err
		}
		return q.EngineeringWebhookSyncRevision(ctx, db.EngineeringWebhookSyncRevisionParams{OrganizationID: row.OrganizationID, IntegrationID: row.ID, IssueID: issue.ID, SyncedRevision: issue.Revision})
	})
}
