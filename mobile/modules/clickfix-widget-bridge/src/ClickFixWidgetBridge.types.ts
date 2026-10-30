import type { EventSubscription } from 'expo';

/** A Spotlight entry; `url` is a `clickfix://` deep link and its identity. */
export type SpotlightItem = {
  id: string;
  title: string;
  subtitle: string;
  url: string;
  keywords?: string[];
};

/** Value of `clickfix.widget.presenceOverride` (JSON). */
export type PresenceOverride = {
  status: 'online' | 'away' | 'busy' | 'offline';
  at: string;
};

/** Widget token stored in the shared Keychain (docs/mobile/widgets.md). */
export type WidgetCredential = {
  token: string;
  expires_at: string;
  api_url: string;
  org: string;
};

// Live Activities. Field names match the Swift ActivityAttributes the
// backend pushes; times are unix SECONDS.

export type LiveActivityType = 'call' | 'urgent_conversation';

export type CallActivityAttributes = {
  /** Video room uuid (deep link clickfix:///video/<room>). */
  room: string;
  title: string;
  subtitle: string;
  /** Unix seconds. */
  startedAt: number;
};

export type CallActivityState = {
  muted: boolean;
  kind: 'video' | 'voice';
};

export type UrgentActivityAttributes = {
  /** Conversation uuid (deep link clickfix:///conversation/<uuid>). */
  conversation: string;
};

export type UrgentActivityState = {
  contact: string;
  channel: string;
  subject: string;
  preview: string;
  /** SLA target, unix seconds. */
  dueAt: number;
  breached: boolean;
};

export type LiveActivityInfo = {
  id: string;
  type: LiveActivityType;
  /** Room uuid (call) or conversation uuid (urgent_conversation). */
  ref: string;
};

export type ActivityPushTokenEvent = LiveActivityInfo & {
  /** APNs update token, lowercase hex. */
  token: string;
};

export type PushToStartTokenEvent = {
  type: 'urgent_conversation';
  /** APNs push-to-start token, lowercase hex. */
  token: string;
};

export type ClickFixWidgetBridgeEvents = {
  onActivityPushToken: (event: ActivityPushTokenEvent) => void;
  onPushToStartToken: (event: PushToStartTokenEvent) => void;
};

/** Native surface of the `ClickFixWidgetBridge` Expo module. */
export type ClickFixWidgetBridgeNativeModule = {
  addListener<E extends keyof ClickFixWidgetBridgeEvents>(
    eventName: E,
    listener: ClickFixWidgetBridgeEvents[E],
  ): EventSubscription;
  isAvailable(): boolean;
  writeSnapshot(json: string): void;
  readPresenceOverride(): string | null;
  setCredential(json: string | null): void;
  consumePendingRoute(): string | null;
  reloadAll(): void;
  indexSpotlight(items: SpotlightItem[]): Promise<void>;
  clearSpotlight(): Promise<void>;

  liveActivitiesEnabled(): boolean;
  startCallActivity(attributes: CallActivityAttributes, state: CallActivityState): string | null;
  updateCallActivity(id: string, state: CallActivityState): Promise<void>;
  startUrgentActivity(
    attributes: UrgentActivityAttributes,
    state: UrgentActivityState,
    staleAt: number | null,
  ): string | null;
  updateUrgentActivity(id: string, state: UrgentActivityState, staleAt: number | null): Promise<void>;
  endActivity(id: string, immediate: boolean): Promise<void>;
  endAllActivities(): Promise<void>;
  listActivities(): LiveActivityInfo[];
  startObservingTokens(): void;
};
