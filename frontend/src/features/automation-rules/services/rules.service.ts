import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type RuleInput = components["schemas"]["AutomationRuleInput"];
export type RuleAction = components["schemas"]["AutomationAction"];
export type RuleCondition = components["schemas"]["AutomationCondition"];

/**
 * Row of `GET /v1/tenant/automations` (sqlc `AutomationRule`). The API sends
 * `conditions` / `actions` as base64 of the stored JSON (Go `[]byte`);
 * `decodeRule` turns them back into objects.
 */
type RawRule = {
  id: number;
  uuid: string;
  name: string;
  trigger_event: string;
  conditions: string | RuleCondition;
  actions: string | RuleAction[];
  position: number;
  enabled: boolean;
  stop_on_match: boolean;
  schedule_minutes: number;
  next_run_at: string | null;
  created_at: string;
  updated_at: string;
};

export type Rule = Omit<RawRule, "conditions" | "actions"> & {
  conditions: RuleCondition;
  actions: RuleAction[];
};

/** Row of `GET /v1/tenant/automations/executions` (one rule × event). */
export type RuleExecution = {
  uuid: string;
  rule_id: number;
  event_id: string;
  conversation_uuid: string;
  status: "running" | "succeeded" | "failed" | "skipped";
  matched: boolean;
  error: string;
  action_index: number;
  created_at: string;
  finished_at: string | null;
};

export type DryRunResult = {
  matched: boolean;
  actions: unknown;
  dry_run: boolean;
};

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

function decodeJson<T>(v: unknown, fallback: T): T {
  if (typeof v !== "string") return (v as T) ?? fallback;
  try {
    const bin = atob(v);
    const bytes = Uint8Array.from(bin, (c) => c.charCodeAt(0));
    return JSON.parse(new TextDecoder().decode(bytes)) as T;
  } catch {
    try {
      return JSON.parse(v) as T;
    } catch {
      return fallback;
    }
  }
}

export function decodeRule(raw: RawRule): Rule {
  return {
    ...raw,
    conditions: decodeJson<RuleCondition>(raw.conditions, { all: [] }),
    actions: decodeJson<RuleAction[]>(raw.actions, []),
  };
}

/**
 * Automation rules (CF-94): ordered trigger → condition → action rules on the
 * event bus. Read with tenant.automations.read, write with
 * tenant.automations.write. Save and dry-run are silent so the editor shows
 * the validation message inline.
 */
export const rulesService = {
  list: async (slug: string, limit = 100, offset = 0) => {
    const page = await tenantRequest<Page<RawRule>>(
      slug,
      "GET",
      "/v1/tenant/automations",
      { query: { limit, offset } },
    );
    return { ...page, items: (page.items ?? []).map(decodeRule) };
  },
  get: async (slug: string, uuid: string) =>
    decodeRule(
      await tenantRequest<RawRule>(
        slug,
        "GET",
        `/v1/tenant/automations/${encodeURIComponent(uuid)}`,
      ),
    ),
  triggers: (slug: string) =>
    tenantRequest<string[]>(slug, "GET", "/v1/tenant/automations/triggers"),
  create: async (slug: string, body: RuleInput) =>
    decodeRule(
      await tenantRequest<RawRule>(slug, "POST", "/v1/tenant/automations", {
        body,
        silent: true,
      }),
    ),
  update: async (slug: string, uuid: string, body: RuleInput) =>
    decodeRule(
      await tenantRequest<RawRule>(
        slug,
        "PUT",
        `/v1/tenant/automations/${encodeURIComponent(uuid)}`,
        { body, silent: true },
      ),
    ),
  remove: (slug: string, uuid: string) =>
    tenantRequest<{ deleted: boolean }>(
      slug,
      "DELETE",
      `/v1/tenant/automations/${encodeURIComponent(uuid)}`,
    ),
  /** Evaluates the saved rule against a conversation; runs no action. */
  test: (slug: string, uuid: string, conversation: string) =>
    tenantRequest<DryRunResult>(
      slug,
      "POST",
      `/v1/tenant/automations/${encodeURIComponent(uuid)}/test`,
      { body: { conversation_uuid: conversation }, silent: true },
    ),
  executions: (slug: string, limit = 25, offset = 0) =>
    tenantRequest<Page<RuleExecution>>(
      slug,
      "GET",
      "/v1/tenant/automations/executions",
      { query: { limit, offset } },
    ),
};
