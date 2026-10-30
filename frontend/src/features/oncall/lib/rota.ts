import { addDays, addMinutes, format, startOfDay } from "date-fns";

import type { OncallLayer, OncallOverride } from "../services/oncall.service";

/**
 * Who is on call at `at`. Port of the API's `oncall.Agents`: half-open
 * windows, later layers win, then the latest matching override.
 */
export function agentsAt(
  layers: OncallLayer[],
  overrides: OncallOverride[],
  at: Date,
): string[] {
  const t = at.getTime();
  let out: string[] = [];
  for (const l of layers) {
    const start = Date.parse(l.starts_at);
    const end = l.ends_at ? Date.parse(l.ends_at) : null;
    if (
      t < start ||
      (end !== null && t >= end) ||
      l.agents.length === 0 ||
      l.rotation_minutes < 1
    )
      continue;
    const n = Math.floor(Math.floor((t - start) / 60000) / l.rotation_minutes);
    out = [l.agents[n % l.agents.length]!];
  }
  for (const o of overrides) {
    if (t >= Date.parse(o.starts_at) && t < Date.parse(o.ends_at))
      out = [o.agent];
  }
  return out;
}

export type Shift = {
  start: Date;
  end: Date;
  agent: string | null;
  override: boolean;
};

const STEP_MINUTES = 30;

/**
 * On-call shifts of one local day, sampled every 30 minutes and merged.
 * Rotations shorter than the step are approximated; the API resolves the
 * exact agent when it pages.
 */
export function shiftsOfDay(
  layers: OncallLayer[],
  overrides: OncallOverride[],
  day: Date,
): Shift[] {
  const shifts: Shift[] = [];
  const begin = startOfDay(day);
  const end = addDays(begin, 1);
  for (let at = begin; at < end; at = addMinutes(at, STEP_MINUTES)) {
    const agent = agentsAt(layers, overrides, at)[0] ?? null;
    const override = overrides.some(
      (o) =>
        at.getTime() >= Date.parse(o.starts_at) &&
        at.getTime() < Date.parse(o.ends_at),
    );
    const last = shifts[shifts.length - 1];
    if (last && last.agent === agent && last.override === override) {
      last.end = addMinutes(at, STEP_MINUTES);
    } else {
      shifts.push({
        start: at,
        end: addMinutes(at, STEP_MINUTES),
        agent,
        override,
      });
    }
  }
  return shifts;
}

/** `yyyy-MM-dd` + `HH:mm` in the browser's zone → RFC 3339 (UTC). */
export function toInstant(day: string, time: string) {
  if (!day) return "";
  const d = new Date(`${day}T${time || "00:00"}:00`);
  return Number.isNaN(d.getTime()) ? "" : d.toISOString();
}

/** RFC 3339 → local `{ day, time }` for the date and time inputs. */
export function fromInstant(value?: string | null) {
  if (!value) return { day: "", time: "" };
  const d = new Date(value);
  return { day: format(d, "yyyy-MM-dd"), time: format(d, "HH:mm") };
}

/** Rotation length presets (minutes). */
export const ROTATION_PRESETS = [
  { minutes: 480, key: "8h" },
  { minutes: 720, key: "12h" },
  { minutes: 1440, key: "daily" },
  { minutes: 10080, key: "weekly" },
] as const;
