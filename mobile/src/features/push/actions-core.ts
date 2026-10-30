/**
 * Push notification categories and their buttons (CF-81, CF-157). The
 * backend sets the category (`notifications/usecase/expo.go`); the app
 * registers the same ids with the buttons below. "open" also happens on a
 * plain tap, so it is last: Android shows at most three buttons. Kept free
 * of app imports so it runs under `node --test`.
 */
export const REPLY_ACTION = 'reply';
export const OPEN_ACTION = 'open';
export const READ_ACTION = 'read';
export const ASSIGN_ACTION = 'assign';

export type PushAction = typeof REPLY_ACTION | typeof OPEN_ACTION | typeof READ_ACTION | typeof ASSIGN_ACTION;

export const CONVERSATION_CATEGORY = 'conversation';
/** A conversation nobody is assigned to: "assign to me" first. */
export const CONVERSATION_ASSIGN_CATEGORY = 'conversation_assign';
export const GENERIC_CATEGORY = 'generic';

export const PUSH_CATEGORIES: Record<string, readonly PushAction[]> = {
  [CONVERSATION_CATEGORY]: [REPLY_ACTION, READ_ACTION, OPEN_ACTION],
  [CONVERSATION_ASSIGN_CATEGORY]: [ASSIGN_ACTION, REPLY_ACTION, READ_ACTION, OPEN_ACTION],
  [GENERIC_CATEGORY]: [READ_ACTION, OPEN_ACTION],
};

/** Actions that run in the background without opening the app. */
export function runsInBackground(action: string): boolean {
  return action === REPLY_ACTION || action === READ_ACTION || action === ASSIGN_ACTION;
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** The in-app notification a push mirrors (`data.notification_uuid`), or null. */
export function notificationOf(data: unknown): string | null {
  const d = (data && typeof data === 'object' ? data : {}) as Record<string, unknown>;
  return typeof d.notification_uuid === 'string' && UUID.test(d.notification_uuid) ? d.notification_uuid : null;
}
