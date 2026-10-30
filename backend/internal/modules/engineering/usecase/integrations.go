package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type IntegrationInput struct {
	Config      IntegrationConfig `json:"config"`
	Credentials *Credentials      `json:"credentials,omitempty"`
	Enabled     bool              `json:"enabled"`
}
type Integration struct {
	UUID              uuid.UUID         `json:"uuid"`
	Provider          string            `json:"provider"`
	Config            IntegrationConfig `json:"config"`
	Enabled           bool              `json:"enabled"`
	Connected         bool              `json:"connected"`
	WebhookConfigured bool              `json:"webhook_configured"`
	OAuthConfigured   bool              `json:"oauth_configured"`
}

func (s *Service) open(row db.EngineeringIntegration) (Credentials, IntegrationConfig, error) {
	var c Credentials
	var cfg IntegrationConfig
	if s.Box == nil {
		return c, cfg, ErrUnavailable
	}
	raw, err := s.Box.Decrypt(row.CredentialsEnc)
	if err != nil {
		return c, cfg, ErrUnavailable
	}
	if err = json.Unmarshal([]byte(raw), &c); err != nil {
		return c, cfg, err
	}
	err = json.Unmarshal(row.Config, &cfg)
	return c, cfg, err
}
func (s *Service) integrationDTO(row db.EngineeringIntegration) (Integration, error) {
	c, cfg, err := s.open(row)
	return Integration{UUID: row.Uuid, Provider: row.Provider, Config: cfg, Enabled: row.Enabled, Connected: c.AccessToken != "", WebhookConfigured: c.WebhookSecret != "", OAuthConfigured: c.ClientID != "" && c.ClientSecret != ""}, err
}
func (s *Service) Integrations(ctx context.Context, org int64) ([]Integration, error) {
	rows, err := s.Store.Q.EngineeringIntegrations(ctx, org)
	if err != nil {
		return nil, err
	}
	out := []Integration{}
	for _, r := range rows {
		d, e := s.integrationDTO(r)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, nil
}
func (s *Service) saveIntegration(ctx context.Context, q *db.Queries, org int64, provider string, c Credentials, cfg IntegrationConfig, enabled bool) (db.EngineeringIntegration, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return db.EngineeringIntegration{}, err
	}
	sealed, err := s.Box.Encrypt(string(raw))
	if err != nil {
		return db.EngineeringIntegration{}, err
	}
	config, err := json.Marshal(cfg)
	if err != nil {
		return db.EngineeringIntegration{}, err
	}
	return q.EngineeringIntegrationSave(ctx, db.EngineeringIntegrationSaveParams{OrganizationID: org, Provider: provider, CredentialsEnc: sealed, Config: config, Enabled: enabled})
}

