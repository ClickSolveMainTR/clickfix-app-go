/**
 * The widget snapshot (CF-152, docs/mobile/widgets.md): what the app writes
 * to the App Group for the iOS widgets / controls / Siri and to local storage
 * for the Android widgets. Built from the same API answers the app screens
 * use; free of app imports so it runs under `node --test`.
 */

export const SNAPSHOT_VERSION = 1;

/** How many rows each surface may show (the large widget shows five). */
export const LIMITS = { top: 5, sla: 5, today: 6 } as const;

/** Conversations whose next SLA target is this close count as "at risk". */
export const SLA_WINDOW_MINUTES = 60;

export type SnapshotState = 'ready' | 'signed_out' | 'no_org' | 'no_permission';
export type PresenceStatus = 'online' | 'away' | 'busy' | 'offline';

export type SnapshotConversation = {
  uuid: string;
  contact: string;
  channel: string;
  preview: string;
  at: string;
  unread: number;
  direction: 'in' | 'out' | 'internal';
};

export type SnapshotSLA = {
  uuid: string;
  contact: string;
  channel: string;
  subject: string;
  due_at: string;
  breached: boolean;
};

export type SnapshotAppointment = {
  uuid: string;
  title: string;
  with: string;
  starts_at: string;
  ends_at: string;
  kind: 'video' | 'phone' | 'in_person' | 'custom_url';
  join: string | null;
};

export type WidgetSnapshot = {
  v: typeof SNAPSHOT_VERSION;
  generated_at: string;
  state: SnapshotState;
  locale: 'tr' | 'en';
  org: { slug: string; name: string } | null;
  user: { name: string } | null;
  can: { inbox: boolean; appointments: boolean; presence: boolean };
  presence: PresenceStatus | null;
  inbox: { unread: number; mine: number; open: number; breached: number; top: SnapshotConversation[] } | null;
  sla: SnapshotSLA[] | null;
  today: SnapshotAppointment[] | null;
};

/** The parts of the API answers the snapshot reads (subsets of the OpenAPI types). */
export type ApiConversation = {
  uuid: string;
  channel_type: string;
  contact: { name: string; email?: string | null; phone?: string | null };
  subject: string;
  status: string;
  unread_count: number;
  last_message_preview: string;
  last_message_direction: 'in' | 'out' | 'internal';
  last_message_at: string;
  first_response_at?: string | null;
  first_response_due_at: string | null;
  next_response_due_at: string | null;
  resolution_due_at: string | null;
  sla_breached_at: string | null;
};

export type ApiCounts = { open: number; mine: number; breached: number };

export type ApiAppointment = {
  uuid: string;
  status: string;
  title: string;
  starts_at: string;
  ends_at: string;
  location_kind: SnapshotAppointment['kind'];
  video_room_uuid: string | null;
  invitee?: { name?: string | null } | null;
  contact?: { name?: string | null } | null;
};

export type SnapshotInput = {
  now: Date;
  locale: string;
  signedIn: boolean;
  org: { slug: string; name: string } | null;
  user: { name: string } | null;
  can: { inbox: boolean; appointments: boolean; presence: boolean };
  presence?: PresenceStatus | null;
  counts?: ApiCounts | null;
  /** Newest activity first (`sort=-last_message_at`). */
  recent?: ApiConversation[] | null;
  /** Number of open conversations with unread messages (`unread=1`, the page total). */
  unreadTotal?: number | null;
  /** Open conversations whose SLA is due soon or already breached. */
  atRisk?: ApiConversation[] | null;
  appointments?: ApiAppointment[] | null;
};

const ONE_LINE = /\s+/g;
const PREVIEW_MAX = 140;

/** One line, trimmed, at most `max` characters (widgets have no room for more). */
export function oneLine(s: string | null | undefined, max = PREVIEW_MAX): string {
  const flat = (s ?? '').replace(ONE_LINE, ' ').trim();
  return flat.length > max ? `${flat.slice(0, max - 1).trimEnd()}…` : flat;
}

function contactName(c: ApiConversation['contact']): string {
  return oneLine(c.name || c.phone || c.email || '', 60);
}

const DIRECTIONS = new Set(['in', 'out', 'internal']);

function toConversation(c: ApiConversation): SnapshotConversation {
  return {
    uuid: c.uuid,
    contact: contactName(c.contact),
    channel: c.channel_type,
    preview: oneLine(c.last_message_preview),
    at: c.last_message_at,
    unread: Math.max(0, c.unread_count | 0),
    direction: DIRECTIONS.has(c.last_message_direction) ? c.last_message_direction : 'in',
  };
}

/** The SLA target that is due next: first response until answered, then the reply / resolution clocks. */
export function nextDue(c: ApiConversation): string | null {
  const candidates = [
    c.first_response_at ? null : c.first_response_due_at,
    c.next_response_due_at,
    c.resolution_due_at,
  ].filter((d): d is string => !!d && !Number.isNaN(Date.parse(d)));
  if (!candidates.length) return null;
  return candidates.reduce((a, b) => (Date.parse(a) <= Date.parse(b) ? a : b));
}

