import * as Notifications from 'expo-notifications';
import { type Href, router } from 'expo-router';
import { useEffect, useRef } from 'react';
import { AppState } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';
import { i18n } from '@/core/i18n';

import { pushTarget } from './links';
import { refreshPermission, registerPush, usePushPermission } from './push';
import { runsInBackground } from './actions-core';
import { handleBackgroundAction, OPEN_ACTION, registerPushCategories, registerPushTask } from './quick-reply';

/**
 * Registers the phone for pushes once per sign-in and app start, keeps the
 * app icon badge on the unread count, runs the notification buttons (reply,
 * mark read, assign to me), and opens what a tapped push is about (a conversation in
 * its own organization, the QR approval). Rendered inside the root stack.
 */
export function PushNotifications() {
  const status = useSession((s) => s.status);
  const response = Notifications.useLastNotificationResponse();
  // Turned on later in the system settings: register again.
  const granted = usePushPermission((s) => s.status === 'granted');
  const handled = useRef<string | null>(null);

  useEffect(() => {
    if (status !== 'signedIn') return;
    void registerPush();
    void registerPushTask();
  }, [status, granted]);

  // Button titles follow the app language.
  useEffect(() => {
    void registerPushCategories();
    i18n.on('languageChanged', registerPushCategories);
    return () => i18n.off('languageChanged', registerPushCategories);
  }, []);

  // The permission may change in the system settings while the app is away; the badge follows the unread count.
  useEffect(() => {
    void refreshPermission(false);
    if (status === 'signedIn') void syncBadge();
    const sub = AppState.addEventListener('change', (s) => {
      if (s !== 'active') return;
      void refreshPermission(false);
      if (useSession.getState().status === 'signedIn') void syncBadge();
    });
    return () => sub.remove();
  }, [status]);

  useEffect(() => {
    if (!response) return;
    const id = `${response.notification.request.identifier}:${response.actionIdentifier}`;
    if (handled.current === id) return;
    handled.current = id;
    Notifications.clearLastNotificationResponse();
    if (runsInBackground(response.actionIdentifier)) {
      void handleBackgroundAction(response);
      return;
    }
    if (
      response.actionIdentifier === Notifications.DEFAULT_ACTION_IDENTIFIER ||
      response.actionIdentifier === OPEN_ACTION
    )
      open(response.notification.request.content.data);
  }, [response]);

  return null;
}

/** The app icon badge: the member's unread in-app notifications (the push sets it too). */
async function syncBadge() {
  try {
    const { count } = await unwrap(api.GET('/v1/notifications/unread-count'));
    await Notifications.setBadgeCountAsync(count);
  } catch {
    // Offline or not allowed: keep the badge the last push set.
  }
}

function open(data: unknown) {
  const target = pushTarget(data);
  const session = useSession.getState();
  // Signed out (or no organization yet): the push only opens the app.
  if (!target.route || target.route === '/' || session.status !== 'signedIn') return;
  // A push about another organization switches to it first (if still a member), so it never opens "not found".
  if (target.organization && target.organization !== session.organization?.slug) {
    const member = session.me?.organizations?.some((o) => o.slug === target.organization && o.status === 'active');
    if (!member) return;
    session.selectOrganization(target.organization);
  }
  if (!useSession.getState().organization) return;
  router.push(target.route as Href);
}
