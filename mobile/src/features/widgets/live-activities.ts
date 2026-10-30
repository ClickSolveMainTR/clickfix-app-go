import * as Bridge from 'clickfix-widget-bridge';
import { Platform } from 'react-native';

import { api, unwrapVoid } from '@/core/api/client';
import { readJSON, writeJSON } from '@/core/storage';

import { deviceId } from './device';
import type { SnapshotSLA } from './snapshot-core';
import {
  apnsEnvironment,
  liveActivityTokenBody,
  type LiveActivityTokenBody,
  type LiveActivityType,
  SentTokens,
} from './tokens-core';

/**
 * Live Activities and the Dynamic Island (CF-152): an ongoing call (LiveKit
 * room) and a conversation waiting for my reply with its SLA countdown. The
 * SwiftUI views live in targets/widgets; the app starts / updates / ends them
 * through the bridge module, and the backend starts the urgent one with
 * push-to-start and updates / ends both over APNs with the tokens registered
 * here. iOS only, and only while the member keeps them on in settings.
 */

const ENABLED_KEY = 'widgets.live_activities';
const sent = new SentTokens();
let subscriptions: { remove: () => void }[] = [];

/** The member's choice, mirrored locally so a call never waits on the network. */
export function liveActivitiesEnabled(): boolean {
  return readJSON<boolean>(ENABLED_KEY) ?? true;
}

export function setLiveActivitiesEnabledLocal(on: boolean) {
  writeJSON(ENABLED_KEY, on);
  if (!on) void Bridge.endAllActivities();
}

function available(): boolean {
  return Platform.OS === 'ios' && liveActivitiesEnabled() && Bridge.liveActivitiesEnabled();
}

async function register(body: LiveActivityTokenBody | null) {
  if (!body || !sent.claim(body)) return;
  try {
    await unwrapVoid(api.POST('/v1/mobile/live-activity-tokens', { body }));
  } catch {
    sent.release(body);
  }
}

function tokenBody(
  kind: 'push_to_start' | 'update',
  type: LiveActivityType,
  token: string,
  id?: string,
  ref?: string,
) {
  return liveActivityTokenBody({
    deviceId: deviceId(),
    kind,
    type,
    token,
    environment: apnsEnvironment(__DEV__),
    activityId: id,
    ref,
  });
}

/**
 * After sign-in: the push-to-start token goes to the API (so the backend can
 * open the urgent activity when an SLA is about to break) and so does the
 * update token of every running activity, including pushed ones.
 */
export function initLiveActivities() {
  if (!available() || subscriptions.length) return;
  subscriptions = [
    Bridge.addPushToStartTokenListener(({ type, token }) => void register(tokenBody('push_to_start', type, token))),
    Bridge.addPushTokenListener(({ id, type, ref, token }) => void register(tokenBody('update', type, token, id, ref))),
  ];
  Bridge.startObservingTokens();
}

/** Sign-out: no activity outlives the session, and tokens are re-sent for the next account. */
export async function resetLiveActivities() {
  subscriptions.forEach((s) => s.remove());
  subscriptions = [];
  sent.clear();
  activityRefs.clear();
  await Bridge.endAllActivities();
}

/** The member turned the setting on / off; the API stops or resumes pushes. */
export async function saveLiveActivitiesSetting(enabled: boolean) {
  await unwrapVoid(api.PUT('/v1/mobile/live-activity-settings', { body: { enabled } }));
  setLiveActivitiesEnabledLocal(enabled);
  if (enabled) initLiveActivities();
  else await resetLiveActivities();
}

export type CallActivityHandle = {
  setMuted: (muted: boolean) => void;
  end: () => void;
};

/**
 * Starts the call activity (timer, mute / end buttons that deep link back to
 * the call screen). Returns null when Live Activities are off or unsupported.
 */
export function startCallActivity(input: {
  room: string;
  title: string;
  subtitle?: string;
  kind: 'video' | 'voice';
}): CallActivityHandle | null {
  if (!available()) return null;
  let state = { muted: false, kind: input.kind };
  const id = Bridge.startCallActivity(
    { room: input.room, title: input.title, subtitle: input.subtitle ?? '', startedAt: Date.now() / 1000 },
    state,
  );
  if (!id) return null;
  return {
    setMuted: (muted) => {
      if (muted === state.muted) return;
      state = { ...state, muted };
      void Bridge.updateCallActivity(id, state);
    },
    end: () => void Bridge.endActivity(id, true),
  };
}

/**
 * ActivityKit ids of the urgent activities this launch started, keyed to
 * their conversation. Activities the backend pushed are updated and ended by
 * the backend itself.
 */
const activityRefs = new Map<string, string>();

/** An SLA this close (or already breached) puts the conversation on the Lock Screen. */
export const URGENT_WITHIN_MS = 15 * 60_000;

/**
 * Keeps the app-started urgent activity in step with the snapshot: the most
 * pressing conversation assigned to me is shown, the ones that left the list
 * (answered, resolved, reassigned) are ended. A conversation that already has
 * an activity (for example one the backend pushed) is not shown twice.
 */
export function syncUrgentActivity(sla: SnapshotSLA[] | null, now = Date.now()) {
  if (!available()) return;
  const pick = (sla ?? []).find((s) => s.breached || Date.parse(s.due_at) - now <= URGENT_WITHIN_MS) ?? null;
  for (const [id, uuid] of activityRefs) {
    if (uuid === pick?.uuid) continue;
    activityRefs.delete(id);
    void Bridge.endActivity(id, false);
  }
  if (!pick) return;
  const state = {
    contact: pick.contact,
    channel: pick.channel,
    subject: pick.subject,
    preview: pick.subject,
    dueAt: Date.parse(pick.due_at) / 1000,
    breached: pick.breached,
  };
  // Stale a quarter of an hour after the target: the countdown means nothing then.
  const staleAt = Math.max(Date.parse(pick.due_at), now) / 1000 + 15 * 60;
  const running = Bridge.listActivities().find((a) => a.type === 'urgent_conversation' && a.ref === pick.uuid);
  if (running) {
    if (activityRefs.has(running.id)) void Bridge.updateUrgentActivity(running.id, state, staleAt);
    return;
  }
  const id = Bridge.startUrgentActivity({ conversation: pick.uuid }, state, staleAt);
  if (id) activityRefs.set(id, pick.uuid);
}
