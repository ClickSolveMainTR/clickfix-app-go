import * as Bridge from 'clickfix-widget-bridge';
import { Platform } from 'react-native';

import { api, orgHeader, unwrap, unwrapVoid } from '@/core/api/client';
import { API_URL } from '@/core/config';
import { i18n } from '@/core/i18n';
import { queryClient } from '@/core/query';
import { readJSON, storage, writeJSON } from '@/core/storage';
import { can, P } from '@/core/auth/permissions';
import { useSession } from '@/core/auth/session';
import { presenceKeys } from '@/features/presence/keys';
import type { PresenceMe } from '@/features/presence/presence';

import {
  type ApiAppointment,
  type ApiConversation,
  buildSnapshot,
  type PresenceStatus,
  SLA_WINDOW_MINUTES,
  snapshotKey,
  spotlightItems,
  type WidgetSnapshot,
} from './snapshot-core';
import { deviceId } from './device';
import { syncUrgentActivity } from './live-activities';
import { shouldRenewWidgetToken, type StoredWidgetToken } from './tokens-core';

/**
 * Keeps the OS surfaces in step with the app (CF-152, docs/mobile/widgets.md):
 * builds the widget snapshot from the same endpoints the screens use, hands it
 * to the iOS App Group (widgets, controls, Siri) or the Android widgets, keeps
 * the scoped widget token fresh and indexes recent conversations in Spotlight.
 */

const TOKEN_KEY = 'widgets.token';
/** Android widgets render from here (headless task in the app process). */
export const ANDROID_SNAPSHOT_KEY = 'widgets.snapshot';

const MIN_INTERVAL_MS = 15_000;
let lastRun = 0;
let lastKey = '';
let timer: ReturnType<typeof setTimeout> | null = null;
let running: Promise<void> | null = null;

/**
 * Refreshes the snapshot soon. Events arrive in bursts (a new message moves
 * the list, the counts and the SLA at once), so calls inside the interval are
 * folded into one trailing run; `force` skips the wait (sign-in, foreground).
 */
export function scheduleWidgetSync(force = false) {
  if (timer) clearTimeout(timer);
  const wait = force ? 0 : Math.max(1_500, MIN_INTERVAL_MS - (Date.now() - lastRun));
  timer = setTimeout(() => {
    timer = null;
    void syncWidgets();
  }, wait);
}

export async function syncWidgets(): Promise<void> {
  if (running) return running;
  running = run().finally(() => {
    running = null;
  });
  return running;
}

async function run() {
  lastRun = Date.now();
  const snap = await collect().catch(() => null);
  if (!snap) return;
  publish(snap);
  if (Platform.OS === 'ios') syncUrgentActivity(snap.state === 'ready' ? snap.sla : null);
  if (snap.state === 'ready' && snap.org && snap.can.presence) await ensureWidgetToken(snap.org.slug);
}

type Page<T> = { items: T[]; total: number };

async function list(query: Record<string, string | number>): Promise<Page<ApiConversation>> {
  return unwrap(
    api.GET('/v1/tenant/conversations', {
      params: { header: orgHeader(), query: query as never },
    }),
  ) as Promise<Page<ApiConversation>>;
}

async function collect(): Promise<WidgetSnapshot> {
  const { status, me, organization } = useSession.getState();
  const now = new Date();
  const perms = {
    inbox: can(organization, P.conversationsRead),
    appointments: can(organization, P.appointmentsRead),
    // Any member may set their presence (backend presence/routes.go).
    presence: !!organization,
  };
  const base = {
    now,
    locale: i18n.language,
    signedIn: status === 'signedIn' && !!me,
    org: organization ? { slug: organization.slug, name: organization.name } : null,
    user: me ? { name: [me.user.name, me.user.surname].filter(Boolean).join(' ') } : null,
    can: perms,
  };
  if (!base.signedIn || !organization) return buildSnapshot(base);

  const open = 'open,pending';
  const [counts, recent, unread, dueSoon, breached, appointments] = await Promise.all([
    perms.inbox
      ? unwrap(api.GET('/v1/tenant/conversations/counts', { params: { header: orgHeader() } })).catch(() => null)
      : null,
    perms.inbox ? list({ status: open, assignee: 'me', sort: '-last_message_at', limit: 5 }).catch(() => null) : null,
    perms.inbox ? list({ status: open, unread: '1', limit: 1 }).catch(() => null) : null,
    perms.inbox
      ? list({ status: open, assignee: 'me', sla_due_within: SLA_WINDOW_MINUTES, limit: 10 }).catch(() => null)
      : null,
    perms.inbox ? list({ status: open, assignee: 'me', sla_breached: '1', limit: 5 }).catch(() => null) : null,
    perms.appointments
      ? (unwrap(
          api.GET('/v1/tenant/appointments/upcoming', {
            params: { header: orgHeader(), query: { scope: 'mine', limit: 20 } },
          }),
        ).catch(() => null) as Promise<{ items: ApiAppointment[] } | null>)
      : null,
  ]);
  return buildSnapshot({
    ...base,
    presence: presenceNow(organization.slug),
    counts,
    recent: recent?.items ?? null,
    unreadTotal: unread?.total ?? null,
    atRisk: [...(breached?.items ?? []), ...(dueSoon?.items ?? [])],
    appointments: appointments?.items ?? null,
  });
}

