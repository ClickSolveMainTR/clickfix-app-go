// Package eventconsumers is the single list of durable domain-event
// consumers. The API process fans out to them and the worker runs them, so
// both build the registry from here.
package eventconsumers

import (
	agentusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/usecase"
	analyticsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics"
	analyticsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/autoreply"
	categoriesusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/categories/usecase"
	kbusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/usecase"
	mobileextusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mobileext/usecase"
	notifmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications"
	notifusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/oncall"
	reportsuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/reports/usecase"
	routingmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing"
	routingusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/usecase"
	slamodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla"
	slausecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/usecase"
	statusmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage"
	statususecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage/usecase"
	surveysmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys"
	surveysusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
)

// Deps are the services consumers need. Fields may be nil in processes that
// only fan out; handlers run in the worker.
type Deps struct {
	Oncall    *oncall.Service
	Reports   *reportsuc.Service
	Agents    *agentusecase.Service
	Knowledge *kbusecase.RAG
	Status    *statususecase.Service
	Surveys   *surveysusecase.Service
	Analytics *analyticsusecase.Service
	// Webhooks delivers organization events to outgoing webhooks (CF-95).
	Webhooks eventdispatch.WebhookSink
	// Notifications turns helpdesk events into user notifications (CF-9).
	Notifications       *notifusecase.Service
	NotificationsLookup notifmodule.Lookup
	// SLA keeps conversation SLA clocks in step with the inbox (CF-46).
	SLA *slausecase.Service
	// AutoReply acknowledges inbound messages and posts out-of-hours
	// notices (CF-198).
	AutoReply *autoreply.Service
	// Routing assigns conversations automatically (CF-53).
	Routing *routingusecase.Service
	// LiveActivities sends iOS Live Activity pushes (CF-152); registered
	// only when APNs is configured.
	LiveActivities *mobileextusecase.Service
	// Categories classifies new conversations (CF-218).
	Categories *categoriesusecase.Service
}

// New builds the consumer registry.
func New(deps Deps) (*eventdispatch.Registry, error) {
	consumers := []eventdispatch.Consumer{}
	if deps.Oncall != nil {
		consumers = append(consumers, deps.Oncall.Consumer())
	}
	if deps.Reports != nil {
		consumers = append(consumers, deps.Reports.Consumer())
	}
	if deps.Agents != nil {
		consumers = append(consumers, deps.Agents.Consumer())
	}
	if deps.Knowledge != nil {
		consumers = append(consumers, deps.Knowledge.Consumer())
	}
	if deps.Status != nil {
		consumers = append(consumers, statusmodule.EventConsumer(deps.Status))
	}
	if deps.Surveys != nil {
		consumers = append(consumers, surveysmodule.EventConsumer(deps.Surveys))
	}
	if deps.Analytics != nil {
		consumers = append(consumers, analyticsmodule.EventConsumer(deps.Analytics))
	}
	if deps.Notifications != nil && deps.NotificationsLookup != nil {
		consumers = append(consumers, notifmodule.EventConsumer(deps.Notifications, deps.NotificationsLookup))
	}
	if deps.SLA != nil {
		consumers = append(consumers, slamodule.EventConsumer(deps.SLA))
	}
	if deps.AutoReply != nil {
		consumers = append(consumers, deps.AutoReply.Consumer())
	}
	if deps.Routing != nil {
		consumers = append(consumers, routingmodule.EventConsumer(deps.Routing))
	}
	if deps.Categories != nil {
		consumers = append(consumers, deps.Categories.Consumer())
	}
	if deps.LiveActivities.PushConfigured() {
		consumers = append(consumers, deps.LiveActivities.Consumer())
	}
	if deps.Webhooks != nil {
		consumers = append(consumers, eventdispatch.WebhookConsumer(deps.Webhooks))
	}
	return eventdispatch.NewRegistry(consumers...)
}
