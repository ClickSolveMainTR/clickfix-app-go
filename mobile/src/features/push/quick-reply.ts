import * as Notifications from 'expo-notifications';
import * as TaskManager from 'expo-task-manager';
import { Platform } from 'react-native';

import { api, orgHeader, unwrap } from '@/core/api/client';
import type { Me } from '@/core/auth/api';
import { can, P, type Permission } from '@/core/auth/permissions';
import { getTokens, loadTokens } from '@/core/auth/tokens';
import { clientId } from '@/features/inbox/helpers';
import { flushOutbox, queueMessage } from '@/features/inbox/outbox';
import { i18n } from '@/core/i18n';
import { readJSON, writeJSON } from '@/core/storage';

import {
  ASSIGN_ACTION,
  notificationOf,
  OPEN_ACTION,
  PUSH_CATEGORIES,
  READ_ACTION,
  REPLY_ACTION,
  runsInBackground,
  type PushAction,
} from './actions-core';
import { pushTarget } from './links';

export { CONVERSATION_CATEGORY, OPEN_ACTION, REPLY_ACTION } from './actions-core';

/**
 * Actions on a notification (CF-81, CF-157). The backend sets the category
 * (`notifications/usecase/expo.go`): `conversation` offers "Reply" (text
 * input), "Mark read" and "Open"; `conversation_assign` (nobody assigned)
 * adds "Assign to me"; everything else is `generic`: "Mark read", "Open".
 * A reply goes through the inbox send queue with the push's organization,
 * so it is sent even when another organization is selected, and waits for
 * the network (or the next launch) when there is none. Assigning and
 * marking read call the API with the push's organization.
 *
 * Delivery of the action to JavaScript: while the app runs (foreground or
 * background) through the response listener; on Android also when the app
 * was killed, through the background notification task below. iOS starts
 * the app in the background for a non-foreground action and hands the
 * response to the listener.
 */

export const PUSH_TASK = 'clickfix-push-actions';

const HANDLED_KEY = 'push.replied';
const ME_KEY = 'session.me';

function button(action: PushAction): Notifications.NotificationAction {
  const options = { opensAppToForeground: !runsInBackground(action) };
  switch (action) {
    case REPLY_ACTION:
      return {
        identifier: action,
        buttonTitle: i18n.t('settings:push_reply'),
        textInput: {
          submitButtonTitle: i18n.t('settings:push_reply_send'),
          placeholder: i18n.t('settings:push_reply_placeholder'),
        },
        options,
      };
    case ASSIGN_ACTION:
      return { identifier: action, buttonTitle: i18n.t('settings:push_assign'), options };
    case READ_ACTION:
      return { identifier: action, buttonTitle: i18n.t('settings:push_read'), options };
    default:
      return { identifier: OPEN_ACTION, buttonTitle: i18n.t('settings:push_open'), options };
  }
}

/** Registers the categories with the app language's button titles (again after a language change). */
export async function registerPushCategories() {
  try {
    for (const [category, actions] of Object.entries(PUSH_CATEGORIES)) {
      await Notifications.setNotificationCategoryAsync(category, actions.map(button));
    }
  } catch {
    // Not supported here (web, Expo Go without push): the plain notification still works.
  }
}

/** Remembers handled responses: the listener and the Android task may both see the same one. */
function firstTime(id: string) {
  const seen = readJSON<string[]>(HANDLED_KEY) ?? [];
  if (seen.includes(id)) return false;
  writeJSON(HANDLED_KEY, [...seen.slice(-49), id]);
  return true;
}

/**
 * Sends the text typed on a notification as a reply in its conversation.
 * Returns false when the response is not a usable quick reply.
 */
export async function handleQuickReply(response: Notifications.NotificationResponse): Promise<boolean> {
  if (response.actionIdentifier !== REPLY_ACTION) return false;
  const text = response.userText?.trim();
  const request = response.notification.request;
  const target = pushTarget(request.content.data);
  if (!text || !target.organization || !target.conversation) return false;
  // Signed out, or no longer allowed to reply there: nothing is sent.
  const me = memberFor(target.organization, P.conversationsReply);
  if (!me) return false;
  // Marked only once it is really sent, so an early refusal does not swallow a later delivery.
  if (!firstTime(`${request.identifier}:${REPLY_ACTION}`)) return false;

  queueMessage({
    org: target.organization,
    conversation: target.conversation,
    client_id: clientId(),
    content: text,
    private: false,
    attachments: [],
    sender: { uuid: me.user.uuid, name: [me.user.name, me.user.surname].filter(Boolean).join(' ') },
    created_at: new Date().toISOString(),
  });
  if (await ensureTokens()) await flushOutbox();
  await Notifications.dismissNotificationAsync(request.identifier).catch(() => undefined);
  return true;
}

