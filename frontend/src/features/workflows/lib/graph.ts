import type { components } from "@/generated/api";

export type WorkflowGraph = components["schemas"]["WorkflowGraph"];
export type WorkflowNode = components["schemas"]["WorkflowNode"];
export type WorkflowEdge = components["schemas"]["WorkflowEdge"];
export type NodeKind = WorkflowNode["type"];
export type TriggerName = NonNullable<WorkflowNode["trigger"]>;
export type Condition = components["schemas"]["AutomationCondition"];
export type ConditionOperator = NonNullable<Condition["operator"]>;
export type WorkflowAction = components["schemas"]["WorkflowAction"];
export type ActionType = WorkflowAction["type"];
export type Port = WorkflowEdge["port"];

/*
 * Mirrors backend/internal/modules/automation/usecase/{graph,conditions,service}.go.
 * The server stays authoritative (it re-validates on save and publish); this
 * copy only exists so the editor can point at the offending node inline.
 */

export const NODE_KINDS: NodeKind[] = [
  "trigger",
  "condition",
  "branch",
  "action",
  "wait",
  "end",
];

export const TRIGGERS: TriggerName[] = [
  "conversation.created",
  "conversation.updated",
  "message.received",
  "conversation.tag_added",
  "sla.breached",
  "scheduled",
  "webhook",
];

export const ACTIONS: ActionType[] = [
  "assign_user",
  "add_tag",
  "remove_tag",
  "set_priority",
  "set_status",
  "send_canned_reply",
  "send_template",
  "add_note",
  "notify",
  "call_webhook",
  "run_system_action",
  "run_ai_procedure",
  "create_engineering_issue",
  "route",
];

export const CONDITION_FIELDS = [
  "uuid",
  "number",
  "status",
  "priority",
  "subject",
  "channel_type",
  "source",
  "inbox.uuid",
  "contact.uuid",
  "contact.name",
  "contact.email",
  "contact.phone",
  "company.uuid",
  "company.name",
  "assignee.uuid",
  "team_id",
  "tag_uuids",
  "unread_count",
  "message_count",
  "last_message_direction",
  "hours_since_created",
  "hours_since_updated",
  "hours_since_inbound",
  "hours_waiting",
  "sla_breached",
  "clock.hour",
  "clock.weekday",
  // Screen context of the latest widget message (CF-197).
  "screen.page_url",
  "screen.page_path",
  "screen.last_event",
  "screen.event_names",
] as const;

export const NUMERIC_FIELDS = new Set<string>([
  "number",
  "unread_count",
  "message_count",
  "hours_since_created",
  "hours_since_updated",
  "hours_since_inbound",
  "hours_waiting",
  "clock.hour",
  "clock.weekday",
]);
export const BOOLEAN_FIELDS = new Set<string>(["sla_breached"]);

export const OPERATORS: ConditionOperator[] = [
  "eq",
  "neq",
  "contains",
  "not_contains",
  "in",
  "not_in",
  "gt",
  "gte",
  "lt",
  "lte",
  "exists",
  "not_exists",
];
export const LIST_OPERATORS = new Set<string>(["in", "not_in"]);
export const NUMERIC_OPERATORS = new Set<string>(["gt", "gte", "lt", "lte"]);
export const UNARY_OPERATORS = new Set<string>(["exists", "not_exists"]);

/** set_status accepts every base status except snoozed. */
export const STATUSES = ["open", "pending", "resolved", "closed"] as const;
export const PRIORITIES = ["low", "normal", "high", "urgent"] as const;
export const ROUTE_STRATEGIES = ["round_robin", "load_balanced"] as const;

export const LIMITS = {
  minNodes: 2,
  maxNodes: 100,
  maxEdges: 200,
  maxBytes: 128 << 10,
  minWait: 60,
  maxWait: 2_592_000,
  maxScheduleMinutes: 525_600,
  maxConditionDepth: 8,
  maxConditionNodes: 128,
  maxContent: 10_000,
  maxTitle: 200,
  maxSkills: 32,
} as const;

/** Which action fields the side panel shows and the validator requires. */
export type TargetKind =
  "user" | "tag" | "canned" | "webhook" | "procedure" | "agent";
export const ACTION_TARGET: Partial<Record<ActionType, TargetKind>> = {
  assign_user: "agent",
  add_tag: "tag",
  remove_tag: "tag",
  notify: "user",
  call_webhook: "webhook",
  run_ai_procedure: "procedure",
  send_template: "canned",
};