// SaveIntegration omits secrets on read; omitted/empty secret fields preserve
// stored values, following modules/integrations/github's credential pattern.
func (s *Service) SaveIntegration(ctx context.Context, org int64, provider string, in IntegrationInput) (Integration, error) {
	if s.Adapters[provider] == nil && provider != "sentry" {
		return Integration{}, invalid("unknown provider")
	}
	if s.Box == nil {
		return Integration{}, ErrUnavailable
	}
	if in.Credentials != nil && in.Credentials.WebhookSecret != "" && len(in.Credentials.WebhookSecret) < 16 {
		return Integration{}, invalid("webhook_secret must contain at least 16 bytes")
	}

	if in.Config.RedirectURI != "" && !validURL(in.Config.RedirectURI) {
		return Integration{}, invalid("redirect_uri must use HTTPS")
	}
	if in.Config.SiteURL != "" && !validURL(in.Config.SiteURL) {
		return Integration{}, invalid("site_url must use HTTPS")
	}
	if provider == "github" && !repositoryPattern.MatchString(in.Config.Repository) {
		return Integration{}, invalid("repository must be owner/name")
	}
	if provider == "jira" && !externalIDPattern.MatchString(in.Config.CloudID) {
		return Integration{}, invalid("cloud_id is required")
	}
	for status := range in.Config.States {
		if !validStatus(status) {
			return Integration{}, invalid("unknown local state mapping")
		}
	}
	var result db.EngineeringIntegration
	err := s.Store.InTx(ctx, func(ctx context.Context, q *db.Queries) error {
		var c Credentials
		old, err := q.EngineeringLockIntegration(ctx, db.EngineeringLockIntegrationParams{OrganizationID: org, Provider: provider})
		if err == nil {
			c, _, err = s.open(old)
			if err != nil {
				return err
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if next := in.Credentials; next != nil {
			if next.AccessToken != "" {
				c.AccessToken = next.AccessToken
				c.ExpiresAt = next.ExpiresAt
			}
			if next.RefreshToken != "" {
				c.RefreshToken = next.RefreshToken
			}
			if next.ClientID != "" {
				c.ClientID = next.ClientID
			}
			if next.ClientSecret != "" {
				c.ClientSecret = next.ClientSecret
			}
			if next.WebhookSecret != "" {
				c.WebhookSecret = next.WebhookSecret
			}
		}
		result, err = s.saveIntegration(ctx, q, org, provider, c, in.Config, in.Enabled)
		return err
	})
	if err != nil {
		return Integration{}, err
	}
	return s.integrationDTO(result)
}
func validStatus(s string) bool {
	switch s {
	case "backlog", "todo", "in_progress", "done", "canceled":
		return true
	}
	return false
}
func stateHash(state string) string {
	h := sha256.Sum256([]byte(state))
	return hex.EncodeToString(h[:])
}
func oauthEndpoints(provider string) (string, string, string) {
	switch provider {
	case "linear":
		return "https://linear.app/oauth/authorize", "https://api.linear.app/oauth/token", "read,write,issues:create,comments:create"
	case "github":
		return "https://github.com/login/oauth/authorize", "https://github.com/login/oauth/access_token", "repo"
	case "jira":
		return "https://auth.atlassian.com/authorize", "https://auth.atlassian.com/oauth/token", "read:jira-work write:jira-work offline_access"
	}
	return "", "", ""
}
func (s *Service) StartOAuth(ctx context.Context, org, user int64, provider string) (string, error) {
	row, err := s.Store.Q.EngineeringIntegration(ctx, db.EngineeringIntegrationParams{OrganizationID: org, Provider: provider})
	if err != nil {
		return "", missing(err)
	}
	c, cfg, err := s.open(row)
	if err != nil {
		return "", err
	}
	authorize, _, scope := oauthEndpoints(provider)
	if !row.Enabled || authorize == "" || c.ClientID == "" || c.ClientSecret == "" || !validURL(cfg.RedirectURI) || cfg.RedirectURI == "" {
		return "", invalid("OAuth client credentials and HTTPS redirect_uri are required")
	}
	var entropy [32]byte
	if _, err = rand.Read(entropy[:]); err != nil {
		return "", err
	}
	state := base64.RawURLEncoding.EncodeToString(entropy[:])
	if err = s.Store.Q.EngineeringOAuthState(ctx, db.EngineeringOAuthStateParams{OrganizationID: org, IntegrationID: row.ID, UserID: user, StateHash: stateHash(state), ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(10 * time.Minute), Valid: true}}); err != nil {
		return "", err
	}
	values := url.Values{"client_id": {c.ClientID}, "redirect_uri": {cfg.RedirectURI}, "response_type": {"code"}, "state": {state}, "scope": {scope}}
	if provider == "jira" {
		values.Set("audience", "api.atlassian.com")
		values.Set("prompt", "consent")
	}
	return authorize + "?" + values.Encode(), nil
}

// OAuthExchange is replaceable in tests through the adapter's HTTP client; it
// never accepts an arbitrary token endpoint from an organization.
func (s *Service) token(ctx context.Context, provider string, c Credentials, cfg IntegrationConfig, values url.Values) (Credentials, error) {
	_, endpoint, _ := oauthEndpoints(provider)
	if endpoint == "" {
		return c, ErrUnavailable
	}
	values.Set("client_id", c.ClientID)
	values.Set("client_secret", c.ClientSecret)
	var body io.Reader = strings.NewReader(values.Encode())
	contentType := "application/x-www-form-urlencoded"
	if provider == "jira" {
		m := map[string]string{}
		for key := range values {
			m[key] = values.Get(key)
		}
		raw, err := json.Marshal(m)
		if err != nil {
			return c, err
		}
		body = strings.NewReader(string(raw))
		contentType = "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, body)
	if err != nil {
		return c, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	adapter, ok := s.Adapters[provider].(*providerAdapter)
	if !ok {
		return c, ErrUnavailable
	}
	resp, err := adapter.Client.Do(req)
	if err != nil {
		return c, ErrUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c, ErrUnavailable
	}
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Error        string `json:"error"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&result); err != nil {
		return c, ErrUnavailable
	}
	if result.AccessToken == "" || result.Error != "" {
		return c, ErrUnavailable
	}
	c.AccessToken = result.AccessToken
	if result.RefreshToken != "" {
		c.RefreshToken = result.RefreshToken
	}
	c.ExpiresAt = time.Time{}
	if result.ExpiresIn > 0 {
		c.ExpiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	}
	return c, nil
}
func (s *Service) CompleteOAuth(ctx context.Context, org, user int64, provider, state, code string) (Integration, error) {
	if len(state) < 32 || len(state) > 256 || code == "" || len(code) > 4096 {
		return Integration{}, invalid("invalid OAuth state or code")
	}
	row, err := s.Store.Q.EngineeringIntegration(ctx, db.EngineeringIntegrationParams{OrganizationID: org, Provider: provider})
	if err != nil {
		return Integration{}, missing(err)
	}
	if !row.Enabled {
		return Integration{}, ErrUnavailable
	}
	if _, err = s.Store.Q.EngineeringConsumeState(ctx, db.EngineeringConsumeStateParams{OrganizationID: org, IntegrationID: row.ID, UserID: user, StateHash: stateHash(state)}); err != nil {
		return Integration{}, invalid("OAuth state expired or already used")
	}
	c, cfg, err := s.open(row)
	if err != nil {
		return Integration{}, err
	}
	c, err = s.token(ctx, provider, c, cfg, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {cfg.RedirectURI}})
	if err != nil {
		return Integration{}, err
	}
	row, err = s.saveIntegration(ctx, s.Store.Q, org, provider, c, cfg, row.Enabled)
	if err != nil {
		return Integration{}, err
	}
	return s.integrationDTO(row)
}

// credentials must be called under the integration row lock. Rotating refresh
// tokens are persisted before another sync can use the old token.
func (s *Service) credentials(ctx context.Context, q *db.Queries, row db.EngineeringIntegration) (Credentials, IntegrationConfig, error) {
	c, cfg, err := s.open(row)
	if err != nil {
		return c, cfg, err
	}
	if !row.Enabled || c.AccessToken == "" {
		return c, cfg, ErrUnavailable
	}
	if !c.ExpiresAt.IsZero() && time.Now().Add(time.Minute).After(c.ExpiresAt) {
		if c.RefreshToken == "" {
			return c, cfg, ErrUnavailable
		}
		c, err = s.token(ctx, row.Provider, c, cfg, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {c.RefreshToken}})
		if err != nil {
			return c, cfg, err
		}
		if _, err = s.saveIntegration(ctx, q, row.OrganizationID, row.Provider, c, cfg, row.Enabled); err != nil {
			return c, cfg, err
		}
	}
	return c, cfg, nil
}

// ConnectIssue queues create/link and subsequent synchronization. No provider
// request is performed by the mutation's HTTP handler.
func (s *Service) ConnectIssue(ctx context.Context, org, actor int64, id uuid.UUID, provider, external string) (db.EngineeringExternalIssue, error) {
	if s.Adapters[provider] == nil {
		return db.EngineeringExternalIssue{}, invalid("unknown issue provider")
	}
	if external != "" && !externalIDPattern.MatchString(external) {
		return db.EngineeringExternalIssue{}, invalid("invalid external issue ID")
	}
	var result db.EngineeringExternalIssue
	err := s.Store.InTx(ctx, func(ctx context.Context, q *db.Queries) error {
		integration, err := q.EngineeringIntegration(ctx, db.EngineeringIntegrationParams{OrganizationID: org, Provider: provider})
		if err != nil {
			return missing(err)
		}
		if !integration.Enabled {
			return ErrUnavailable
		}
		issue, err := q.EngineeringLockIssue(ctx, db.EngineeringLockIssueParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return missing(err)
		}
		_, err = q.EngineeringExternalIssue(ctx, db.EngineeringExternalIssueParams{OrganizationID: org, IntegrationID: integration.ID, IssueID: issue.ID})
		if err == nil {
			return ErrConflict
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		result, err = q.EngineeringExternalSave(ctx, db.EngineeringExternalSaveParams{OrganizationID: org, IntegrationID: integration.ID, IssueID: issue.ID, ExternalID: external})
		if err != nil {
			return err
		}
		return emit(ctx, q, issue, actor, "sync_requested")
	})
	return result, err
}
func (s *Service) SyncSweep(ctx context.Context) error {
	// Also scans unchanged mappings because new native comments do not change an
	// issue's revision. Processing is bounded; updated_at provides rotation.
	rows, err := s.Store.Q.EngineeringAllExternal(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, r := range rows {
		if err = s.sync(ctx, r.OrganizationID, r.IssueUuid, r.Provider); err != nil {
			failures = append(failures, err)
			if e := s.Store.Q.EngineeringExternalError(ctx, db.EngineeringExternalErrorParams{OrganizationID: r.OrganizationID, ID: r.ID, LastError: "sync_failed"}); e != nil {
				failures = append(failures, e)
			}
		}
	}
	return errors.Join(failures...)
}
func (s *Service) refresh(ctx context.Context, org int64, provider string) error {
	return s.Store.InTx(ctx, func(ctx context.Context, q *db.Queries) error {
		row, err := q.EngineeringLockIntegration(ctx, db.EngineeringLockIntegrationParams{OrganizationID: org, Provider: provider})
		if err != nil {
			return err
		}
		_, _, err = s.credentials(ctx, q, row)
		return err
	})
}
func (s *Service) sync(ctx context.Context, org int64, id uuid.UUID, provider string) error {
	if err := s.refresh(ctx, org, provider); err != nil {
		return err
	}
	return s.Store.InTx(ctx, func(ctx context.Context, q *db.Queries) error {
		integration, err := q.EngineeringLockIntegration(ctx, db.EngineeringLockIntegrationParams{OrganizationID: org, Provider: provider})
		if err != nil {
			return missing(err)
		}
		creds, cfg, err := s.open(integration)
		if err != nil {
			return err
		}
		issue, err := q.EngineeringLockIssue(ctx, db.EngineeringLockIssueParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return missing(err)
		}
		mapping, err := q.EngineeringExternalIssue(ctx, db.EngineeringExternalIssueParams{OrganizationID: org, IntegrationID: integration.ID, IssueID: issue.ID})
		if err != nil {
			return missing(err)
		}
		dto, err := s.dto(ctx, q, issue)
		if err != nil {
			return err
		}
		adapter := s.Adapters[provider]
		if adapter == nil {
			return ErrUnavailable
		}
		remote := RemoteIssue{ID: mapping.ExternalID, Key: mapping.ExternalKey, URL: mapping.ExternalUrl}
		switch {
		case mapping.ExternalID == "":
			remote, err = adapter.Create(ctx, creds, cfg, dto)
		case mapping.SyncedRevision == 0:
			// Linking an existing provider issue adopts its current state instead of
			// overwriting it with a local placeholder on first synchronization.
			remote, err = adapter.Fetch(ctx, creds, cfg, mapping.ExternalID)
			if err == nil {
				issue, err = s.applyRemote(ctx, q, issue, remote, 0)
			}
		case mapping.SyncedRevision < issue.Revision:
			err = adapter.Update(ctx, creds, cfg, mapping.ExternalID, dto)
		}
		if err != nil {
			return err
		}
		if _, err = q.EngineeringExternalSave(ctx, db.EngineeringExternalSaveParams{OrganizationID: org, IntegrationID: integration.ID, IssueID: issue.ID, ExternalID: remote.ID, ExternalKey: remote.Key, ExternalUrl: remote.URL, SyncedRevision: issue.Revision}); err != nil {
			return err
		}
		comments, err := q.EngineeringCommentsToSync(ctx, db.EngineeringCommentsToSyncParams{OrganizationID: org, IssueID: issue.ID, IntegrationID: integration.ID})
		if err != nil {
			return err
		}
		for _, comment := range comments {
			body := comment.Body + "\n\nClickFix-Comment: " + comment.Uuid.String()
			if err = adapter.Comment(ctx, creds, cfg, remote.ID, body); err != nil {
				return err
			}
			if err = q.EngineeringCommentSyncMark(ctx, db.EngineeringCommentSyncMarkParams{OrganizationID: org, CommentID: comment.ID, IntegrationID: integration.ID}); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Service) applyRemote(ctx context.Context, q *db.Queries, cur db.EngineeringIssue, remote RemoteIssue, actor int64) (db.EngineeringIssue, error) {
	if remote.Title == "" || len([]rune(remote.Title)) > 240 || len(remote.Description) > 50000 || !validStatus(remote.Status) {
		return cur, invalid("invalid remote issue")
	}
	if cur.Title == remote.Title && cur.Description == remote.Description && cur.Status == remote.Status {
		return cur, nil
	}
	next, err := q.EngineeringUpdateIssue(ctx, db.EngineeringUpdateIssueParams{OrganizationID: cur.OrganizationID, ID: cur.ID, ExpectedRevision: cur.Revision, Title: remote.Title, Description: remote.Description, Status: remote.Status, Priority: cur.Priority, Labels: cur.Labels, Cycle: cur.Cycle, ProjectID: cur.ProjectID, AssigneeUserID: cur.AssigneeUserID, ReleaseID: cur.ReleaseID})
	if err != nil {
		return cur, err
	}
	if cur.Status != "done" && next.Status == "done" {
		if err = s.queueResolution(ctx, q, next, actor, resolutionGeneration(next), nil); err != nil {
			return next, err
		}
	}
	return next, emit(ctx, q, next, actor, "synced")
}

func (s *Service) Cleanup(ctx context.Context) error {
	return s.Store.Q.EngineeringPurgeOAuthStates(ctx)
}
