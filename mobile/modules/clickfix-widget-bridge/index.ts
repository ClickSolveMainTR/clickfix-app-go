// CF-152: JS API of the widget bridge. Every function is a safe no-op when the
// native module is missing (Expo Go, web, tests) or reports unavailable
// (Android, or an iOS build without the App Group entitlement).

import { Platform } from 'react-native';
import { requireOptionalNativeModule, type EventSubscription } from 'expo';

import type {
  ActivityPushTokenEvent,
  CallActivityAttributes,
  CallActivityState,
  ClickFixWidgetBridgeNativeModule,
  LiveActivityInfo,
  PresenceOverride,
  PushToStartTokenEvent,
  SpotlightItem,
  UrgentActivityAttributes,
  UrgentActivityState,
  WidgetCredential,
} from './src/ClickFixWidgetBridge.types';

export type {
  ActivityPushTokenEvent,
  CallActivityAttributes,
  CallActivityState,
  ClickFixWidgetBridgeEvents,
  ClickFixWidgetBridgeNativeModule,
  LiveActivityInfo,
  LiveActivityType,
  PresenceOverride,
  PushToStartTokenEvent,
  SpotlightItem,
  UrgentActivityAttributes,
  UrgentActivityState,
  WidgetCredential,
} from './src/ClickFixWidgetBridge.types';
export type { EventSubscription } from 'expo';

const native = requireOptionalNativeModule<ClickFixWidgetBridgeNativeModule>('ClickFixWidgetBridge');

let available: boolean | null = null;

function bridge(): ClickFixWidgetBridgeNativeModule | null {
  if (!native) return null;
  if (available === null) {
    try {
      available = native.isAvailable();
    } catch {
      available = false;
    }
  }
  return available ? native : null;
}

/** True when the iOS bridge can reach the App Group. */
export function isAvailable(): boolean {
  return bridge() !== null;
}

/** Writes the snapshot v1 JSON, clears the presence override, reloads widgets and controls. */
export function writeSnapshot(json: string): void {
  try {
    bridge()?.writeSnapshot(json);
  } catch {
    // Widgets keep the previous snapshot.
  }
}

/** Raw presence override JSON written by a widget / control / Siri, or null. */
export function readPresenceOverride(): string | null {
  try {
    return bridge()?.readPresenceOverride() ?? null;
  } catch {
    return null;
  }
}

/** Parsed presence override, or null when missing or malformed. */
export function readPresenceOverrideParsed(): PresenceOverride | null {
  const raw = readPresenceOverride();
  if (!raw) return null;
  try {
    const value: unknown = JSON.parse(raw);
    if (
      typeof value === 'object' &&
      value !== null &&
      'status' in value &&
      'at' in value &&
      typeof value.status === 'string' &&
      typeof value.at === 'string' &&
      ['online', 'away', 'busy', 'offline'].includes(value.status)
    ) {
      return { status: value.status as PresenceOverride['status'], at: value.at };
    }
  } catch {
    // Malformed: ignore.
  }
  return null;
}

/**
 * Stores the widget token in the shared Keychain (`null` deletes it).
 * Returns false when the write failed (e.g. missing keychain entitlement).
 */
export function setCredential(credential: WidgetCredential | string | null): boolean {
  const target = bridge();
  if (!target) return false;
  try {
    target.setCredential(
      credential === null || typeof credential === 'string' ? credential : JSON.stringify(credential),
    );
    return true;
  } catch {
    return false;
  }
}

/** Route left by an intent that opened the app (e.g. `/search?q=x`); read once. */
export function consumePendingRoute(): string | null {
  try {
    return bridge()?.consumePendingRoute() ?? null;
  } catch {
    return null;
  }
}

/** Reloads all widget timelines and (iOS 18) controls. */
export function reloadAll(): void {
  try {
    bridge()?.reloadAll();
  } catch {
    // Nothing to reload.
  }
}