export function ports(kind: NodeKind): Port[] {
  switch (kind) {
    case "end":
      return [];
    case "condition":
    case "branch":
      return ["true", "false"];
    default:
      return ["next"];
  }
}

export function hasInput(kind: NodeKind) {
  return kind !== "trigger";
}

/** Route action content: a routing snapshot (settings, team, skills). */
export type RouteSnapshot = {
  settings: {
    enabled: boolean;
    strategy: (typeof ROUTE_STRATEGIES)[number];
    respect_capacity: boolean;
    vip_account_owner: boolean;
    reassign_on_offline: boolean;
    unanswered_minutes: number | null;
  };
  team_uuid: string | null;
  skills: string[];
};

export function defaultRouteSnapshot(): RouteSnapshot {
  return {
    settings: {
      enabled: true,
      strategy: "round_robin",
      respect_capacity: true,
      vip_account_owner: false,
      reassign_on_offline: false,
      unanswered_minutes: null,
    },
    team_uuid: null,
    skills: [],
  };
}

export function parseRouteSnapshot(content?: string): RouteSnapshot {
  const base = defaultRouteSnapshot();
  if (!content) return base;
  try {
    const raw = JSON.parse(content) as Partial<RouteSnapshot>;
    return {
      settings: { ...base.settings, ...(raw.settings ?? {}) },
      team_uuid: raw.team_uuid ?? null,
      skills: Array.isArray(raw.skills) ? raw.skills : [],
    };
  } catch {
    return base;
  }
}

let seq = 0;
export function newNodeId(kind: NodeKind) {
  seq += 1;
  const rand = Math.random().toString(36).slice(2, 7);
  return `${kind}_${rand}${seq}`;
}

/** A fresh node with the smallest valid-looking config for its kind. */
export function createNode(
  kind: NodeKind,
  position: { x: number; y: number },
): WorkflowNode {
  const node: WorkflowNode = { id: newNodeId(kind), type: kind, position };
  switch (kind) {
    case "trigger":
      node.trigger = "conversation.created";
      break;
    case "condition":
    case "branch":
      node.condition = {
        all: [{ field: "status", operator: "eq", value: "open" }],
      };
      break;
    case "action":
      node.action = { type: "add_note", content: "" };
      break;
    case "wait":
      node.delay_seconds = 3600;
      break;
  }
  return node;
}

/** New workflows start as trigger -> end so the first save already validates. */
export function starterGraph(): WorkflowGraph {
  const trigger = createNode("trigger", { x: 0, y: 0 });
  const end = createNode("end", { x: 280, y: 0 });
  return {
    schema_version: 1,
    nodes: [trigger, end],
    edges: [{ from: trigger.id, port: "next", to: end.id, input: "in" }],
  };
}

export type Issue = { key: string; params?: Record<string, string | number> };
export type ValidationResult = {
  graph: Issue[];
  nodes: Record<string, Issue[]>;
  count: number;
};

function isNumber(v: unknown): v is number {
  return typeof v === "number" && Number.isFinite(v);
}

export function validateCondition(
  c: Condition,
  depth = 0,
  counter = { n: 0 },
): Issue | null {
  counter.n += 1;
  if (depth > LIMITS.maxConditionDepth || counter.n > LIMITS.maxConditionNodes)
    return { key: "workflows.issues.condition_too_large" };
  const groups =
    Number(c.all !== undefined) +
    Number(c.any !== undefined) +
    Number(Boolean(c.field));
  if (groups !== 1) return { key: "workflows.issues.condition_shape" };
  if (c.all !== undefined || c.any !== undefined) {
    for (const sub of [...(c.all ?? []), ...(c.any ?? [])]) {
      const issue = validateCondition(sub, depth + 1, counter);
      if (issue) return issue;
    }
    return null;
  }
  const field = c.field ?? "";
  const known =
    (CONDITION_FIELDS as readonly string[]).includes(field) ||
    (field.startsWith("custom_fields.") &&
      field.length <= 180 &&
      field !== "custom_fields.");
  if (!known)
    return { key: "workflows.issues.condition_field", params: { field } };
  if (!c.operator || !OPERATORS.includes(c.operator))
    return { key: "workflows.issues.condition_operator" };
  if (LIST_OPERATORS.has(c.operator) && !Array.isArray(c.value))
    return { key: "workflows.issues.condition_list" };
  if (NUMERIC_OPERATORS.has(c.operator) && !isNumber(c.value))
    return { key: "workflows.issues.condition_number" };
  return null;
}

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export function isUuid(v: unknown): v is string {
  return typeof v === "string" && UUID_RE.test(v);
}

