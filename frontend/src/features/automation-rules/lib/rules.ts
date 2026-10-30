import {
  LIMITS,
  slug,
  validateAction,
  validateCondition,
  type ActionType,
  type Issue,
} from "@/features/workflows/lib/graph";

import type {
  Rule,
  RuleAction,
  RuleCondition,
  RuleInput,
} from "../services/rules.service";

/*
 * Mirrors backend/internal/modules/automation/usecase/service.go `Validate`.
 * Conditions and action fields are shared with the visual workflow editor
 * (CF-96); the server stays authoritative and re-validates on save.
 */

/** Actions an automation rule accepts (a subset of the workflow actions). */
export const RULE_ACTIONS: readonly ActionType[] = [
  "assign_user",
  "add_tag",
  "remove_tag",
  "set_status",
  "set_priority",
  "send_canned_reply",
  "add_note",
  "notify",
  "call_webhook",
];

export const RULE_LIMITS = {
  maxName: 200,
  maxActions: 20,
  maxScheduleMinutes: LIMITS.maxScheduleMinutes,
} as const;

/**
 * Triggers with a label; `GET /automations/triggers` is the source of truth
 * and any other name it returns is shown as is.
 */
const KNOWN_TRIGGERS = new Set([
  "scheduled",
  "conversation.created",
  "conversation.updated",
  "conversation.assigned",
  "conversation.unassigned",
  "conversation.status_changed",
  "conversation.priority_changed",
  "conversation.resolved",
  "conversation.reopened",
  "conversation.snoozed",
  "conversation.mentioned",
  "conversation.tag_added",
  "message.received",
  "message.sent",
  "message.status_changed",
  "message.failed",
  "sla.applied",
  "sla.warning",
  "sla.breached",
  "sla.met",
]);

type T = (key: string, params?: Record<string, string | number>) => string;

export function triggerLabel(t: T, name: string) {
  return KNOWN_TRIGGERS.has(name) ? t(`rules.triggers.${slug(name)}`) : name;
}

export function emptyRule(): RuleInput {
  return {
    name: "",
    trigger: "conversation.created",
    conditions: { all: [] },
    actions: [{ type: "add_note", content: "" }],
    position: 0,
    enabled: true,
    stop_on_match: false,
    schedule_minutes: 0,
  };
}

export function toInput(r: Rule): RuleInput {
  return {
    name: r.name,
    trigger: r.trigger_event,
    conditions: r.conditions,
    actions: r.actions,
    position: r.position,
    enabled: r.enabled,
    stop_on_match: r.stop_on_match,
    schedule_minutes: r.schedule_minutes,
  };
}

/** Number of field comparisons in the tree. */
export function countConditions(c: RuleCondition | undefined): number {
  if (!c) return 0;
  if (c.all || c.any)
    return [...(c.all ?? []), ...(c.any ?? [])].reduce(
      (n, x) => n + countConditions(x),
      0,
    );
  return c.field ? 1 : 0;
}

export type RuleIssues = {
  form: Issue[];
  conditions: Issue | null;
  actions: (Issue | null)[];
};

export function validateRule(r: RuleInput): RuleIssues {
  const form: Issue[] = [];
  if (!r.name.trim() || r.name.length > RULE_LIMITS.maxName)
    form.push({ key: "rules.issues.name" });
  const minutes = r.schedule_minutes ?? 0;
  if (r.trigger === "scheduled") {
    if (minutes < 1 || minutes > RULE_LIMITS.maxScheduleMinutes)
      form.push({ key: "rules.issues.schedule" });
  }
  if (r.actions.length === 0) form.push({ key: "rules.issues.no_actions" });
  if (r.actions.length > RULE_LIMITS.maxActions)
    form.push({
      key: "rules.issues.too_many_actions",
      params: { count: RULE_LIMITS.maxActions },
    });
  return {
    form,
    conditions: validateCondition(r.conditions),
    actions: r.actions.map((a: RuleAction) =>
      RULE_ACTIONS.includes(a.type)
        ? validateAction(a)
        : { key: "rules.issues.action_type" },
    ),
  };
}

export function hasIssues(i: RuleIssues) {
  return i.form.length > 0 || i.conditions !== null || i.actions.some(Boolean);
}