/** Breached first, then the nearest due time; conversations without a clock are dropped. */
export function slaAtRisk(list: ApiConversation[], now: Date, windowMinutes = SLA_WINDOW_MINUTES): SnapshotSLA[] {
  const limit = now.getTime() + windowMinutes * 60_000;
  const rows: SnapshotSLA[] = [];
  const seen = new Set<string>();
  for (const c of list) {
    if (seen.has(c.uuid) || c.status === 'resolved' || c.status === 'closed') continue;
    const due = nextDue(c);
    const breached = !!c.sla_breached_at || (!!due && Date.parse(due) <= now.getTime());
    if (!due && !breached) continue;
    if (!breached && due && Date.parse(due) > limit) continue;
    seen.add(c.uuid);
    rows.push({
      uuid: c.uuid,
      contact: contactName(c.contact),
      channel: c.channel_type,
      subject: oneLine(c.subject || c.last_message_preview, 80),
      due_at: due ?? (c.sla_breached_at as string),
      breached,
    });
  }
  rows.sort((a, b) => Number(b.breached) - Number(a.breached) || Date.parse(a.due_at) - Date.parse(b.due_at));
  return rows.slice(0, LIMITS.sla);
}

function sameLocalDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

const LIVE_APPOINTMENT = new Set(['scheduled', 'confirmed']);

/** Today's appointments (device time zone) that have not ended or been cancelled, earliest first. */
export function todayAppointments(list: ApiAppointment[], now: Date): SnapshotAppointment[] {
  return list
    .filter((a) => {
      const start = new Date(a.starts_at);
      const end = Date.parse(a.ends_at);
      return LIVE_APPOINTMENT.has(a.status) && sameLocalDay(start, now) && end > now.getTime();
    })
    .sort((a, b) => Date.parse(a.starts_at) - Date.parse(b.starts_at))
    .slice(0, LIMITS.today)
    .map((a) => ({
      uuid: a.uuid,
      title: oneLine(a.title, 80),
      with: oneLine(a.contact?.name || a.invitee?.name || '', 60),
      starts_at: a.starts_at,
      ends_at: a.ends_at,
      kind: a.location_kind,
      join: a.location_kind === 'video' && a.video_room_uuid ? `clickfix:///video/${a.video_room_uuid}` : null,
    }));
}

function emptySnapshot(input: SnapshotInput, state: SnapshotState): WidgetSnapshot {
  return {
    v: SNAPSHOT_VERSION,
    generated_at: input.now.toISOString(),
    state,
    locale: input.locale === 'en' ? 'en' : 'tr',
    org: state === 'signed_out' ? null : input.org,
    user: state === 'signed_out' ? null : input.user,
    can: state === 'signed_out' ? { inbox: false, appointments: false, presence: false } : input.can,
    presence: null,
    inbox: null,
    sla: null,
    today: null,
  };
}

/**
 * The snapshot for the current session. Signed out, no organization and no
 * permission are explicit states, so the widgets can show an intentional
 * message instead of empty numbers.
 */
export function buildSnapshot(input: SnapshotInput): WidgetSnapshot {
  if (!input.signedIn) return emptySnapshot(input, 'signed_out');
  if (!input.org) return emptySnapshot(input, 'no_org');
  if (!input.can.inbox && !input.can.appointments && !input.can.presence) return emptySnapshot(input, 'no_permission');

  const snap = emptySnapshot(input, 'ready');
  snap.presence = input.can.presence ? (input.presence ?? null) : null;
  if (input.can.inbox) {
    const recent = input.recent ?? [];
    snap.inbox = {
      unread: Math.max(0, input.unreadTotal ?? recent.filter((c) => c.unread_count > 0).length),
      mine: Math.max(0, input.counts?.mine ?? 0),
      open: Math.max(0, input.counts?.open ?? 0),
      breached: Math.max(0, input.counts?.breached ?? 0),
      top: recent.slice(0, LIMITS.top).map(toConversation),
    };
    snap.sla = slaAtRisk(input.atRisk ?? [], input.now);
  }
  if (input.can.appointments) snap.today = todayAppointments(input.appointments ?? [], input.now);
  return snap;
}

/** Spotlight rows for the conversations the snapshot knows (recent and at risk), without duplicates. */
export function spotlightItems(snap: WidgetSnapshot): { id: string; title: string; subtitle: string; url: string }[] {
  const out = new Map<string, { id: string; title: string; subtitle: string; url: string }>();
  for (const c of snap.inbox?.top ?? []) {
    out.set(c.uuid, { id: c.uuid, title: c.contact, subtitle: c.preview, url: `clickfix:///conversation/${c.uuid}` });
  }
  for (const s of snap.sla ?? []) {
    if (!out.has(s.uuid))
      out.set(s.uuid, { id: s.uuid, title: s.contact, subtitle: s.subject, url: `clickfix:///conversation/${s.uuid}` });
  }
  return [...out.values()];
}

/** Stable content for comparing two snapshots (ignores the generation time). */
export function snapshotKey(snap: WidgetSnapshot): string {
  return JSON.stringify({ ...snap, generated_at: '' });
}
