// Package aiagents registers organization-scoped AI workflow APIs.
package aiagents

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/middleware"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/handler"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"net/http"
)

func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	for _, route := range []struct {
		pattern    string
		fn         http.HandlerFunc
		permission string
	}{
		{"GET /v1/tenant/ai/business-context/meta", h.WorkflowMeta, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/ai/evaluation-cases/meta", h.WorkflowMeta, rbac.PermTenantAIInsightsRead},
		{"GET /v1/tenant/ai/evaluations/meta", h.WorkflowMeta, rbac.PermTenantAIInsightsRead},
		{"GET /v1/tenant/ai/knowledge-gaps/meta", h.WorkflowMeta, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/ai/knowledge-suggestions/meta", h.WorkflowMeta, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/knowledge/reviews/meta", h.WorkflowMeta, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/knowledge/sources/meta", h.WorkflowMeta, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/ai/business-context", h.BusinessContext, rbac.PermTenantSettingsRead},
		{"POST /v1/tenant/ai/business-context", h.BusinessContext, rbac.PermTenantSettingsWrite},
		{"GET /v1/tenant/ai/evaluation-cases", h.EvaluationCases, rbac.PermTenantAIInsightsRead},
		{"POST /v1/tenant/ai/evaluation-cases", h.EvaluationCases, rbac.PermTenantAIInsightsManage},
		{"PUT /v1/tenant/ai/evaluation-cases/{uuid}", h.UpdateEvaluationCase, rbac.PermTenantAIInsightsManage},
		{"GET /v1/tenant/ai/evaluations", h.Evaluations, rbac.PermTenantAIInsightsRead},
		{"POST /v1/tenant/ai/evaluations", h.Evaluations, rbac.PermTenantAIInsightsManage},
		{"GET /v1/tenant/ai/evaluations/watch", h.EvaluationWatch, rbac.PermTenantAIInsightsRead},
		{"PUT /v1/tenant/ai/evaluations/watch", h.EvaluationWatch, rbac.PermTenantAIInsightsManage},
		{"GET /v1/tenant/ai/knowledge-suggestions", h.KnowledgeSuggestions, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/ai/knowledge-gaps", h.KnowledgeGaps, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/ai/performance", h.Performance, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/knowledge/reviews", h.KnowledgeReviews, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/knowledge/sources", h.Sources, rbac.PermTenantSettingsRead},
		{"POST /v1/tenant/knowledge/sources", h.Sources, rbac.PermTenantSettingsWrite},
		{"PUT /v1/tenant/ai/business-context/{uuid}", h.BusinessContext, rbac.PermTenantSettingsWrite},
		{"GET /v1/tenant/ai/business-context/{uuid}/versions", h.BusinessContextVersions, rbac.PermTenantSettingsRead},
		{"POST /v1/tenant/ai/simulate", h.Simulation, rbac.PermTenantSettingsWrite},
		{"GET /v1/tenant/ai/evaluations/{uuid}", h.Evaluation, rbac.PermTenantAIInsightsRead},
		{"GET /v1/tenant/ai/evaluations/{uuid}/results", h.EvaluationResults, rbac.PermTenantAIInsightsRead},
		{"POST /v1/tenant/conversations/{uuid}/ai/knowledge-suggestion", h.SuggestKnowledge, rbac.PermTenantSettingsWrite},
		{"POST /v1/tenant/ai/knowledge-suggestions/{uuid}/review", h.ReviewSuggestion, rbac.PermTenantSettingsWrite},
		{"POST /v1/tenant/knowledge/articles/{uuid}/translate", h.TranslateArticle, rbac.PermTenantSettingsWrite},
		{"POST /v1/tenant/knowledge/articles/{uuid}/check-contradiction", h.CheckContradiction, rbac.PermTenantSettingsWrite},
		{"POST /v1/tenant/knowledge/articles/{uuid}/flag", h.FlagArticle, rbac.PermTenantSettingsWrite},
		{"POST /v1/tenant/knowledge/reviews/sweep", h.SweepReviews, rbac.PermTenantSettingsWrite},
		{"POST /v1/tenant/knowledge/reviews/{uuid}/complete", h.ReviewDone, rbac.PermTenantSettingsWrite},
		{"POST /v1/tenant/knowledge/sources/upload", h.SourceUpload, rbac.PermTenantSettingsWrite},
		{"POST /v1/tenant/knowledge/sources/{uuid}/sync", h.SourceSync, rbac.PermTenantSettingsWrite},
		{"PUT /v1/tenant/knowledge/sources/{uuid}", h.SourceToggle, rbac.PermTenantSettingsWrite},
		{"GET /v1/tenant/ai/procedures/meta", h.Meta, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/ai/tools/meta", h.Meta, rbac.PermTenantSettingsRead},
		{"GET /v1/tenant/ai/procedures", h.Procedures, rbac.PermTenantSettingsRead},
		{"POST /v1/tenant/ai/procedures", h.Procedures, rbac.PermTenantSettingsWrite},
		{"PUT /v1/tenant/ai/procedures/{uuid}", h.Procedures, rbac.PermTenantSettingsWrite},
		{"POST /v1/tenant/ai/procedures/{uuid}/publish", h.PublishProcedure, rbac.PermTenantSettingsWrite},
		{"GET /v1/tenant/ai/tools", h.Tools, rbac.PermTenantSettingsRead},
		{"PUT /v1/tenant/ai/tools", h.Tools, rbac.PermTenantSettingsWrite},
		{"GET /v1/tenant/ai/tool-calls", h.ToolCalls, rbac.PermTenantSettingsRead},
		{"POST /v1/tenant/conversations/{uuid}/ai/tools/invoke", h.InvokeTool, rbac.PermTenantConversationsRead},
		{"GET /v1/tenant/ai/agent", h.Config, rbac.PermTenantSettingsRead},
		{"PUT /v1/tenant/ai/agent", h.Config, rbac.PermTenantSettingsWrite},
		{"GET /v1/tenant/conversations/{uuid}/ai", h.Jobs, rbac.PermTenantConversationsRead},
		{"POST /v1/tenant/conversations/{uuid}/copilot", h.Copilot, rbac.PermTenantConversationsReply},
	} {
		mux.Handle(route.pattern, middleware.Chain(route.fn, middleware.Authenticate(tokens, loader), middleware.RequireOrganization(resolver), middleware.RequireTenantPermission(route.permission)))
	}
}
