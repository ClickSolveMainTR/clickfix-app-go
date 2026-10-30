import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";
import { tenantRequest } from "@/lib/api/tenant-request";

type Schemas = components["schemas"];

export type AIAgentConfig = Schemas["AIAgentConfig"];
export type AIAgentMode = AIAgentConfig["mode"];
export type AIProcedureRecord = Schemas["AIProcedureRecord"];
export type AIProcedureInput = Schemas["AIProcedureInput"];
export type AIToolView = Schemas["AIToolView"];
export type AIToolInput = Schemas["AIToolInput"];
export type AIAgentToolCall = Schemas["AIAgentToolCall"];
export type AIRun = Schemas["AIRun"];
export type AIRunInput = Schemas["AIRunInput"];
export type AIRunResult = Schemas["AIRunResult"];
export type AICitation = Schemas["AICitation"];
export type AIPolicy = Schemas["AIPolicy"];
export type AIPolicyInput = Schemas["AIPolicyInput"];

export type AgentConfigState = { config: AIAgentConfig; revision: number };

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/** Built-in tools the agent always has (backend `aiagents/usecase/tools.go`). */
export const BUILTIN_TOOLS = [
  { name: "lookup_contact", requiresApproval: false },
  { name: "conversation_history", requiresApproval: false },
  { name: "knowledge_search", requiresApproval: false },
  { name: "create_note", requiresApproval: true },
  { name: "set_status", requiresApproval: true },
] as const;

const AI = "/v1/tenant/ai";

/**
 * Organization AI settings (CF-64, CF-67, CF-68, CF-69, CF-71). Every call is
 * organization-scoped through `X-Organization`; errors surface as `ApiError`
 * so callers can react to 409 (stale revision) and 422/429/503 AI errors.
 */
export const aiSettingsService = {
  agent(slug: string) {
    return tenantRequest<AgentConfigState>(slug, "GET", `${AI}/agent`);
  },
  saveAgent(slug: string, body: AIAgentConfig) {
    return tenantRequest<AgentConfigState>(slug, "PUT", `${AI}/agent`, {
      body,
      silent: true,
    });
  },

  procedures(slug: string) {
    return tenantRequest<Page<AIProcedureRecord>>(
      slug,
      "GET",
      `${AI}/procedures`,
      { query: { limit: 100 } },
    );
  },
  createProcedure(slug: string, body: AIProcedureInput) {
    return tenantRequest<AIProcedureRecord>(slug, "POST", `${AI}/procedures`, {
      body,
      silent: true,
    });
  },
  updateProcedure(slug: string, uuid: string, body: AIProcedureInput) {
    return tenantRequest<AIProcedureRecord>(
      slug,
      "PUT",
      `${AI}/procedures/${uuid}`,
      { body, silent: true },
    );
  },
  publishProcedure(
    slug: string,
    uuid: string,
    body: { revision: number; publish: boolean },
  ) {
    return tenantRequest<AIProcedureRecord>(
      slug,
      "POST",
      `${AI}/procedures/${uuid}/publish`,
      { body, silent: true },
    );
  },

  tools(slug: string) {
    return tenantRequest<Page<AIToolView>>(slug, "GET", `${AI}/tools`, {
      query: { limit: 100 },
    });
  },
  saveTool(slug: string, body: AIToolInput) {
    return tenantRequest<{ uuid: string }>(slug, "PUT", `${AI}/tools`, {
      body,
      silent: true,
    });
  },
  toolCalls(slug: string, offset: number, limit = 20) {
    return tenantRequest<Page<AIAgentToolCall>>(
      slug,
      "GET",
      `${AI}/tool-calls`,
      { query: { limit, offset } },
    );
  },

  runs(slug: string, offset: number, limit = 20) {
    return tenantRequest<Page<AIRun>>(slug, "GET", `${AI}/runs`, {
      query: { limit, offset },
    });
  },
  run(slug: string, uuid: string) {
    return tenantRequest<AIRun>(slug, "GET", `${AI}/runs/${uuid}`);
  },
  execute(slug: string, body: AIRunInput) {
    return tenantRequest<AIRunResult>(slug, "POST", `${AI}/runs`, {
      body,
      silent: true,
    });
  },

  policy(slug: string) {
    return tenantRequest<AIPolicy>(slug, "GET", `${AI}/policy`);
  },
  savePolicy(slug: string, body: AIPolicyInput) {
    return tenantRequest<AIPolicy>(slug, "PUT", `${AI}/policy`, {
      body,
      silent: true,
    });
  },
};

/** Platform admins read the same masked run log of one organization. */
export const platformAIRunsService = {
  runs(organization: string, offset: number, limit = 20) {
    return platformRequest<Page<AIRun>>(
      "GET",
      `/v1/platform/organizations/${organization}/ai/runs`,
      { query: { limit, offset } },
    );
  },
  run(organization: string, uuid: string) {
    return platformRequest<AIRun>(
      "GET",
      `/v1/platform/organizations/${organization}/ai/runs/${uuid}`,
    );
  },
};
