/**
 * Voice channel call log (CF-169): how a call row reads. Wording and rules
 * follow the panel (`frontend/src/features/channels/components/voice-calls-page.tsx`).
 * Free of app imports so it runs under `node --test`.
 */

/** `t` of the `admin` namespace; every key here starts with `calls.`. */
export type T = (key: string, params?: Record<string, string | number>) => string;

/** Statuses with a label (Twilio / Netgsm names, as the panel lists them). */
const STATUSES = new Set([
  'queued',
  'initiated',
  'ringing',
  'in-progress',
  'answered',
  'completed',
  'busy',
  'failed',
  'no-answer',
  'canceled',
  'recording',
]);
const FAILED = new Set(['busy', 'failed', 'no-answer', 'canceled']);
const DIRECTIONS = new Set(['inbound', 'outbound', 'internal']);
const TRANSCRIPT_STATUSES = new Set(['unavailable', 'pending', 'processing', 'completed', 'failed']);

/** A value the app has no label for, shown readable instead of as a translation key. */
const readable = (raw: string) => raw.replace(/[._-]+/g, ' ').trim();

export function statusLabel(t: T, status: string) {
  return STATUSES.has(status) ? t(`calls.status.${status}`) : readable(status);
}

export function directionLabel(t: T, direction: string) {
  return DIRECTIONS.has(direction) ? t(`calls.direction.${direction}`) : readable(direction);
}

export function transcriptStatusLabel(t: T, status: string) {
  return TRANSCRIPT_STATUSES.has(status) ? t(`calls.transcript_status.${status}`) : readable(status);
}

/** Calls nobody talked on are shown as a problem, like the panel's red badge. */
export function callFailed(status: string) {
  return FAILED.has(status);
}

/** "m:ss", hours folded into minutes like the panel ("75:04"). */
export function duration(seconds: number | null | undefined) {
  const total = Number.isFinite(seconds) && (seconds as number) > 0 ? Math.floor(seconds as number) : 0;
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`;
}

type Numbers = { direction: string; from_number: string; to_number: string; internal_number?: string };

/** The other side of the call: who called in, or who was called. */
export function counterpart(call: Numbers) {
  const other = call.direction === 'inbound' ? call.from_number : call.to_number;
  return other || call.from_number || call.to_number || '—';
}

/** A call in the pages loaded so far (the API has no single-call endpoint). */
export function findCall<C extends { uuid: string }>(pages: { items: C[] }[] | undefined, uuid: string) {
  for (const page of pages ?? []) {
    const hit = page.items.find((c) => c.uuid === uuid);
    if (hit) return hit;
  }
  return undefined;
}

/** A call from another year than `now` shows its year too ("3 Ara 2025 14:30"). */
export function needsYear(iso: string | null | undefined, now = new Date()) {
  if (!iso) return false;
  const at = new Date(iso);
  return !Number.isNaN(at.getTime()) && at.getFullYear() !== now.getFullYear();
}

/** Why the transcript of a recorded call cannot be shown; only a plain failure is worth another try. */
export function transcriptProblem(status: number | null | undefined): 'gone' | 'forbidden' | 'failed' {
  if (status === 404) return 'gone';
  if (status === 403) return 'forbidden';
  return 'failed';
}
