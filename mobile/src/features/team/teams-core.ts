/**
 * Team form rules (CF-166), the same ones the API enforces
 * (`backend/internal/modules/teams/usecase/service.go`) and the panel's form
 * applies (`frontend/src/features/teams/schemas/team-form.ts`,
 * `components/skills-input.tsx`). Free of app imports so it runs under
 * `node --test`.
 */

export type WeekDay = 'mon' | 'tue' | 'wed' | 'thu' | 'fri' | 'sat' | 'sun';
export type DayRow = { day: WeekDay; open: boolean; start: string; end: string };

/** Monday first, the order the API requires. */
export const WEEK_DAYS: WeekDay[] = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];

export const MAX_SKILLS = 30;

const TIME_RE = /^([01][0-9]|2[0-3]):[0-5][0-9]$/;
const COLOR_RE = /^#[0-9A-Fa-f]{6}$/;
const SKILL_RE = /^[\p{L}\p{N}][\p{L}\p{N} ._+-]{0,39}$/u;

export type TeamForm = {
  name: string;
  description: string;
  color: string;
  skills: string[];
  inherit_hours: boolean;
  timezone: string;
  days: DayRow[];
};

type TeamLike = {
  name: string;
  description: string;
  color: string;
  skills: string[];
  timezone: string | null;
  working_hours: { days: DayRow[] } | null;
};

/** Weekdays 09:00-18:00, weekend closed (the panel's default). */
export function defaultWeekDays(): DayRow[] {
  return WEEK_DAYS.map((day) => ({ day, open: day !== 'sat' && day !== 'sun', start: '09:00', end: '18:00' }));
}

export function formOf(team?: TeamLike | null): TeamForm {
  const own = team?.working_hours ?? null;
  const days = own
    ? WEEK_DAYS.map((day) => {
        const row = own.days.find((d) => d.day === day);
        return { day, open: row?.open ?? false, start: row?.start || '09:00', end: row?.end || '18:00' };
      })
    : defaultWeekDays();
  return {
    name: team?.name ?? '',
    description: team?.description ?? '',
    color: team?.color ?? '',
    skills: team?.skills ?? [],
    inherit_hours: team ? own === null : true,
    timezone: team?.timezone ?? '',
    days,
  };
}

export type TeamRequest = {
  name: string;
  description: string;
  color: string;
  skills: string[];
  inherit_hours?: boolean;
  timezone?: string;
  working_hours?: { days: DayRow[] };
};

/** Form → request. `inherit_hours` drops the team's own hours and time zone. */
export function toRequest(form: TeamForm): TeamRequest {
  const base = {
    name: form.name.trim(),
    description: form.description.trim(),
    color: form.color.trim().toUpperCase(),
    skills: form.skills,
  };
  if (form.inherit_hours) return { ...base, inherit_hours: true };
  return {
    ...base,
    working_hours: { days: form.days },
    ...(form.timezone ? { timezone: form.timezone } : {}),
  };
}

export type FormErrorKey =
  'name_required' | 'name_too_long' | 'description_too_long' | 'color' | 'too_many_skills' | 'time' | 'end_after_start';

/**
 * Field → error key; empty when the API would accept the form. Day rows use
 * `days.<index>.start` / `days.<index>.end`. Hours are only checked when the
 * team keeps its own.
 */
export function formErrors(form: TeamForm): Record<string, FormErrorKey> {
  const out: Record<string, FormErrorKey> = {};
  const name = form.name.trim();
  if (!name) out.name = 'name_required';
  else if (name.length > 120) out.name = 'name_too_long';
  if (form.description.length > 1000) out.description = 'description_too_long';
  const color = form.color.trim();
  if (color && !COLOR_RE.test(color)) out.color = 'color';
  if (form.skills.length > MAX_SKILLS) out.skills = 'too_many_skills';
  if (!form.inherit_hours)
    form.days.forEach((d, i) => {
      const start = TIME_RE.test(d.start);
      const end = TIME_RE.test(d.end);
      if (!start) out[`days.${i}.start`] = 'time';
      if (!end) out[`days.${i}.end`] = 'time';
      else if (d.open && start && d.end <= d.start) out[`days.${i}.end`] = 'end_after_start';
    });
  return out;
}

export function sameForm(a: TeamForm, b: TeamForm) {
  return JSON.stringify(toRequest(a)) === JSON.stringify(toRequest(b)) && a.inherit_hours === b.inherit_hours;
}

/** Same normalization as the API: trimmed and lower-cased. */
export function normalizeSkill(value: string) {
  return value.trim().toLocaleLowerCase('tr-TR');
}

export type AddSkill = { skills: string[]; error?: 'skill' | 'too_many_skills' };

/** Adds the typed tags (comma separated) to the list, sorted; the first rejected tag stops with its error. */
export function addSkills(current: string[], raw: string): AddSkill {
  let skills = current;
  for (const part of raw.split(',')) {
    const tag = normalizeSkill(part);
    if (!tag || skills.includes(tag)) continue;
    if (!SKILL_RE.test(tag)) return { skills, error: 'skill' };
    if (skills.length >= MAX_SKILLS) return { skills, error: 'too_many_skills' };
    skills = [...skills, tag].sort((a, b) => a.localeCompare(b));
  }
  return { skills };
}

/** `org` when the team follows the organization's hours, else how many days it is open. */
export function hoursSummary(team: { working_hours: { days: { open: boolean }[] } | null }) {
  if (!team.working_hours) return { kind: 'org' as const };
  return { kind: 'own' as const, open: team.working_hours.days.filter((d) => d.open).length };
}

export function personName(user: { name: string; surname: string; email: string }) {
  return `${user.name} ${user.surname}`.trim() || user.email;
}

const FALLBACK_TIME_ZONES = [
  'Europe/Istanbul',
  'Europe/London',
  'Europe/Berlin',
  'Europe/Amsterdam',
  'Europe/Paris',
  'Europe/Madrid',
  'Europe/Rome',
  'Europe/Athens',
  'Europe/Moscow',
  'Asia/Baku',
  'Asia/Dubai',
  'Asia/Riyadh',
  'Asia/Tbilisi',
  'Asia/Almaty',
  'America/New_York',
  'America/Chicago',
  'America/Los_Angeles',
];

/** IANA zones of the runtime (sorted), else the panel's fallback list; `keep` stays listed even when unknown. */
export function timeZones(keep = ''): string[] {
  const intl = Intl as typeof Intl & { supportedValuesOf?: (key: 'timeZone') => string[] };
  let zones = FALLBACK_TIME_ZONES;
  try {
    zones = intl.supportedValuesOf?.('timeZone') ?? FALLBACK_TIME_ZONES;
  } catch {
    // Hermes without the list: the fallback stands.
  }
  return [...new Set([...zones, 'UTC', ...(keep ? [keep] : [])])].sort((a, b) => a.localeCompare(b));
}
