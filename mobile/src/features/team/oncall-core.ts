/**
 * On-call read views (CF-168): how escalation policies, uptime monitors and
 * status components are summarized, the way the panel's on-call page does
 * (`frontend/src/features/oncall/components/policies-tab.tsx`,
 * `monitors-tab.tsx`). Free of app imports so it runs under `node --test`.
 *
 * Every lookup of an API value returns `null` for one the app does not know,
 * so the screen shows `readable(value)` instead of a raw translation key.
 */

/** `partial_outage` → `Partial outage`: an unknown API value, shown as text. */
export function readable(value: string | null | undefined) {
  const text = (value ?? '').replace(/[_-]+/g, ' ').trim();
  return text ? text.charAt(0).toUpperCase() + text.slice(1) : '—';
}

export type Tone = 'neutral' | 'progress' | 'done' | 'problem';

const CHANNELS = ['inapp', 'push', 'email', 'sms'] as const;
export type ChannelKey = (typeof CHANNELS)[number];
export function channelKey(value: string): ChannelKey | null {
  return (CHANNELS as readonly string[]).includes(value) ? (value as ChannelKey) : null;
}

type Step = { agent?: string | null; rota?: string | null; team?: string | null };
export type StepTarget = { kind: 'rota' | 'team' | 'agent'; uuid: string } | null;

/** Who a step notifies: the rota, else the team, else the agent (the panel's order). */
export function stepTarget(step: Step): StepTarget {
  if (step.rota) return { kind: 'rota', uuid: step.rota };
  if (step.team) return { kind: 'team', uuid: step.team };
  if (step.agent) return { kind: 'agent', uuid: step.agent };
  return null;
}

/** `{unit, count}` of a check interval: whole hours, else whole minutes, else seconds. */
export function intervalParts(seconds: number): { unit: 'hours' | 'minutes' | 'seconds'; count: number } {
  if (seconds > 0 && seconds % 3600 === 0) return { unit: 'hours', count: seconds / 3600 };
  if (seconds > 0 && seconds % 60 === 0) return { unit: 'minutes', count: seconds / 60 };
  return { unit: 'seconds', count: seconds };
}

type Monitor = { enabled: boolean; last_up: boolean | null; last_status: number };
export type MonitorState = { key: 'paused' | 'pending' | 'up' | 'down' | 'down_code'; tone: Tone; code?: number };

/** Paused when disabled, pending before the first check, else up or down (with the HTTP code when there is one). */
export function monitorState(m: Monitor): MonitorState {
  if (!m.enabled) return { key: 'paused', tone: 'neutral' };
  if (m.last_up === null || m.last_up === undefined) return { key: 'pending', tone: 'neutral' };
  if (m.last_up) return { key: 'up', tone: 'done' };
  return m.last_status ? { key: 'down_code', tone: 'problem', code: m.last_status } : { key: 'down', tone: 'problem' };
}

type Result = { up: boolean; latency_ms: number; checked_at: string };

/** Share of successful checks and the newest latency; `null` before the first check. Results come newest first. */
export function resultsSummary(results: Result[]) {
  if (!results.length) return null;
  const up = results.filter((r) => r.up).length;
  return { up, total: results.length, ratio: up / results.length, latency: results[0]!.latency_ms };
}

const COMPONENT_STATUS = {
  operational: 'done',
  degraded: 'progress',
  partial_outage: 'progress',
  major_outage: 'problem',
  maintenance: 'neutral',
} as const satisfies Record<string, Tone>;
export type ComponentStatusKey = keyof typeof COMPONENT_STATUS;

/** Known status → its key and tone; an unknown one keeps a neutral tone and `key: null`. */
export function componentStatus(value: string | null | undefined): { key: ComponentStatusKey | null; tone: Tone } {
  return value && Object.prototype.hasOwnProperty.call(COMPONENT_STATUS, value)
    ? { key: value as ComponentStatusKey, tone: COMPONENT_STATUS[value as ComponentStatusKey] }
    : { key: null, tone: 'neutral' };
}

type Component = { uuid?: string; name?: string; group_name?: string; sort_order?: number };

/** Components by group, in the status page's order (`sort_order`, then name); ungrouped ones first under `''`. */
export function groupComponents<T extends Component>(items: T[]): { group: string; items: T[] }[] {
  const sorted = [...items].sort(
    (a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0) || (a.name ?? '').localeCompare(b.name ?? ''),
  );
  const groups = new Map<string, T[]>();
  for (const item of sorted) {
    const group = (item.group_name ?? '').trim();
    groups.set(group, [...(groups.get(group) ?? []), item]);
  }
  return [...groups.entries()]
    .sort(([a], [b]) => (a === '' ? -1 : b === '' ? 1 : 0))
    .map(([group, list]) => ({ group, items: list }));
}
