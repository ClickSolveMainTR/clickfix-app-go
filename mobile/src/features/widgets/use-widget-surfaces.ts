import * as Bridge from 'clickfix-widget-bridge';
import * as Notifications from 'expo-notifications';
import { type Href, router } from 'expo-router';
import { useEffect } from 'react';
import { AppState, Platform } from 'react-native';

import { useSession } from '@/core/auth/session';
import { useChannel } from '@/features/realtime/client';

import { initLiveActivities } from './live-activities';
import { scheduleWidgetSync } from './sync';

/** Routes an intent may leave behind (iOS 17 opens the app without a URL). */
const PENDING_ROUTE = /^\/[A-Za-z0-9/_\-?=&%.]*$/;

function openPendingRoute() {
  if (Platform.OS !== 'ios') return;
  const route = Bridge.consumePendingRoute();
  if (route && PENDING_ROUTE.test(route)) router.push(route as Href);
}

/**
 * Mounted with the organization screens: refreshes the widgets on sign-in,
 * organization switch, foreground, realtime inbox / appointment events and
 * pushes received while open; registers the Live Activity layouts and opens
 * the screen a Siri / Shortcuts intent asked for (CF-152).
 */
export function useWidgetSurfaces() {
  const org = useSession((s) => s.organization);
  const orgUuid = org?.uuid ?? null;

  useEffect(() => {
    scheduleWidgetSync(true);
    initLiveActivities();
    openPendingRoute();
  }, [org?.slug, org?.permissions]);

  useEffect(() => {
    const appState = AppState.addEventListener('change', (state) => {
      if (state === 'active') {
        openPendingRoute();
        scheduleWidgetSync(true);
      } else if (state === 'background') {
        // Leave the freshest numbers behind for the Home Screen.
        scheduleWidgetSync(true);
      }
    });
    const push = Notifications.addNotificationReceivedListener(() => scheduleWidgetSync());
    return () => {
      appState.remove();
      push.remove();
    };
  }, []);

  useChannel(orgUuid ? `inbox:${orgUuid}` : null, () => scheduleWidgetSync());
  useChannel(orgUuid ? `org:${orgUuid}` : null, () => scheduleWidgetSync());
}
