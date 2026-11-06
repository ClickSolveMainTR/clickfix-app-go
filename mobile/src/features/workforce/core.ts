/**
 * Workforce read views (CF-170): forecast window and rows, timesheet
 * grouping. Same rules as the panel (`frontend/src/features/workforce/lib/
 * forecast.ts`, `timesheets.ts`). Free of app imports so it runs under
 * `node --test`.
 */

const HOUR = 3_600_000;
const DAY = 24 * HOUR;

export type Granularity = 'hour' | 'day';

/** Horizons per granularity. The API allows 31 days; hourly stops at 7 (168 rows) to stay readable on a phone. */
export const HORIZONS: Record<Granularity, readonly number[]> = { day: [7, 14, 31], hour: [1, 7] };

/** The panel's default model parameters; the phone view does not edit them. */
export const MODEL = { handle_seconds: 300, sla_seconds: 30, target_sla: 0.8, alpha: 0.3 } as const;

function floorUtcHour(d: Date) {
  const out = new Date(d);
  out.setUTCMinutes(0, 0, 0);
  return out;
}

/**
 * `[since, until)` of a forecast for `days` days starting today: whole UTC
 * hours, from the current hour (the API takes no past hour) to local
 * midnight `days` days on.
 */
export function forecastWindow(days: number, now = new Date()) {
  const start = new Date(now);
  start.setHours(0, 0, 0, 0);
  const since = floorUtcHour(now);
  const until = floorUtcHour(new Date(start.getTime() + days * DAY));
  return { since: since.toISOString(), until: until.toISOString() };
}

type Hour = {
  start: string;
  volume: number;
  required_agents: number;
  scheduled_agents: number;
  gap: number;
  capacity_limited: boolean;
};
type Day = {
  date: string;
  volume: number;
  required_agent_hours: number;
  scheduled_agent_hours: number;
  gap_agent_hours: number;
  capacity_limited: boolean;
};

/** One forecast bucket in a shape shared by hourly and daily results. */
export type ForecastRow = {
  key: string;
  /** Hour start (RFC 3339) or date (`yyyy-MM-dd`). */
  at: string;
  volume: number;
  required: number;
  scheduled: number;
  gap: number;
  capacityLimited: boolean;
};

/** Rows of a forecast; the bucket shape is read from the data, so a mismatched answer never shows `NaN`. */
export function forecastRows(buckets: (Hour | Day)[]): ForecastRow[] {
  return buckets.map((b) =>
    'date' in b
      ? {
          key: b.date,
          at: b.date,
          volume: b.volume,
          required: b.required_agent_hours,
          scheduled: b.scheduled_agent_hours,
          gap: b.gap_agent_hours,
          capacityLimited: b.capacity_limited,
        }
      : {
          key: b.start,
          at: b.start,
          volume: b.volume,
          required: b.required_agents,
          scheduled: b.scheduled_agents,
          gap: b.gap,
          capacityLimited: b.capacity_limited,
        },
  );
}

export function forecastTotals(rows: ForecastRow[]) {
  return rows.reduce(
    (acc, r) => ({
      volume: acc.volume + r.volume,
      peakRequired: Math.max(acc.peakRequired, r.required),
      gapBuckets: acc.gapBuckets + (r.gap > 0 ? 1 : 0),
      gap: acc.gap + r.gap,
      capacityLimited: acc.capacityLimited || r.capacityLimited,
    }),
    { volume: 0, peakRequired: 0, gapBuckets: 0, gap: 0, capacityLimited: false },
  );
}

/** Short of staff, exactly covered, or with agents to spare. */
export function coverage(row: ForecastRow): 'gap' | 'covered' | 'surplus' {
  if (row.gap > 0) return 'gap';
  return row.scheduled > row.required ? 'surplus' : 'covered';
}

/**
 * Bucket label in the organization's zone (the API groups by that zone, not
 * the phone's). A date is formatted at noon UTC so no zone shifts the day.
 */
export function bucketLabel(
  row: ForecastRow,
  granularity: Granularity,
  timeZone: string,
  locale: string,
  long = false,
) {
  try {
    if (granularity === 'day')
      return new Intl.DateTimeFormat(locale, {
        timeZone: 'UTC',
        weekday: 'short',
        day: 'numeric',
        month: 'short',
      }).format(new Date(`${row.at}T12:00:00Z`));
    return new Intl.DateTimeFormat(locale, {
      timeZone,
      weekday: 'short',
      ...(long ? { day: 'numeric', month: 'short' } : {}),
      hour: '2-digit',
      minute: '2-digit',
    }).format(new Date(row.at));
  } catch {
    return row.at;
  }
}

/** The timesheet API accepts at most 31 days per request. */
export const TIMESHEET_RANGES = [7, 14, 31] as const;
export type TimesheetRange = (typeof TIMESHEET_RANGES)[number];

/** `[since, until)`: the last `days` local days, today included. */
export function timesheetWindow(days: number, now = new Date()) {
  const until = new Date(now);
  until.setHours(24, 0, 0, 0);
  const since = new Date(until);
  since.setDate(since.getDate() - days);
  return { since: since.toISOString(), until: until.toISOString() };
}

type TimesheetRow = { agent_uuid: string; conversation_uuid?: string | null; state: string; seconds: number };

export type AgentTime = {
  agent: string;
  active: number;
  away: number;
  handling: number;
  /** Handling seconds per conversation (`null` = deleted conversation), longest first. */
  conversations: { uuid: string | null; seconds: number }[];
};

/**
 * Groups the API rows (agent × state × conversation) into one line per
 * agent, most working time first. Working time is active + handling; away
 * is reported separately. A state the app does not know is not counted.
 */
export function agentTimes(rows: TimesheetRow[]): AgentTime[] {
  const byAgent = new Map<string, AgentTime>();
  for (const row of rows) {
    let entry = byAgent.get(row.agent_uuid);
    if (!entry) {
      entry = { agent: row.agent_uuid, active: 0, away: 0, handling: 0, conversations: [] };
      byAgent.set(row.agent_uuid, entry);
    }
    if (row.state === 'active') entry.active += row.seconds;
    else if (row.state === 'away') entry.away += row.seconds;
    else if (row.state === 'handling') {
      entry.handling += row.seconds;
      entry.conversations.push({ uuid: row.conversation_uuid ?? null, seconds: row.seconds });
    }
  }
  const list = [...byAgent.values()];
  for (const a of list) a.conversations.sort((x, y) => y.seconds - x.seconds);
  return list.sort((x, y) => y.active + y.handling - (x.active + x.handling));
}