/** The signed-in member of the push's organization holding perm, or null. */
function memberFor(organization: string | null, perm: Permission): Me | null {
  const me = readJSON<Me>(ME_KEY);
  const membership = me?.organizations?.find((o) => o.slug === organization);
  return me && membership && can(membership, perm) ? me : null;
}

async function ensureTokens() {
  // The tokens may not be loaded yet (the app was started for this action).
  if (!getTokens()) await loadTokens();
  return Boolean(getTokens());
}

/** "Mark read": the in-app notification the push mirrors. */
export async function handleMarkRead(response: Notifications.NotificationResponse): Promise<boolean> {
  if (response.actionIdentifier !== READ_ACTION) return false;
  const request = response.notification.request;
  const id = notificationOf(request.content.data);
  if (!id || !readJSON<Me>(ME_KEY) || !(await ensureTokens())) return false;
  if (!firstTime(`${request.identifier}:${READ_ACTION}`)) return false;
  await unwrap(api.POST('/v1/notifications/{uuid}/read', { params: { path: { uuid: id } } })).catch(() => undefined);
  await Notifications.dismissNotificationAsync(request.identifier).catch(() => undefined);
  return true;
}

/** "Assign to me": the conversation in the push's organization (needs the assign permission). */
export async function handleAssignToMe(response: Notifications.NotificationResponse): Promise<boolean> {
  if (response.actionIdentifier !== ASSIGN_ACTION) return false;
  const request = response.notification.request;
  const target = pushTarget(request.content.data);
  if (!target.organization || !target.conversation) return false;
  const me = memberFor(target.organization, P.conversationsAssign);
  if (!me || !(await ensureTokens())) return false;
  if (!firstTime(`${request.identifier}:${ASSIGN_ACTION}`)) return false;
  try {
    await unwrap(
      api.PATCH('/v1/tenant/conversations/{uuid}', {
        params: {
          header: { ...orgHeader(), 'X-Organization': target.organization },
          path: { uuid: target.conversation },
        },
        body: { assignee: me.user.uuid },
      }),
    );
  } catch {
    // Taken meanwhile or no longer allowed: the notification stays so it can be opened.
    return false;
  }
  const id = notificationOf(request.content.data);
  if (id)
    await unwrap(api.POST('/v1/notifications/{uuid}/read', { params: { path: { uuid: id } } })).catch(() => undefined);
  await Notifications.dismissNotificationAsync(request.identifier).catch(() => undefined);
  return true;
}

/** Runs a background action (reply, mark read, assign); false for anything else. */
export async function handleBackgroundAction(response: Notifications.NotificationResponse): Promise<boolean> {
  switch (response.actionIdentifier) {
    case REPLY_ACTION:
      return handleQuickReply(response);
    case READ_ACTION:
      return handleMarkRead(response);
    case ASSIGN_ACTION:
      return handleAssignToMe(response);
  }
  return false;
}

// Android delivers action taps to a killed app only through a background task; it must be
// defined at module scope so the headless JS run finds it.
if (Platform.OS === 'android') {
  TaskManager.defineTask<Notifications.NotificationTaskPayload>(PUSH_TASK, async ({ data }) => {
    if (data && typeof data === 'object' && 'actionIdentifier' in data) {
      await handleBackgroundAction(data as Notifications.NotificationResponse);
    }
  });
}

/** Registers the background task once (Android). */
export async function registerPushTask() {
  if (Platform.OS !== 'android') return;
  try {
    if (!(await TaskManager.isTaskRegisteredAsync(PUSH_TASK))) await Notifications.registerTaskAsync(PUSH_TASK);
  } catch {
    // No task manager in this build (Expo Go): replies work while the app runs.
  }
}
