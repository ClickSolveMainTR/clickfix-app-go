/**
 * Shift (working hours) of a team for "today", in the team's time zone
 * (CF-82). Free of app imports so it runs under `node --test`.
 */
export type WorkingDay = {
  day: 'mon' | 'tue' | 'wed' | 'thu' | 'fri' | 'sat' | 'sun';
  open: boolean;
  start: string;
  end: string;
};

const DAYS: WorkingDay['day'][] = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'];

/** Weekday and "HH:MM" of `now` in `timeZone`. */
export function localClock(now: Date, timeZone: string): { day: WorkingDay['day']; time: string } {
  const parts = new Intl.DateTimeFormat('en-GB', {
    timeZone,
    weekday: 'short',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(now);
  const get = (t: string) => parts.find((p) => p.type === t)?.value ?? '';
  const wd = get('weekday').toLowerCase().slice(0, 3) as WorkingDay['day'];
  return { day: DAYS.includes(wd) ? wd : 'mon', time: `${get('hour')}:${get('minute')}` };
}

export type ShiftToday =
  { state: 'org' } | { state: 'closed' } | { state: 'before' | 'on' | 'after'; start: string; end: string };

/**
 * Today's shift: `org` when the team follows the organization's business
 * hours (`working_hours` null), `closed` on a day off, else where now falls
 * relative to the hours. An overnight shift (end before start) counts as on
 * after the start or before the end.
 */
export function shiftToday(days: WorkingDay[] | null | undefined, now: Date, timeZone: string): ShiftToday {
  if (!days) return { state: 'org' };
  const { day, time } = localClock(now, timeZone);
  const d = days.find((x) => x.day === day);
  if (!d || !d.open) return { state: 'closed' };
  const overnight = d.end <= d.start;
  const on = overnight ? time >= d.start || time < d.end : time >= d.start && time < d.end;
  return { state: on ? 'on' : !overnight && time >= d.end ? 'after' : 'before', start: d.start, end: d.end };
}
