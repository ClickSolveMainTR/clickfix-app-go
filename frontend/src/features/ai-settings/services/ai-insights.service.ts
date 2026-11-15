import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

import type { Page } from "./ai-settings.service";

type Schemas = components["schemas"];

export type AIKnowledgePanel = Schemas["AIKnowledgePanel"];
export type AIKnowledgeEvaluationPoint = Schemas["AIKnowledgeEvaluationPoint"];
export type AIEvaluationCase = Schemas["AIEvaluationCase"];
export type AIEvaluationCaseInput = Schemas["AIEvaluationCaseInput"];
export type AIEvaluation = Schemas["AIEvaluation"];
export type AIEvaluationWatch = Schemas["AIEvaluationWatch"];

const AI = "/v1/tenant/ai";

/**
 * Knowledge layer measurement (CF-220): the AI knowledge panel, the
 * evaluation set, run history and automatic runs. Reads need
 * `tenant.ai_insights.read`, changes `tenant.ai_insights.manage`.
 */
export const aiInsightsService = {
  panel(
    slug: string,
    query: { from?: string; until?: string; department_uuid?: string },
  ) {
    return tenantRequest<AIKnowledgePanel>(
      slug,
      "GET",
      "/v1/tenant/analytics/ai-knowledge",
      { query },
    );
  },
  cases(slug: string, offset: number, limit = 50) {
    return tenantRequest<Page<AIEvaluationCase>>(
      slug,
      "GET",
      `${AI}/evaluation-cases`,
      { query: { limit, offset } },
    );
  },
  createCase(slug: string, body: AIEvaluationCaseInput) {
    return tenantRequest<AIEvaluationCase>(
      slug,
      "POST",
      `${AI}/evaluation-cases`,
      { body, silent: true },
    );
  },
  updateCase(slug: string, uuid: string, body: AIEvaluationCaseInput) {
    return tenantRequest<AIEvaluationCase>(
      slug,
      "PUT",
      `${AI}/evaluation-cases/${uuid}`,
      { body, silent: true },
    );
  },
  evaluations(slug: string, offset: number, limit = 20) {
    return tenantRequest<Page<AIEvaluation>>(slug, "GET", `${AI}/evaluations`, {
      query: { limit, offset },
    });
  },
  run(slug: string, body: { department_uuid?: string } = {}) {
    return tenantRequest<AIEvaluation>(slug, "POST", `${AI}/evaluations`, {
      body,
      silent: true,
    });
  },
  watch(slug: string) {
    return tenantRequest<AIEvaluationWatch>(
      slug,
      "GET",
      `${AI}/evaluations/watch`,
    );
  },
  setWatch(slug: string, enabled: boolean) {
    return tenantRequest<AIEvaluationWatch>(
      slug,
      "PUT",
      `${AI}/evaluations/watch`,
      { body: { enabled }, silent: true },
    );
  },
};