export function validateAction(a: WorkflowAction): Issue | null {
  const content = (a.content ?? "").trim();
  if (ACTION_TARGET[a.type] && !isUuid(a.target_uuid))
    return { key: "workflows.issues.action_target" };
  switch (a.type) {
    case "set_status":
      return (STATUSES as readonly string[]).includes(a.value ?? "")
        ? null
        : { key: "workflows.issues.action_status" };
    case "set_priority":
      return (PRIORITIES as readonly string[]).includes(a.value ?? "")
        ? null
        : { key: "workflows.issues.action_priority" };
    case "run_system_action":
      return /^[a-z][a-z0-9_]{1,49}$/.test(a.value ?? "")
        ? null
        : { key: "workflows.issues.action_system" };
    case "add_note":
    case "send_canned_reply":
      return content && [...content].length <= LIMITS.maxContent
        ? null
        : { key: "workflows.issues.action_content" };
    case "create_engineering_issue":
      if (!(a.value ?? "").trim() || (a.value ?? "").length > LIMITS.maxTitle)
        return { key: "workflows.issues.action_title" };
      return (a.content ?? "").length > LIMITS.maxContent
        ? { key: "workflows.issues.action_content" }
        : null;
    case "route": {
      if (!a.content || a.content.length > LIMITS.maxContent)
        return { key: "workflows.issues.action_route" };
      try {
        const snap = JSON.parse(a.content) as RouteSnapshot;
        if (
          !(ROUTE_STRATEGIES as readonly string[]).includes(
            snap?.settings?.strategy,
          )
        )
          return { key: "workflows.issues.action_route" };
        const skills = snap.skills ?? [];
        if (
          skills.length > LIMITS.maxSkills ||
          skills.some((s) => !s.trim() || s.length > 80)
        )
          return { key: "workflows.issues.action_route" };
      } catch {
        return { key: "workflows.issues.action_route" };
      }
      return null;
    }
    default:
      return null;
  }
}

function nodeIssue(n: WorkflowNode): Issue | null {
  switch (n.type) {
    case "trigger":
      if (!n.trigger || !TRIGGERS.includes(n.trigger))
        return { key: "workflows.issues.trigger_unknown" };
      if (
        (n.trigger === "scheduled") !== (n.schedule_minutes ?? 0) > 0 ||
        (n.schedule_minutes ?? 0) > LIMITS.maxScheduleMinutes
      )
        return { key: "workflows.issues.trigger_schedule" };
      return null;
    case "condition":
    case "branch":
      return n.condition
        ? validateCondition(n.condition)
        : { key: "workflows.issues.condition_shape" };
    case "action":
      return n.action
        ? validateAction(n.action)
        : { key: "workflows.issues.action_target" };
    case "wait": {
      const d = n.delay_seconds ?? 0;
      return d >= LIMITS.minWait && d <= LIMITS.maxWait
        ? null
        : { key: "workflows.issues.wait_range" };
    }
    default:
      return null;
  }
}

