import Constants from 'expo-constants';
import * as Device from 'expo-device';
import * as Notifications from 'expo-notifications';
import { AndroidImportance } from 'expo-notifications';
import { Platform } from 'react-native';
import { create } from 'zustand';

import { api, unwrap } from '@/core/api/client';
import { APP_VERSION, PLATFORM } from '@/core/config';
import { i18n } from '@/core/i18n';

/**
 * Push notifications (Expo push service). After sign-in the phone registers
 * its Expo token for the user's in-app notifications (`/v1/push-devices`:
 * mentions, assignments, everything the panel's bell shows). Sign-out removes
 * it first.
 */

// Shown while the app is open as well (the push is about something new).
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowBanner: true,
    shouldShowList: true,
    shouldPlaySound: true,
    shouldSetBadge: true,
  }),
});

export type PushPermission = 'granted' | 'denied' | 'undetermined' | 'unavailable';

/** The OS permission as the settings card shows it. */
export const usePushPermission = create<{ status: PushPermission; set: (s: PushPermission) => void }>((set) => ({
  status: 'undetermined',
  set: (status) => set({ status }),
}));

/** EAS project id (`eas init` writes it to app.json `extra.eas.projectId`). */
const projectId: string | undefined =
  (Constants.expoConfig?.extra as { eas?: { projectId?: string } } | undefined)?.eas?.projectId ??
  Constants.easConfig?.projectId;

let token: string | null = null;

/** Reads (and with `ask`, requests) the permission; Android gets its channel first. */
export async function refreshPermission(ask: boolean): Promise<PushPermission> {
  let status: PushPermission;
  if (!Device.isDevice || !projectId) {
    status = 'unavailable';
  } else {
    if (Platform.OS === 'android') {
      await Notifications.setNotificationChannelAsync('default', {
        name: i18n.t('settings:push_channel'),
        importance: AndroidImportance.HIGH,
      });
    }
    let p = await Notifications.getPermissionsAsync();
    if (ask && p.status === 'undetermined' && p.canAskAgain) p = await Notifications.requestPermissionsAsync();
    status = p.granted ? 'granted' : p.status === 'denied' ? 'denied' : 'undetermined';
  }
  usePushPermission.getState().set(status);
  return status;
}

/** The phone's Expo token, asking for the permission once; null when there is none. */
async function pushToken(): Promise<string | null> {
  if (token) return token;
  if ((await refreshPermission(true)) !== 'granted') return null;
  token = (await Notifications.getExpoPushTokenAsync({ projectId })).data;
  return token;
}

const device = () => ({
  platform: PLATFORM as 'ios' | 'android',
  locale: i18n.language,
  app_version: APP_VERSION,
});

/** Mirror the user's in-app notifications to this phone. Best effort. */
export async function registerPush() {
  try {
    const t = await pushToken();
    if (!t) return;
    await unwrap(
      api.POST('/v1/push-devices', {
        body: { token: t, ...device(), device_name: Device.deviceName ?? Device.modelName ?? '' },
      }),
    );
  } catch {
    // No network or push service: the next app start tries again.
  }
}

/** Before sign-out (while the access token is still valid). */
export async function unregisterPush() {
  if (!token) return;
  await unwrap(api.DELETE('/v1/push-devices/{token}', { params: { path: { token } } })).catch(() => undefined);
}
