package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/engineering/model"
)

type Credentials struct {
	AccessToken   string    `json:"access_token,omitempty"`
	RefreshToken  string    `json:"refresh_token,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	ClientID      string    `json:"client_id,omitempty"`
	ClientSecret  string    `json:"client_secret,omitempty"`
	WebhookSecret string    `json:"webhook_secret,omitempty"`
}
type IntegrationConfig struct {
	TeamID      string `json:"team_id"`
	ProjectID   string `json:"project_id"`
	CloudID     string `json:"cloud_id"`
	Repository  string `json:"repository"`
	SiteURL     string `json:"site_url"`
	RedirectURI string `json:"redirect_uri"`
	IssueType   string `json:"issue_type"`
	// States maps ClickFix base statuses to provider state IDs (Linear/Jira).
	States      map[string]string `json:"states"`
	Transitions map[string]string `json:"transitions"`
}
type RemoteIssue struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// Adapter is shared by Linear, Jira Cloud and GitHub Issues. URLs are fixed in
// implementations; organization settings cannot select an arbitrary API host.
type Adapter interface {
	Create(context.Context, Credentials, IntegrationConfig, model.Issue) (RemoteIssue, error)
	Update(context.Context, Credentials, IntegrationConfig, string, model.Issue) error
	Fetch(context.Context, Credentials, IntegrationConfig, string) (RemoteIssue, error)
	Comment(context.Context, Credentials, IntegrationConfig, string, string) error
}
type providerAdapter struct {
	Provider string
	Client   *http.Client
}

func DefaultAdapters() map[string]Adapter {
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	return map[string]Adapter{"linear": &providerAdapter{Provider: "linear", Client: client}, "jira": &providerAdapter{Provider: "jira", Client: client}, "github": &providerAdapter{Provider: "github", Client: client}}
}
func (a *providerAdapter) request(ctx context.Context, c Credentials, method, endpoint string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if a.Provider == "github" {
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	resp, err := a.Client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s request failed", ErrUnavailable, a.Provider)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: %s returned HTTP %d", ErrUnavailable, a.Provider, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out); err != nil {
		return fmt.Errorf("%w: invalid %s response", ErrUnavailable, a.Provider)
	}
	return nil
}
func (a *providerAdapter) graphql(ctx context.Context, c Credentials, query string, variables, out any) error {
	var envelope struct {
		Data   json.RawMessage   `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := a.request(ctx, c, "POST", "https://api.linear.app/graphql", map[string]any{"query": query, "variables": variables}, &envelope); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 || len(envelope.Data) == 0 {
		return fmt.Errorf("%w: linear rejected operation", ErrUnavailable)
	}
	return json.Unmarshal(envelope.Data, out)
}

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var externalIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,120}$`)

func (a *providerAdapter) base(cfg IntegrationConfig) (string, error) {
	switch a.Provider {
	case "github":
		if !repositoryPattern.MatchString(cfg.Repository) {
			return "", invalid("repository must be owner/name")
		}
		return "https://api.github.com/repos/" + cfg.Repository, nil
	case "jira":
		if !externalIDPattern.MatchString(cfg.CloudID) {
			return "", invalid("Jira cloud_id is required")
		}
		return "https://api.atlassian.com/ex/jira/" + url.PathEscape(cfg.CloudID) + "/rest/api/3", nil
	}
	return "", invalid("unknown provider")
}
func adf(text string) map[string]any {
	return map[string]any{"type": "doc", "version": 1, "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}}}}
}
func adfText(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	var text string
	if t, ok := m["text"].(string); ok {
		text = t
	}
	if children, ok := m["content"].([]any); ok {
		for _, c := range children {
			text += adfText(c)
		}
	}
	if m["type"] == "paragraph" {
		text += "\n"
	}
	return text
}
func linearPriority(p string) int {
	switch p {
	case "urgent":
		return 1
	case "high":
		return 2
	case "low":
		return 4
	default:
		return 3
	}
}
func (a *providerAdapter) Create(ctx context.Context, c Credentials, cfg IntegrationConfig, in model.Issue) (RemoteIssue, error) {
	if a.Provider == "linear" {
		input := map[string]any{"id": in.UUID.String(), "teamId": cfg.TeamID, "title": in.Title, "description": in.Description, "priority": linearPriority(in.Priority)}
		if cfg.TeamID == "" {
			return RemoteIssue{}, invalid("Linear team_id is required")
		}
		if cfg.ProjectID != "" {
			input["projectId"] = cfg.ProjectID
		}
		if cfg.States[in.Status] != "" {
			input["stateId"] = cfg.States[in.Status]
		}
		var result struct {
			IssueCreate struct {
				Success bool                                        `json:"success"`
				Issue   struct{ ID, Identifier, URL, Title string } `json:"issue"`
			} `json:"issueCreate"`
		}
		err := a.graphql(ctx, c, `mutation CreateIssue($input: IssueCreateInput!){issueCreate(input:$input){success issue{id identifier url title}}}`, map[string]any{"input": input}, &result)
		if err != nil {
			return RemoteIssue{}, err
		}
		if !result.IssueCreate.Success {
			return RemoteIssue{}, ErrUnavailable
		}
		r := result.IssueCreate.Issue
		return RemoteIssue{ID: r.ID, Key: r.Identifier, URL: r.URL, Title: r.Title}, nil
	}
	base, err := a.base(cfg)
	if err != nil {
		return RemoteIssue{}, err
	}
	switch a.Provider {
	case "github":
		var r struct {
			Number  int64  `json:"number"`
			HTMLURL string `json:"html_url"`
			Title   string `json:"title"`
		}
		err = a.request(ctx, c, "POST", base+"/issues", map[string]any{"title": in.Title, "body": in.Description + "\n\n<!-- clickfix-issue:" + in.UUID.String() + " -->", "labels": in.Labels}, &r)
		return RemoteIssue{ID: strconv.FormatInt(r.Number, 10), Key: cfg.Repository + "#" + strconv.FormatInt(r.Number, 10), URL: r.HTMLURL, Title: r.Title}, err
	case "jira":
		if cfg.ProjectID == "" {
			return RemoteIssue{}, invalid("Jira project_id is required")
		}
		kind := cfg.IssueType
		if kind == "" {
			kind = "Task"
		}
		var r struct{ ID, Key string }
		err = a.request(ctx, c, "POST", base+"/issue", map[string]any{"fields": map[string]any{"project": map[string]any{"key": cfg.ProjectID}, "issuetype": map[string]any{"name": kind}, "summary": in.Title, "description": adf(in.Description + "\nClickFix-Issue: " + in.UUID.String()), "labels": in.Labels}}, &r)
		return RemoteIssue{ID: r.ID, Key: r.Key, URL: strings.TrimRight(cfg.SiteURL, "/") + "/browse/" + r.Key, Title: in.Title}, err
	}
	return RemoteIssue{}, ErrUnavailable
}
func (a *providerAdapter) Update(ctx context.Context, c Credentials, cfg IntegrationConfig, id string, in model.Issue) error {
	if !externalIDPattern.MatchString(id) {
		return invalid("invalid external issue ID")
	}
	if a.Provider == "linear" {
		input := map[string]any{"title": in.Title, "description": in.Description, "priority": linearPriority(in.Priority)}
		if cfg.States[in.Status] != "" {
			input["stateId"] = cfg.States[in.Status]
		}
		var r struct {
			IssueUpdate struct {
				Success bool `json:"success"`
			} `json:"issueUpdate"`
		}
		if err := a.graphql(ctx, c, `mutation UpdateIssue($id:String!,$input:IssueUpdateInput!){issueUpdate(id:$id,input:$input){success}}`, map[string]any{"id": id, "input": input}, &r); err != nil {
			return err
		}
		if !r.IssueUpdate.Success {
			return ErrUnavailable
		}
		return nil
	}
	base, err := a.base(cfg)
	if err != nil {
		return err
	}
	if a.Provider == "github" {
		state := "open"
		reason := "reopened"
		if in.Status == "done" || in.Status == "canceled" {
			state = "closed"
			reason = "completed"
			if in.Status == "canceled" {
				reason = "not_planned"
			}
		}
		return a.request(ctx, c, "PATCH", base+"/issues/"+id, map[string]any{"title": in.Title, "body": in.Description + "\n\n<!-- clickfix-issue:" + in.UUID.String() + " -->", "labels": in.Labels, "state": state, "state_reason": reason}, nil)
	}
	if err = a.request(ctx, c, "PUT", base+"/issue/"+id, map[string]any{"fields": map[string]any{"summary": in.Title, "description": adf(in.Description + "\nClickFix-Issue: " + in.UUID.String()), "labels": in.Labels}}, nil); err != nil {
		return err
	}
	if transition := cfg.Transitions[in.Status]; transition != "" {
		return a.request(ctx, c, "POST", base+"/issue/"+id+"/transitions", map[string]any{"transition": map[string]string{"id": transition}}, nil)
	}
	return nil
}
func (a *providerAdapter) Fetch(ctx context.Context, c Credentials, cfg IntegrationConfig, id string) (RemoteIssue, error) {
	if !externalIDPattern.MatchString(id) {
		return RemoteIssue{}, invalid("invalid external issue ID")
	}
	if a.Provider == "linear" {
		var r struct {
			Issue struct {
				ID, Identifier, URL, Title, Description string
				State                                   struct{ ID, Type string }
			}
		}
		if err := a.graphql(ctx, c, `query Issue($id:String!){issue(id:$id){id identifier url title description state{id type}}}`, map[string]string{"id": id}, &r); err != nil {
			return RemoteIssue{}, err
		}
		if r.Issue.ID == "" {
			return RemoteIssue{}, ErrNotFound
		}
		st := "todo"
		switch r.Issue.State.Type {
		case "backlog":
			st = "backlog"
		case "started":
			st = "in_progress"
		case "completed":
			st = "done"
		case "canceled":
			st = "canceled"
		}
		for local, remote := range cfg.States {
			if remote == r.Issue.State.ID {
				st = local
			}
		}
		return RemoteIssue{ID: r.Issue.ID, Key: r.Issue.Identifier, URL: r.Issue.URL, Title: r.Issue.Title, Description: r.Issue.Description, Status: st}, nil
	}
	base, err := a.base(cfg)
	if err != nil {
		return RemoteIssue{}, err
	}
	if a.Provider == "github" {
		var r struct {
			Number             int64
			Title, Body, State string
			StateReason        string          `json:"state_reason"`
			HTMLURL            string          `json:"html_url"`
			PullRequest        json.RawMessage `json:"pull_request"`
		}
		if err = a.request(ctx, c, "GET", base+"/issues/"+id, nil, &r); err != nil {
			return RemoteIssue{}, err
		}
		if len(r.PullRequest) > 0 {
			return RemoteIssue{}, invalid("pull requests are not issues")
		}
		st := "todo"
		if r.State == "closed" {
			st = "done"
			if r.StateReason == "not_planned" {
				st = "canceled"
			}
		}
		return RemoteIssue{ID: strconv.FormatInt(r.Number, 10), Key: cfg.Repository + "#" + id, URL: r.HTMLURL, Title: r.Title, Description: r.Body, Status: st}, nil
	}
	var r struct {
		ID, Key string
		Fields  struct {
			Summary     string
			Description any
			Status      struct {
				ID             string
				StatusCategory struct{ Key string }
			}
		}
	}
	if err = a.request(ctx, c, "GET", base+"/issue/"+id+"?fields=summary,description,status", nil, &r); err != nil {
		return RemoteIssue{}, err
	}
	st := "todo"
	switch r.Fields.Status.StatusCategory.Key {
	case "done":
		st = "done"
	case "indeterminate":
		st = "in_progress"
	}
	for local, remote := range cfg.States {
		if remote == r.Fields.Status.ID {
			st = local
		}
	}
	return RemoteIssue{ID: r.ID, Key: r.Key, URL: strings.TrimRight(cfg.SiteURL, "/") + "/browse/" + r.Key, Title: r.Fields.Summary, Description: strings.TrimSpace(adfText(r.Fields.Description)), Status: st}, nil
}
func (a *providerAdapter) Comment(ctx context.Context, c Credentials, cfg IntegrationConfig, id, body string) error {
	if pos := strings.LastIndex(body, "ClickFix-Comment: "); pos >= 0 {
		exists, err := a.commentExists(ctx, c, cfg, id, body[pos:])
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
	}
	if !externalIDPattern.MatchString(id) {
		return invalid("invalid external issue ID")
	}
	if a.Provider == "linear" {
		var r struct {
			CommentCreate struct {
				Success bool `json:"success"`
			} `json:"commentCreate"`
		}
		if err := a.graphql(ctx, c, `mutation Comment($input:CommentCreateInput!){commentCreate(input:$input){success}}`, map[string]any{"input": map[string]string{"issueId": id, "body": body}}, &r); err != nil {
			return err
		}
		if !r.CommentCreate.Success {
			return ErrUnavailable
		}
		return nil
	}
	base, err := a.base(cfg)
	if err != nil {
		return err
	}
	if a.Provider == "github" {
		return a.request(ctx, c, "POST", base+"/issues/"+id+"/comments", map[string]string{"body": body}, nil)
	}
	return a.request(ctx, c, "POST", base+"/issue/"+id+"/comment", map[string]any{"body": adf(body)}, nil)
}

// commentExists recovers an acknowledged provider comment after a local crash.
// Exhaust the bounded provider pagination before deciding a marker is absent.
func (a *providerAdapter) commentExists(ctx context.Context, c Credentials, cfg IntegrationConfig, id, marker string) (bool, error) {
	if marker == "" {
		return false, nil
	}
	var cursor *string
	for page := 1; page <= 50; page++ {
		switch a.Provider {
		case "linear":
			var r struct {
				Issue struct {
					Comments struct {
						Nodes    []struct{ Body string }
						PageInfo struct {
							HasNextPage bool
							EndCursor   string
						}
					}
				}
			}
			err := a.graphql(ctx, c, `query Comments($id:String!,$after:String){issue(id:$id){comments(first:100,after:$after){nodes{body} pageInfo{hasNextPage endCursor}}}}`, map[string]any{"id": id, "after": cursor}, &r)
			if err != nil {
				return false, err
			}
			for _, comment := range r.Issue.Comments.Nodes {
				if strings.Contains(comment.Body, marker) {
					return true, nil
				}
			}
			if !r.Issue.Comments.PageInfo.HasNextPage {
				return false, nil
			}
			next := r.Issue.Comments.PageInfo.EndCursor
			cursor = &next
		case "github":
			base, err := a.base(cfg)
			if err != nil {
				return false, err
			}
			var comments []struct{ Body string }
			if err = a.request(ctx, c, "GET", base+"/issues/"+id+"/comments?per_page=100&page="+strconv.Itoa(page), nil, &comments); err != nil {
				return false, err
			}
			for _, comment := range comments {
				if strings.Contains(comment.Body, marker) {
					return true, nil
				}
			}
			if len(comments) < 100 {
				return false, nil
			}
		case "jira":
			base, err := a.base(cfg)
			if err != nil {
				return false, err
			}
			var result struct {
				Comments []struct{ Body any }
				Total    int
			}
			if err = a.request(ctx, c, "GET", base+"/issue/"+id+"/comment?maxResults=100&startAt="+strconv.Itoa((page-1)*100), nil, &result); err != nil {
				return false, err
			}
			for _, comment := range result.Comments {
				if strings.Contains(adfText(comment.Body), marker) {
					return true, nil
				}
			}
			if page*100 >= result.Total {
				return false, nil
			}
		}
	}
	return false, fmt.Errorf("%w: comment pagination limit reached", ErrUnavailable)
}
