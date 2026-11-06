import type { components } from '@/core/api/schema';

/**
 * Read-only summaries of workflows and automation rules (CF-164). The shapes
 * and the wording follow the panel (`frontend/src/features/workflows/lib/graph.ts`,
 * `frontend/src/features/automation-rules/lib/rules.ts`); editing stays there.
 */

export type Workflow = components['schemas']['Workflow'];
export type WorkflowNode = components['schemas']['WorkflowNode'];
export type WorkflowEdge = components['schemas']['WorkflowEdge'];
export type WorkflowRun = components['schemas']['WorkflowRun'];
export type Condition = components['schemas']['AutomationCondition'];
export type AnyAction = components['schemas']['WorkflowAction'];
export type RuleAction = components['schemas']['AutomationAction'];
export type RuleInput = components['schemas']['AutomationRuleInput'];

/**
 * Row of `GET /v1/tenant/automations`. The API sends `conditions` / `actions`
 * as base64 of the stored JSON (Go `[]byte`); `decodeRule` turns them back.
 */
export type RawRule = {
  id: number;
  uuid: string;
  name: string;
  trigger_event: string;
  conditions: string | Condition;
  actions: string | RuleAction[];
  position: number;
  enabled: boolean;
  stop_on_match: boolean;
  schedule_minutes: number;
  next_run_at: string | null;
  created_at: string;
  updated_at: string;
};

export type Rule = Omit<RawRule, 'conditions' | 'actions'> & { conditions: Condition; actions: RuleAction[] };

/** Row of `GET /v1/tenant/automations/executions` (one rule × event). */
export type RuleExecution = {
  uuid: string;
  rule_id: number;
  event_id: string;
  conversation_uuid: string;
  status: 'running' | 'succeeded' | 'failed' | 'skipped';
  matched: boolean;
  error: string;
  action_index: number;
  created_at: string;
  finished_at: string | null;
};

export type Page<T> = { items: T[]; total: number; limit: number; offset: number };

/** `t` of the `admin` namespace; every key here starts with `automations.`. */
export type T = (key: string, params?: Record<string, string | number>) => string;

const slug = (name: string) => name.replace(/\./g, '_');

/** Bytes of a base64 string read as UTF-8 (Hermes has `atob` but no `TextDecoder` everywhere). */
function utf8FromBase64(v: string) {
  const bin = atob(v);
  let escaped = '';
  for (let i = 0; i < bin.length; i++) escaped += `%${bin.charCodeAt(i).toString(16).padStart(2, '0')}`;
  return decodeURIComponent(escaped);
}

function decodeJson<V>(v: unknown, fallback: V): V {
  if (typeof v !== 'string') return (v as V) ?? fallback;
  try {
    return JSON.parse(utf8FromBase64(v)) as V;
  } catch {
    try {
      return JSON.parse(v) as V;
    } catch {
      return fallback;
    }
  }
}

export function decodeRule(raw: RawRule): Rule {
  return {
    ...raw,
    conditions: decodeJson<Condition>(raw.conditions, { all: [] }),
    actions: decodeJson<RuleAction[]>(raw.actions, []),
  };
}