/** The status the app last saw, unless a widget / control / Siri changed it since. */
function presenceNow(org: string): PresenceStatus | null {
  const me = queryClient.getQueryData<PresenceMe>(presenceKeys.me(org));
  const override = Platform.OS === 'ios' ? Bridge.readPresenceOverrideParsed() : null;
  if (override && (!me || Date.parse(override.at) > queryClient.getQueryState(presenceKeys.me(org))!.dataUpdatedAt))
    return override.status;
  return (me?.status as PresenceStatus | undefined) ?? null;
}

function publish(snap: WidgetSnapshot) {
  const key = snapshotKey(snap);
  if (key === lastKey) return;
  lastKey = key;
  const json = JSON.stringify(snap);
  if (Platform.OS === 'ios') {
    Bridge.writeSnapshot(json);
    void (snap.state === 'ready' ? Bridge.indexSpotlight(spotlightItems(snap)) : Bridge.clearSpotlight());
  } else if (Platform.OS === 'android') {
    storage.set(ANDROID_SNAPSHOT_KEY, json);
    void import('./android/update').then((m) => m.updateAndroidWidgets()).catch(() => undefined);
  }
}

/** The snapshot the Android widgets render (headless task). */
export function readAndroidSnapshot(): WidgetSnapshot | null {
  return readJSON<WidgetSnapshot>(ANDROID_SNAPSHOT_KEY);
}

/**
 * Renews the scoped widget token the iOS intents use for background calls
 * (status from a widget, a control or Siri). The refresh token never leaves
 * the app's own Keychain group.
 */
async function ensureWidgetToken(org: string) {
  if (Platform.OS !== 'ios' || !Bridge.isAvailable()) return;
  const stored = readJSON<StoredWidgetToken>(TOKEN_KEY);
  if (!shouldRenewWidgetToken(stored, org, Date.now())) return;
  try {
    const res = await unwrap(
      api.POST('/v1/mobile/widget-token', { params: { header: orgHeader() }, body: { device_id: deviceId() } }),
    );
    const ok = Bridge.setCredential({ token: res.token, expires_at: res.expires_at, api_url: API_URL, org });
    if (ok) writeJSON(TOKEN_KEY, { org, issuedAt: Date.now(), expiresAt: res.expires_at });
  } catch {
    // Offline: the next foreground tries again; the widgets still read the snapshot.
  }
}

/**
 * Sign-out: the surfaces show the signed-out state, the widget token is
 * revoked (while the access token still works) and Spotlight is emptied.
 */
export async function resetWidgets(opts: { remote: boolean }) {
  if (timer) clearTimeout(timer);
  timer = null;
  const hadToken = !!storage.getString(TOKEN_KEY);
  storage.remove(TOKEN_KEY);
  if (opts.remote && hadToken)
    await unwrapVoid(api.DELETE('/v1/mobile/widget-token', { params: { query: { device_id: deviceId() } } })).catch(
      () => undefined,
    );
  lastKey = '';
  const signedOut = buildSnapshot({
    now: new Date(),
    locale: i18n.language,
    signedIn: false,
    org: null,
    user: null,
    can: { inbox: false, appointments: false, presence: false },
  });
  if (Platform.OS === 'ios') {
    Bridge.setCredential(null);
    void Bridge.clearSpotlight();
  }
  publish(signedOut);
}
