/**
 * Coaching and gamification read views (CF-170), with the panel's rules
 * (`frontend/src/features/coaching`). Free of app imports so it runs under
 * `node --test`.
 */

/** `needs_review` → `Needs review`: an unknown API value, shown as text instead of a translation key. */
export function readable(value: string | null | undefined) {
  const text = (value ?? '').replace(/[_-]+/g, ' ').trim();
  return text ? text.charAt(0).toUpperCase() + text.slice(1) : '—';
}

export const LEADERBOARD_RANGES = [7, 30, 90] as const;

/** `[since, until)`: the last `days` local days, today included (the panel's default range is 30). */
export function leaderboardWindow(days: number, now = new Date()) {
  const until = new Date(now);
  until.setHours(24, 0, 0, 0);
  const since = new Date(until);
  since.setDate(since.getDate() - days);
  return { since: since.toISOString(), until: until.toISOString() };
}

const METRICS = ['resolutions', 'frt', 'csat', 'qa'] as const;
export type Metric = (typeof METRICS)[number];
export function metricKey(value: string): Metric | null {
  return (METRICS as readonly string[]).includes(value) ? (value as Metric) : null;
}

/**
 * How a rule's threshold reads: first response time is the only metric
 * where lower wins and its value is seconds; CSAT is out of 5, QA out of
 * 100. An unknown metric gets a plain "≥ value".
 */
export function thresholdParts(metric: string, threshold: number) {
  switch (metricKey(metric)) {
    case 'frt':
      return { sign: '≤' as const, seconds: true, scale: null };
    case 'csat':
      return { sign: '≥' as const, seconds: false, scale: 5 };
    case 'qa':
      return { sign: '≥' as const, seconds: false, scale: 100 };
    default:
      return { sign: '≥' as const, seconds: false, scale: null };
  }
}

type Item = {
  uuid: string;
  kind: string;
  parent_uuid?: string | null;
  due_at: string | null;
  completed_at: string | null;
};

/**
 * Goals with their follow-up tasks, then notes, like the panel's coaching
 * tab. Anything else (a task whose goal is not in the page, a kind the app
 * does not know) is kept under `other` instead of being dropped.
 */
export function groupItems<T extends Item>(items: T[]) {
  const goals = items.filter((i) => i.kind === 'goal');
  const goalIds = new Set(goals.map((g) => g.uuid));
  const tasks = new Map<string, T[]>();
  const other: T[] = [];
  for (const i of items) {
    if (i.kind === 'goal' || i.kind === 'note') continue;
    if (i.kind === 'task' && i.parent_uuid && goalIds.has(i.parent_uuid))
      tasks.set(i.parent_uuid, [...(tasks.get(i.parent_uuid) ?? []), i]);
    else other.push(i);
  }
  return { goals, tasks, notes: items.filter((i) => i.kind === 'note'), other };
}

const KINDS = ['note', 'goal', 'task'] as const;
export function kindKey(value: string): (typeof KINDS)[number] | null {
  return (KINDS as readonly string[]).includes(value) ? (value as (typeof KINDS)[number]) : null;
}

/** Done, overdue (open and past its date), due (open with a date) or plain. */
export function itemState(item: Item, now: number): 'completed' | 'overdue' | 'due' | 'none' {
  if (item.completed_at) return 'completed';
  if (!item.due_at) return 'none';
  return new Date(item.due_at).getTime() < now ? 'overdue' : 'due';
}

/** Reviewers 20 points or more apart on a criterion: the panel flags it as a high spread. */
export const SPREAD_FLAG = 20;

type Variance = { count: number; mean: number; variance: number; range: number };
type Criterion = { key?: string; label?: string; weight?: number; critical?: boolean };

/** Rubric rows with their statistics (only a closed session has them). */
export function criteriaRows(rubric: Criterion[], criteria: Record<string, Variance> | null | undefined) {
  return rubric.map((c, index) => {
    const stats =
      c.key && criteria && Object.prototype.hasOwnProperty.call(criteria, c.key) ? (criteria[c.key] ?? null) : null;
    return {
      key: c.key || `criterion-${index}`,
      label: c.label || readable(c.key),
      weight: c.weight ?? 0,
      critical: !!c.critical,
      stats,
      spread: !!stats && stats.range >= SPREAD_FLAG,
    };
  });
}