/** Client copy of ValidateGraph that attributes each problem to a node. */
export function validateGraph(g: WorkflowGraph): ValidationResult {
  const graph: Issue[] = [];
  const nodes: Record<string, Issue[]> = {};
  const add = (id: string, issue: Issue) => {
    (nodes[id] ??= []).push(issue);
  };
  if (g.nodes.length < LIMITS.minNodes || g.nodes.length > LIMITS.maxNodes)
    graph.push({
      key: "workflows.issues.node_count",
      params: { min: LIMITS.minNodes, max: LIMITS.maxNodes },
    });
  if (g.edges.length > LIMITS.maxEdges)
    graph.push({
      key: "workflows.issues.edge_count",
      params: { max: LIMITS.maxEdges },
    });
  if (JSON.stringify(g).length > LIMITS.maxBytes)
    graph.push({ key: "workflows.issues.too_large" });

  const byId = new Map<string, WorkflowNode>();
  for (const n of g.nodes) {
    if (byId.has(n.id)) add(n.id, { key: "workflows.issues.duplicate" });
    byId.set(n.id, n);
    const issue = nodeIssue(n);
    if (issue) add(n.id, issue);
  }
  const triggers = g.nodes.filter((n) => n.type === "trigger");
  if (triggers.length !== 1)
    graph.push({ key: "workflows.issues.trigger_count" });

  const outgoing = new Map<string, Set<string>>();
  const adj = new Map<string, string[]>();
  const all = new Map<string, string[]>();
  const reverse = new Map<string, string[]>();
  for (const e of g.edges) {
    const from = byId.get(e.from);
    const to = byId.get(e.to);
    if (!from || !to) {
      graph.push({ key: "workflows.issues.dangling_edge" });
      continue;
    }
    if (to.type === "trigger" || e.input !== "in")
      add(to.id, { key: "workflows.issues.bad_input" });
    if (!ports(from.type).includes(e.port))
      add(from.id, { key: "workflows.issues.bad_port" });
    if (e.loop && from.type !== "wait")
      add(from.id, { key: "workflows.issues.loop_not_wait" });
    const used = outgoing.get(e.from) ?? new Set<string>();
    if (used.has(e.port)) add(from.id, { key: "workflows.issues.port_multi" });
    used.add(e.port);
    outgoing.set(e.from, used);
    if (!e.loop) adj.set(e.from, [...(adj.get(e.from) ?? []), e.to]);
    all.set(e.from, [...(all.get(e.from) ?? []), e.to]);
    reverse.set(e.to, [...(reverse.get(e.to) ?? []), e.from]);
  }

  for (const n of g.nodes) {
    const open = ports(n.type).filter((p) => !outgoing.get(n.id)?.has(p));
    if (open.length > 0)
      add(n.id, {
        key: "workflows.issues.port_open",
        params: { ports: open.join(", ") },
      });
  }

  // Ordinary edges must form a DAG; cycles only through wait loop edges.
  // Only nodes on the cycle itself (the DFS path from the re-entered node)
  // are flagged, not their ancestors.
  const state = new Map<string, number>();
  const cyclic = new Set<string>();
  const path: string[] = [];
  const visit = (id: string) => {
    if (state.get(id) === 1) {
      path.slice(path.indexOf(id)).forEach((x) => cyclic.add(x));
      return;
    }
    if (state.get(id) === 2) return;
    state.set(id, 1);
    path.push(id);
    for (const to of adj.get(id) ?? []) visit(to);
    path.pop();
    state.set(id, 2);
  };
  for (const n of g.nodes) visit(n.id);
  for (const id of cyclic) add(id, { key: "workflows.issues.cycle" });

  const walk = (start: string, a: Map<string, string[]>, seen: Set<string>) => {
    const stack = [start];
    while (stack.length) {
      const id = stack.pop()!;
      if (seen.has(id)) continue;
      seen.add(id);
      stack.push(...(a.get(id) ?? []));
    }
  };
  const reachable = new Set<string>();
  const toEnd = new Set<string>();
  for (const n of g.nodes) {
    if (n.type === "trigger") walk(n.id, all, reachable);
    if (n.type === "end") walk(n.id, reverse, toEnd);
  }
  for (const n of g.nodes) {
    if (!reachable.has(n.id))
      add(n.id, { key: "workflows.issues.unreachable" });
    else if (!toEnd.has(n.id)) add(n.id, { key: "workflows.issues.no_end" });
  }
  const count =
    graph.length +
    Object.values(nodes).reduce((sum, list) => sum + list.length, 0);
  return { graph, nodes, count };
}

/** True when adding from -> to over ordinary edges would close a cycle. */
export function createsCycle(
  edges: WorkflowEdge[],
  from: string,
  to: string,
): boolean {
  if (from === to) return true;
  const adj = new Map<string, string[]>();
  for (const e of edges)
    if (!e.loop) adj.set(e.from, [...(adj.get(e.from) ?? []), e.to]);
  const seen = new Set<string>();
  const stack = [to];
  while (stack.length) {
    const id = stack.pop()!;
    if (id === from) return true;
    if (seen.has(id)) continue;
    seen.add(id);
    stack.push(...(adj.get(id) ?? []));
  }
  return false;
}

export const LAYOUT = { columnGap: 280, rowGap: 150 } as const;

/**
 * Layered left-to-right layout: each node sits one column after its deepest
 * parent (longest path over non-loop edges); rows follow parent order with
 * `true` before `false`. Nodes not reachable from the trigger go last.
 */