/** The rule as the full-replacement body of `PUT /v1/tenant/automations/{uuid}`. */
export function toRuleInput(r: Rule): RuleInput {
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
export function countConditions(c: Condition | undefined): number {
  if (!c) return 0;
  if (c.all || c.any) return [...(c.all ?? []), ...(c.any ?? [])].reduce((n, x) => n + countConditions(x), 0);
  return c.field ? 1 : 0;
}

/** Newest evaluation of the rule in a page of the run log (newest first). */
export function lastExecution(log: RuleExecution[] | undefined, ruleId: number) {
  return (log ?? []).find((e) => e.rule_id === ruleId);
}

// ---- labels -----------------------------------------------------------------

/** Triggers with a label; any other name the API returns is shown as is. */
const KNOWN_TRIGGERS = new Set([
  'scheduled',
  'webhook',
  'conversation.created',
  'conversation.updated',
  'conversation.assigned',
  'conversation.unassigned',
  'conversation.status_changed',
  'conversation.priority_changed',
  'conversation.resolved',
  'conversation.reopened',
  'conversation.snoozed',
  'conversation.mentioned',
  'conversation.tag_added',
  'message.received',
  'message.sent',
  'message.status_changed',
  'message.failed',
  'sla.applied',
  'sla.warning',
  'sla.breached',
  'sla.met',
]);

const KNOWN_FIELDS = new Set([
  'uuid',
  'number',
  'status',
  'priority',
  'subject',
  'channel_type',
  'source',
  'inbox.uuid',
  'contact.uuid',
  'contact.name',
  'contact.email',
  'contact.phone',
  'company.uuid',
  'company.name',
  'assignee.uuid',
  'team_id',
  'tag_uuids',
  'unread_count',
  'message_count',
  'last_message_direction',
  'hours_since_created',
  'hours_since_updated',
  'hours_since_inbound',
  'hours_waiting',
  'sla_breached',
  'clock.hour',
  'clock.weekday',
]);

const KNOWN_ACTIONS = new Set<string>([
  'assign_user',
  'add_tag',
  'remove_tag',
  'set_priority',
  'set_status',
  'send_canned_reply',
  'send_template',
  'add_note',
  'notify',
  'call_webhook',
  'run_ai_procedure',
  'create_engineering_issue',
  'route',
]);

const STATUSES = new Set(['open', 'pending', 'snoozed', 'resolved', 'closed']);
const PRIORITIES = new Set(['low', 'normal', 'high', 'urgent']);
const UNARY = new Set(['exists', 'not_exists']);
const OPERATORS = new Set([
  'eq',
  'neq',
  'contains',
  'not_contains',
  'in',
  'not_in',
  'gt',
  'gte',
  'lt',
  'lte',
  'exists',
  'not_exists',
]);

export function triggerLabel(t: T, name: string | undefined, scheduleMinutes = 0) {
  if (!name) return t('automations.trigger_none');
  const label = KNOWN_TRIGGERS.has(name) ? t(`automations.trigger.${slug(name)}`) : name;
  return name === 'scheduled' && scheduleMinutes > 0
    ? `${label} · ${t('automations.every_minutes', { count: scheduleMinutes })}`
    : label;
}

function fieldLabel(t: T, field: string) {
  if (KNOWN_FIELDS.has(field)) return t(`automations.field.${slug(field)}`);
  return field.startsWith('custom_fields.') ? field.slice('custom_fields.'.length) : field;
}

function valueLabel(t: T, field: string, value: unknown, names?: Names): string {
  if (Array.isArray(value)) return value.map((v) => valueLabel(t, field, v, names)).join(', ');
  if (typeof value === 'boolean') return t(value ? 'automations.yes' : 'automations.no');
  if (value === null || value === undefined) return '';
  const text = String(value);
  if (field === 'status' && STATUSES.has(text)) return t(`automations.status_value.${text}`);
  if (field === 'priority' && PRIORITIES.has(text)) return t(`automations.priority_value.${text}`);
  return names?.get(text) ?? text;
}

/** uuid → display name for tags and agents the member is allowed to list. */
export type Names = ReadonlyMap<string, string>;

export type ConditionLine = { depth: number; text: string };

/**
 * The condition tree as indented lines: a group line ("All of 2 conditions")
 * followed by its members. A root group with no member gives no line; see
 * `emptyCondition`.
 */
export function conditionLines(t: T, c: Condition | undefined, names?: Names, depth = 0): ConditionLine[] {
  if (!c) return [];
  if (c.all !== undefined || c.any !== undefined) {
    const members = [...(c.all ?? []), ...(c.any ?? [])];
    if (!members.length) return [];
    const key = c.all !== undefined ? 'automations.group_all' : 'automations.group_any';
    return [
      { depth, text: t(key, { count: members.length }) },
      ...members.flatMap((m) => conditionLines(t, m, names, depth + 1)),
    ];
  }
  if (!c.field) return [];
  const operator =
    c.operator && OPERATORS.has(c.operator) ? t(`automations.operator.${c.operator}`) : (c.operator ?? '');
  const value = c.operator && UNARY.has(c.operator) ? '' : valueLabel(t, c.field, c.value, names);
  return [{ depth, text: [fieldLabel(t, c.field), operator, value].filter(Boolean).join(' ') }];
}

/** What an empty root group means: "all" matches every conversation, "any" none. */
export function emptyCondition(c: Condition | undefined): 'all' | 'any' | null {
  if (!c || c.field) return null;
  const members = [...(c.all ?? []), ...(c.any ?? [])];
  if (members.length) return null;
  return c.any !== undefined && c.all === undefined ? 'any' : 'all';
}

export function describeAction(t: T, a: AnyAction, names?: Names): { title: string; detail: string } {
  const title = KNOWN_ACTIONS.has(a.type) ? t(`automations.action.${a.type}`) : a.type;
  const content = (a.content ?? '').trim();
  let detail = '';
  switch (a.type) {
    case 'set_status':
      detail = valueLabel(t, 'status', a.value);
      break;
    case 'set_priority':
      detail = valueLabel(t, 'priority', a.value);
      break;
    case 'add_note':
    case 'send_canned_reply':
      detail = content;
      break;
    case 'create_engineering_issue':
      detail = (a.value ?? '').trim();
      break;
    case 'route': {
      try {
        const strategy = (JSON.parse(content) as { settings?: { strategy?: string } }).settings?.strategy;
        if (strategy === 'round_robin' || strategy === 'load_balanced') detail = t(`automations.route.${strategy}`);
      } catch {
        detail = '';
      }
      break;
    }
    default:
      detail = (a.target_uuid && names?.get(a.target_uuid)) || '';
  }
  return { title, detail };
}

// ---- workflows --------------------------------------------------------------

export type WorkflowState = 'draft' | 'enabled' | 'disabled';

/** Never published → draft (it cannot be enabled); otherwise on or off. */
export function workflowState(w: Pick<Workflow, 'published_version' | 'enabled'>): WorkflowState {
  if (w.published_version === null) return 'draft';
  return w.enabled ? 'enabled' : 'disabled';
}

/** The draft moved on since the published version. */
export function hasUnpublishedChanges(w: Pick<Workflow, 'published_version' | 'revision'>) {
  return w.published_version !== null && w.published_version !== w.revision;
}

export function triggerNode(w: Pick<Workflow, 'draft'>) {
  return w.draft.nodes.find((n) => n.type === 'trigger');
}

const PORT_ORDER: Record<WorkflowEdge['port'], number> = { next: 0, true: 1, false: 2 };

/**
 * Nodes in reading order: from the trigger along the edges ("yes" before
 * "no"), each node once, loop edges ignored; nodes nothing leads to come last.
 */
export function orderedNodes(graph: Workflow['draft']): WorkflowNode[] {
  const byId = new Map(graph.nodes.map((n) => [n.id, n]));
  const seen = new Set<string>();
  const out: WorkflowNode[] = [];
  const visit = (id: string) => {
    const node = byId.get(id);
    if (!node || seen.has(id)) return;
    seen.add(id);
    out.push(node);
    graph.edges
      .filter((e) => e.from === id && !e.loop)
      .sort((a, b) => PORT_ORDER[a.port] - PORT_ORDER[b.port])
      .forEach((e) => visit(e.to));
  };
  graph.nodes.filter((n) => n.type === 'trigger').forEach((n) => visit(n.id));
  graph.nodes.forEach((n) => visit(n.id));
  return out;
}

function waitLabel(t: T, seconds: number) {
  if (seconds >= 86_400 && seconds % 86_400 === 0) return t('automations.wait_days', { count: seconds / 86_400 });
  if (seconds >= 3_600 && seconds % 3_600 === 0) return t('automations.wait_hours', { count: seconds / 3_600 });
  return t('automations.wait_minutes', { count: Math.max(1, Math.round(seconds / 60)) });
}

/** One step of the flow: what kind of node it is and what it does, in words. */
export function describeNode(t: T, node: WorkflowNode, names?: Names): { title: string; lines: ConditionLine[] } {
  const title = t(`automations.kind.${node.type}`);
  const line = (text: string): ConditionLine[] => (text ? [{ depth: 0, text }] : []);
  switch (node.type) {
    case 'trigger':
      return { title, lines: line(triggerLabel(t, node.trigger, node.schedule_minutes)) };
    case 'condition':
    case 'branch': {
      const empty = emptyCondition(node.condition);
      return {
        title,
        lines: empty ? line(t(`automations.empty_${empty}`)) : conditionLines(t, node.condition, names),
      };
    }
    case 'action': {
      if (!node.action) return { title, lines: [] };
      const a = describeAction(t, node.action, names);
      return { title, lines: [...line(a.title), ...(a.detail ? [{ depth: 1, text: a.detail }] : [])] };
    }
    case 'wait':
      return { title, lines: line(waitLabel(t, node.delay_seconds ?? 0)) };
    default:
      return { title, lines: line(t('automations.flow_ends')) };
  }
}

export type Tone = 'neutral' | 'progress' | 'done' | 'problem';

/** Workflow run and rule execution statuses share one set of tones. */
export function runTone(status: string): Tone {
  switch (status) {
    case 'succeeded':
      return 'done';
    case 'failed':
    case 'blocked':
      return 'problem';
    case 'running':
    case 'waiting':
    case 'pending':
      return 'progress';
    default:
      return 'neutral';
  }
}

/** Replaces one row of a cached list page, keeping the rest as it is. */
export function patchRow<R extends { uuid: string }>(page: Page<R> | undefined, uuid: string, patch: Partial<R>) {
  if (!page) return page;
  return { ...page, items: page.items.map((r) => (r.uuid === uuid ? { ...r, ...patch } : r)) };
}
