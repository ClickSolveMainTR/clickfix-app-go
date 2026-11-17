package usecase

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/jackc/pgx/v5/pgtype"
)

type AIFeedback interface {
	Satisfaction(context.Context, int64, time.Time, time.Time) (*float64, int64, error)
}
type TopicTrend struct {
	Topic         string   `json:"topic"`
	Conversations int      `json:"conversations"`
	Previous      int      `json:"previous"`
	GrowthPercent *float64 `json:"growth_percent"`
	Alert         bool     `json:"alert"`
}
type PerformanceReport struct {
	From                      time.Time         `json:"from"`
	Until                     time.Time         `json:"until"`
	Conversations             int               `json:"conversations"`
	Resolutions               int               `json:"resolutions"`
	Handoffs                  int               `json:"handoffs"`
	ResolutionRate            float64           `json:"resolution_rate"`
	HandoffRate               float64           `json:"handoff_rate"`
	HandoffReasons            map[string]int    `json:"handoff_reasons"`
	CostMicros                int64             `json:"cost_micros"`
	CostPerConversationMicros *float64          `json:"cost_per_conversation_micros"`
	CostPerResolutionMicros   *float64          `json:"cost_per_resolution_micros"`
	CSAT                      *float64          `json:"csat"`
	CSATSamples               int64             `json:"csat_samples"`
	EstimatedSavedHours       float64           `json:"estimated_saved_hours"`
	MinutesPerResolution      float64           `json:"minutes_per_resolution"`
	Topics                    []TopicTrend      `json:"topics"`
	Definitions               map[string]string `json:"definitions"`
}

func (s *Service) Performance(ctx context.Context, org int64, from, to time.Time, minutes float64) (out PerformanceReport, err error) {
	from, to, err = TimeRange(s.Now(), from, to)
	if err != nil {
		return out, err
	}
	if math.IsNaN(minutes) || math.IsInf(minutes, 0) || minutes <= 0 || minutes > 240 {
		return out, model.ErrInvalid
	}
	rows, err := s.Repo.Q.AIPerformanceJobs(ctx, db.AIPerformanceJobsParams{OrganizationID: org, Since: pgtype.Timestamptz{Time: from.Add(-to.Sub(from)), Valid: true}, Until: pgtype.Timestamptz{Time: to, Valid: true}})
	if err != nil {
		return out, err
	}
	if len(rows) > 50000 {
		return out, model.ErrConflict
	}
	resolved, err := s.Repo.Q.AIPerformanceResolutions(ctx, db.AIPerformanceResolutionsParams{OrganizationID: org, Since: pgtype.Timestamptz{Time: from, Valid: true}, Until: pgtype.Timestamptz{Time: to, Valid: true}})
	if err != nil {
		return out, err
	}
	latest := map[int64]db.AIPerformanceJobsRow{}
	currentTopics, previousTopics := map[string]map[int64]bool{}, map[string]map[int64]bool{}
	out = PerformanceReport{From: from, Until: to, HandoffReasons: map[string]int{}, Topics: []TopicTrend{}, MinutesPerResolution: minutes, Definitions: map[string]string{
		"denominator": "Distinct conversations with reply jobs in the requested interval; terminal outcome is latest reply job in that interval",
		"resolution":  "Delivered bot answer, resolved conversation and no human public reply; resolution recorded in this interval and conversation in denominator",
		"cost":        "Provider micro-currency cost of reply jobs in this interval; simulations and standalone copilot runs excluded",
		"csat":        "Mean 1–5 widget conversation rating for AI-only resolutions, rated in this interval; null when no samples exist",
		"saved_hours": "Estimate only: AI resolutions multiplied by supplied minutes_per_resolution / 60",
		"trend":       "Category from most recent completed triage; compare distinct conversations to preceding equal duration; alert at >=40% growth with >=5 current conversations",
	}}
	for _, r := range rows {
		target := currentTopics
		if r.CreatedAt.Time.Before(from) {
			target = previousTopics
		} else {
			latest[r.ConversationID] = r
			out.CostMicros += r.CostMicros
		}
		if target[r.Topic] == nil {
			target[r.Topic] = map[int64]bool{}
		}
		target[r.Topic][r.ConversationID] = true
	}
	out.Conversations = len(latest)
	for _, r := range latest {
		if r.Status == "handoff" || r.Status == "unknown" {
			out.Handoffs++
			out.HandoffReasons[strings.TrimSuffix(r.Reason, model.HandoffNoticeSent)]++
		}
	}
	for _, id := range resolved {
		if _, ok := latest[id]; ok {
			out.Resolutions++
		}
	}
	if out.Conversations > 0 {
		out.ResolutionRate = float64(out.Resolutions) / float64(out.Conversations)
		out.HandoffRate = float64(out.Handoffs) / float64(out.Conversations)
		v := float64(out.CostMicros) / float64(out.Conversations)
		out.CostPerConversationMicros = &v
	}
	if out.Resolutions > 0 {
		v := float64(out.CostMicros) / float64(out.Resolutions)
		out.CostPerResolutionMicros = &v
	}
	out.EstimatedSavedHours = float64(out.Resolutions) * minutes / 60
	for topic, conversations := range currentTopics {
		t := TopicTrend{Topic: topic, Conversations: len(conversations), Previous: len(previousTopics[topic])}
		if t.Previous > 0 {
			v := 100 * float64(t.Conversations-t.Previous) / float64(t.Previous)
			t.GrowthPercent = &v
			t.Alert = t.Conversations >= 5 && v >= 40
		}
		out.Topics = append(out.Topics, t)
	}
	sort.Slice(out.Topics, func(i, j int) bool {
		if out.Topics[i].Conversations == out.Topics[j].Conversations {
			return out.Topics[i].Topic < out.Topics[j].Topic
		}
		return out.Topics[i].Conversations > out.Topics[j].Conversations
	})
	feedback, e := s.Repo.Q.AIPerformanceCSAT(ctx, db.AIPerformanceCSATParams{OrganizationID: org, Since: pgtype.Timestamptz{Time: from, Valid: true}, Until: pgtype.Timestamptz{Time: to, Valid: true}})
	if e != nil {
		return out, e
	}
	out.CSATSamples = feedback.Samples
	if feedback.Samples > 0 {
		mean := float64(feedback.ScoreSum) / float64(feedback.Samples)
		out.CSAT = &mean
	}
	if s.Feedback != nil {
		out.CSAT, out.CSATSamples, err = s.Feedback.Satisfaction(ctx, org, from, to)
	}
	return out, err
}