/** Replaces the app's Spotlight entries with `items`. */
export async function indexSpotlight(items: SpotlightItem[]): Promise<void> {
  const target = bridge();
  if (!target) return;
  try {
    await target.indexSpotlight(items);
  } catch {
    // Spotlight is best effort.
  }
}

/** Removes every Spotlight entry of the app (call on sign-out / org switch). */
export async function clearSpotlight(): Promise<void> {
  const target = bridge();
  if (!target) return;
  try {
    await target.clearSpotlight();
  } catch {
    // Spotlight is best effort.
  }
}

// MARK: Live Activities (iOS 16.2+; the native side no-ops / returns null below).
// Independent of the App Group: only the native module and iOS are required.

function activities(): ClickFixWidgetBridgeNativeModule | null {
  return Platform.OS === 'ios' && native ? native : null;
}

const noopSubscription: EventSubscription = { remove: () => {} };

/** True when the user allows Live Activities for ClickFix. */
export function liveActivitiesEnabled(): boolean {
  try {
    return activities()?.liveActivitiesEnabled() ?? false;
  } catch {
    return false;
  }
}

/** Starts a call activity (ending any other call activity). Returns its id. */
export function startCallActivity(
  attributes: CallActivityAttributes,
  state: CallActivityState,
): string | null {
  try {
    return activities()?.startCallActivity(attributes, state) ?? null;
  } catch {
    return null;
  }
}

export async function updateCallActivity(id: string, state: CallActivityState): Promise<void> {
  try {
    await activities()?.updateCallActivity(id, state);
  } catch {
    // Activity already gone.
  }
}

/** Starts an urgent conversation activity; `staleAt` in unix seconds. Returns its id. */
export function startUrgentActivity(
  attributes: UrgentActivityAttributes,
  state: UrgentActivityState,
  staleAt?: number,
): string | null {
  try {
    return activities()?.startUrgentActivity(attributes, state, staleAt ?? null) ?? null;
  } catch {
    return null;
  }
}

export async function updateUrgentActivity(
  id: string,
  state: UrgentActivityState,
  staleAt?: number,
): Promise<void> {
  try {
    await activities()?.updateUrgentActivity(id, state, staleAt ?? null);
  } catch {
    // Activity already gone.
  }
}

/** Ends a call or urgent activity; `immediate` removes it from the Lock Screen right away. */
export async function endActivity(id: string, immediate: boolean): Promise<void> {
  try {
    await activities()?.endActivity(id, immediate);
  } catch {
    // Activity already gone.
  }
}

/** Ends every ClickFix Live Activity (sign-out, org switch). */
export async function endAllActivities(): Promise<void> {
  try {
    await activities()?.endAllActivities();
  } catch {
    // Nothing running.
  }
}

/** Running activities, including ones started by push-to-start. */
export function listActivities(): LiveActivityInfo[] {
  try {
    return activities()?.listActivities() ?? [];
  } catch {
    return [];
  }
}

/**
 * Starts token observation (call after sign-in, after adding the listeners).
 * Later calls re-emit the last known tokens.
 */
export function startObservingTokens(): void {
  try {
    activities()?.startObservingTokens();
  } catch {
    // Live Activities unavailable.
  }
}

/** Per-activity APNs update tokens (activities started here or by push). */
export function addPushTokenListener(
  listener: (event: ActivityPushTokenEvent) => void,
): EventSubscription {
  const target = activities();
  if (!target) return noopSubscription;
  try {
    return target.addListener('onActivityPushToken', listener);
  } catch {
    return noopSubscription;
  }
}

/** Push-to-start token for urgent conversation activities (iOS 17.2+). */
export function addPushToStartTokenListener(
  listener: (event: PushToStartTokenEvent) => void,
): EventSubscription {
  const target = activities();
  if (!target) return noopSubscription;
  try {
    return target.addListener('onPushToStartToken', listener);
  } catch {
    return noopSubscription;
  }
}
