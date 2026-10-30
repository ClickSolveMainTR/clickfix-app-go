/**
 * Pure inbox helpers, ported from the panel (`frontend/src/features/inbox/lib/format.ts`,
 * `components/message-timeline.tsx`, `hooks/use-inbox.ts`). Kept free of app
 * imports so they run under `node --test`.
 */

export type InboxView = 'all' | 'mine' | 'unassigned' | 'mentions';
export type InboxStatusTab = 'open' | 'snoozed' | 'resolved' | 'all';

/** Status tab → `status` query (open covers open + pending, like the panel). */
export const STATUS_QUERY: Record<InboxStatusTab, string | undefined> = {
  open: 'open,pending',
  snoozed: 'snoozed',
  resolved: 'resolved,closed',
  all: undefined,
};

/** View → `assignee` / `mentioned` query. */
export const VIEW_QUERY: Record<InboxView, { assignee?: string; mentioned?: '1' }> = {
  all: {},
  mine: { assignee: 'me' },
  unassigned: { assignee: 'unassigned' },
  mentions: { mentioned: '1' },
};

/** Two-letter initials for avatars. */
export function initials(name: string | null | undefined): string {
  const parts = (name ?? '').trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return '?';
  const first = parts[0]?.[0] ?? '';
  const last = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? '') : '';
  return (first + last).toLocaleUpperCase('tr');
}

type ContactLike = { contact: { name: string; email: string | null; phone: string | null } };

/** Display name of a conversation's contact. */
export function contactLabel(c: ContactLike): string {
  return c.contact.name || c.contact.email || c.contact.phone || '—';
}

type Translate = (key: string, params?: Record<string, string>) => string;

/** Timeline line of a system event message (`kind: event`), in the panel's words (`inbox:events.*`). */
export function eventText(attributes: Record<string, unknown> | null | undefined, t: Translate): string {
  const a = (attributes ?? {}) as Record<string, string | undefined>;
  const actor = a.actor_name || t('inbox:sender.system');
  switch (a.event) {
    case 'assigned':
      if (!a.to) return t('inbox:events.unassigned', { actor });
      return a.to === actor ? t('inbox:events.took', { actor }) : t('inbox:events.assigned', { actor, to: a.to });
    case 'status_changed': {
      const [status] = (a.to ?? '').split('|');
      return t('inbox:events.status_changed', { actor, status: t(`inbox:status.${status}`) });
    }
    case 'priority_changed':
      return t('inbox:events.priority_changed', { actor, priority: t(`inbox:priority.${a.to}`) });
    case 'tag_added':
      return t('inbox:events.tag_added', { actor, tag: a.to ?? '' });
    case 'tag_removed':
      return t('inbox:events.tag_removed', { actor, tag: a.to ?? '' });
    case 'subject_changed':
      return t('inbox:events.subject_changed', { actor });
    case 'team_assigned':
      return a.to ? t('inbox:events.team_assigned', { actor, team: a.to }) : t('inbox:events.team_removed', { actor });
    case 'merged_from':
    case 'merged_into':
    case 'split_to':
    case 'split_from':
    case 'unlinked':
      return t(`inbox:events.${a.event}`, { actor, number: a.to ?? '' });
    case 'linked':
      return t('inbox:events.linked', {
        actor,
        number: a.to ?? '',
        relation: t(`conversation:details.relation_${a.relation ?? 'related'}`),
      });
    case 'side_conversation_started':
    case 'side_conversation_sent':
    case 'side_conversation_reply':
      return t(`inbox:events.${a.event}`, { actor, to: a.to ?? '', subject: a.subject ?? '' });
    case 'appointment_booked':
    case 'appointment_rescheduled':
    case 'appointment_confirmed':
    case 'appointment_completed':
    case 'appointment_no_show':
    case 'appointment_cancelled':
      return t(`inbox:events.${a.event}`, { actor, to: a.to ?? '', type: a.type ?? '' });
    case 'snoozed':
    case 'unsnoozed':
    case 'sla_breached':
    case 'sla_applied':
    case 'sla_paused':
    case 'sla_resumed':
      return t(`conversation:events.${a.event}`, { actor });
    default:
      return a.event ? t('conversation:events.updated') : '';
  }
}

type MessageLike = { uuid: string; client_id: string | null };
type MessagePageLike<M> = { items: M[] };

/**
 * Inserts or replaces a message in cached pages (newest page first), matched
 * by uuid or by the composer's `client_id`. A new message goes to the end of
 * the newest page. Returns the same pages object when nothing could be placed.
 */
export function upsertMessagePages<M extends MessageLike, P extends MessagePageLike<M>>(pages: P[], msg: M): P[] {
  const same = (m: M) => m.uuid === msg.uuid || (msg.client_id != null && m.client_id === msg.client_id);
  let found = false;
  const next = pages.map((p) => {
    if (!p.items.some(same)) return p;
    found = true;
    return { ...p, items: p.items.map((m) => (same(m) ? { ...m, ...msg } : m)) };
  });
  if (found) return next;
  const [newest, ...rest] = pages;
  if (!newest) return pages;
  return [{ ...newest, items: [...newest.items, msg] }, ...rest];
}

/** `uuid` v4 for `client_id` (idempotent sends); `crypto.randomUUID` is missing on older Hermes. */
export function clientId(random: () => number = Math.random): string {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = Math.floor(random() * 16);
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16);
  });
}