export function autoLayout(g: WorkflowGraph): WorkflowGraph {
  const order: Port[] = ["next", "true", "false"];
  const children = new Map<string, string[]>();
  const indegree = new Map<string, number>(g.nodes.map((n) => [n.id, 0]));
  const sorted = [...g.edges]
    .filter((e) => !e.loop)
    .sort((a, b) => order.indexOf(a.port) - order.indexOf(b.port));
  for (const e of sorted) {
    if (!indegree.has(e.to) || !indegree.has(e.from)) continue;
    children.set(e.from, [...(children.get(e.from) ?? []), e.to]);
    indegree.set(e.to, (indegree.get(e.to) ?? 0) + 1);
  }
  const layer = new Map<string, number>();
  const queue = g.nodes
    .filter((n) => (indegree.get(n.id) ?? 0) === 0)
    .sort((a, b) => Number(b.type === "trigger") - Number(a.type === "trigger"))
    .map((n) => n.id);
  for (const id of queue) layer.set(id, 0);
  const remaining = new Map(indegree);
  const topo: string[] = [];
  while (queue.length) {
    const id = queue.shift()!;
    topo.push(id);
    for (const c of children.get(id) ?? []) {
      layer.set(c, Math.max(layer.get(c) ?? 0, (layer.get(id) ?? 0) + 1));
      remaining.set(c, (remaining.get(c) ?? 1) - 1);
      if (remaining.get(c) === 0) queue.push(c);
    }
  }
  // Cyclic leftovers (invalid graphs) still get a column.
  let maxLayer = Math.max(0, ...layer.values());
  for (const n of g.nodes)
    if (!topo.includes(n.id)) {
      maxLayer += 1;
      layer.set(n.id, maxLayer);
      topo.push(n.id);
    }
  const columns = new Map<number, string[]>();
  for (const id of topo) {
    const l = layer.get(id) ?? 0;
    columns.set(l, [...(columns.get(l) ?? []), id]);
  }
  const pos = new Map<string, { x: number; y: number }>();
  for (const [l, ids] of columns) {
    const offset = ((ids.length - 1) * LAYOUT.rowGap) / 2;
    ids.forEach((id, i) =>
      pos.set(id, {
        x: l * LAYOUT.columnGap,
        y: i * LAYOUT.rowGap - offset,
      }),
    );
  }
  return {
    ...g,
    nodes: g.nodes.map((n) => ({
      ...n,
      position: pos.get(n.id) ?? n.position,
    })),
  };
}

function nodeFingerprint(n: WorkflowNode) {
  // Position is inert editor metadata; it never counts as a change.
  const rest = { ...n };
  delete rest.position;
  return JSON.stringify(rest);
}
const edgeKey = (e: WorkflowEdge) =>
  `${e.from}:${e.port}->${e.to}${e.loop ? ":loop" : ""}`;

export type GraphDiff = {
  added: WorkflowNode[];
  removed: WorkflowNode[];
  changed: WorkflowNode[];
  edgesAdded: number;
  edgesRemoved: number;
};

/** Summary of what `next` changes relative to `base` (e.g. draft vs published). */
export function diffGraphs(
  base: WorkflowGraph,
  next: WorkflowGraph,
): GraphDiff {
  const before = new Map(base.nodes.map((n) => [n.id, n]));
  const after = new Map(next.nodes.map((n) => [n.id, n]));
  const added = next.nodes.filter((n) => !before.has(n.id));
  const removed = base.nodes.filter((n) => !after.has(n.id));
  const changed = next.nodes.filter((n) => {
    const old = before.get(n.id);
    return old !== undefined && nodeFingerprint(old) !== nodeFingerprint(n);
  });
  const be = new Set(base.edges.map(edgeKey));
  const ae = new Set(next.edges.map(edgeKey));
  return {
    added,
    removed,
    changed,
    edgesAdded: [...ae].filter((k) => !be.has(k)).length,
    edgesRemoved: [...be].filter((k) => !ae.has(k)).length,
  };
}

export function isEmptyDiff(d: GraphDiff) {
  return (
    d.added.length +
      d.removed.length +
      d.changed.length +
      d.edgesAdded +
      d.edgesRemoved ===
    0
  );
}

/** Splits a wait into the largest whole unit for display and editing. */
export type WaitUnit = "minutes" | "hours" | "days";
export const WAIT_UNITS: Record<WaitUnit, number> = {
  minutes: 60,
  hours: 3600,
  days: 86_400,
};
export function splitDelay(seconds: number): {
  amount: number;
  unit: WaitUnit;
} {
  if (seconds % WAIT_UNITS.days === 0)
    return { amount: seconds / WAIT_UNITS.days, unit: "days" };
  if (seconds % WAIT_UNITS.hours === 0)
    return { amount: seconds / WAIT_UNITS.hours, unit: "hours" };
  return { amount: Math.round(seconds / 60), unit: "minutes" };
}

/** i18n key segment for dotted backend names (`conversation.created`). */
export function slug(name: string) {
  return name.replace(/\./g, "_");
}
