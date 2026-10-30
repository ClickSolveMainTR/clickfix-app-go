/** Pure report helpers (period range, change), run under `node --test`. */

export type Period = 7 | 30 | 90;

/** [from, to) for the last `days` days ending tomorrow 00:00, and the matching bucket. */
export function periodRange(days: Period, now = new Date()) {
  const to = new Date(now);
  to.setHours(24, 0, 0, 0);
  const from = new Date(to);
  from.setDate(from.getDate() - days);
  return {
    from: from.toISOString(),
    to: to.toISOString(),
    bucket: days > 31 ? ('week' as const) : ('day' as const),
  };
}

/** Relative change against the previous period, or null when there is nothing to compare. */
export function delta(now: number | null | undefined, before: number | null | undefined): number | null {
  if (now == null || before == null || before === 0) return null;
  return (now - before) / before;
}

export function changeTone(change: number | null, lowerIsBetter = false): 'neutral' | 'good' | 'bad' {
  if (change == null || !Number.isFinite(change) || Math.round(Math.abs(change) * 100) === 0)
    return 'neutral';
  return (lowerIsBetter ? change < 0 : change > 0) ? 'good' : 'bad';
}