type SlaFields = {
  status: string;
  first_response_due_at: string | null;
  next_response_due_at: string | null;
  resolution_due_at: string | null;
};

export type SlaMetricKey = 'first_response' | 'next_response' | 'resolution';
export type SlaBadgeState = { metric: SlaMetricKey; ms: number; tone: 'overdue' | 'soon' | 'running' };

/** Milliseconds before a deadline at which the badge turns amber (panel `SlaBadge`). */
export const SLA_SOON_MS = 30 * 60_000;

/**
 * The nearest open SLA deadline of a conversation, ported from the panel's
 * `features/sla/components/sla-badge.tsx`. Resolved / closed conversations
 * and paused clocks (no due date) have none.
 */
export function slaBadge(c: SlaFields, now: number): SlaBadgeState | null {
  if (c.status === 'resolved' || c.status === 'closed') return null;
  const dues: { metric: SlaMetricKey; at: string | null }[] = [
    { metric: 'first_response', at: c.first_response_due_at },
    { metric: 'next_response', at: c.next_response_due_at },
    { metric: 'resolution', at: c.resolution_due_at },
  ];
  let best: SlaBadgeState | null = null;
  for (const d of dues) {
    if (!d.at) continue;
    const ms = new Date(d.at).getTime() - now;
    if (Number.isNaN(ms)) continue;
    if (!best || ms < best.ms) best = { metric: d.metric, ms, tone: 'running' };
  }
  if (!best) return null;
  best.tone = best.ms <= 0 ? 'overdue' : best.ms <= SLA_SOON_MS ? 'soon' : 'running';
  return best;
}

/** Working-time duration ("2 sa 15 dk" / "2h 15m"), panel `features/sla/lib/format.ts`. */
export function formatDuration(seconds: number, lang: string): string {
  const abs = Math.abs(Math.round(seconds / 60));
  const days = Math.floor(abs / 1440);
  const hours = Math.floor((abs % 1440) / 60);
  const minutes = abs % 60;
  const tr = !lang.startsWith('en');
  const parts: string[] = [];
  if (days) parts.push(`${days}${tr ? ' g' : 'd'}`);
  if (hours) parts.push(`${hours}${tr ? ' sa' : 'h'}`);
  if (minutes || parts.length === 0) parts.push(`${minutes}${tr ? ' dk' : 'm'}`);
  return parts.slice(0, 2).join(' ');
}

/** Overdue badges show the largest unit; remaining clocks retain two units. */
export function formatSlaTime(seconds: number, lang: string): string {
  const duration = formatDuration(seconds, lang);
  if (seconds > 0) return duration;
  const [amount, unit] = duration.split(' ');
  return lang.startsWith('en') ? amount! : `${amount} ${unit}`;
}

export type SnoozePreset = 'one_hour' | 'three_hours' | 'tomorrow' | 'next_week';
export const SNOOZE_PRESETS: SnoozePreset[] = ['one_hour', 'three_hours', 'tomorrow', 'next_week'];

/** Snooze end for a preset, like the panel's header menu: +1 h, +3 h, tomorrow 09:00, next Monday 09:00 (local). */
export function snoozeUntil(preset: SnoozePreset, now: Date = new Date()): Date {
  const at = (days: number, hour: number) => {
    const d = new Date(now);
    d.setDate(d.getDate() + days);
    d.setHours(hour, 0, 0, 0);
    return d;
  };
  switch (preset) {
    case 'one_hour':
      return new Date(now.getTime() + 3_600_000);
    case 'three_hours':
      return new Date(now.getTime() + 3 * 3_600_000);
    case 'tomorrow':
      return at(1, 9);
    case 'next_week':
      return at((8 - now.getDay()) % 7 || 7, 9);
  }
}

export type AttachmentKind = 'image' | 'audio' | 'video' | 'file';

/** How the timeline shows an attachment, from its media type. */
export function attachmentKind(contentType: string | null | undefined): AttachmentKind {
  const t = (contentType ?? '').toLowerCase();
  if (t.startsWith('image/')) return 'image';
  if (t.startsWith('audio/')) return 'audio';
  if (t.startsWith('video/')) return 'video';
  return 'file';
}

/** "1,2 MB" style size for attachment chips. */
export function formatBytes(bytes: number, lang: string): string {
  const units = ['B', 'KB', 'MB', 'GB'];
  let n = Math.max(0, bytes);
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  const digits = i === 0 || n >= 10 ? 0 : 1;
  return `${n.toLocaleString(lang === 'en' ? 'en-GB' : 'tr-TR', { maximumFractionDigits: digits })} ${units[i]}`;
}

/** File name for a picked asset that has none (camera shots): `photo-20261028-1430.jpg`. */
export function fallbackFileName(mimeType: string | null | undefined, now: Date = new Date()): string {
  const ext = (mimeType ?? '').split('/')[1]?.split(';')[0]?.replace('jpeg', 'jpg') || 'bin';
  const pad = (n: number) => String(n).padStart(2, '0');
  const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
  const kind = attachmentKind(mimeType);
  return `${kind === 'image' ? 'photo' : kind === 'video' ? 'video' : kind === 'audio' ? 'audio' : 'file'}-${stamp}.${ext}`;
}
