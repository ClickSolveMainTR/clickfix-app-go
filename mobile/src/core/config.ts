import * as Application from 'expo-application';
import Constants, { ExecutionEnvironment } from 'expo-constants';
import { Platform } from 'react-native';

import { normalizeApiUrl } from '@/core/api/endpoints';

/**
 * Where a build without the EXPO_PUBLIC_* variables points: the local stack in
 * development, the production panel otherwise. A release build used to fall
 * back to `localhost`, which on a phone fails every call as "no connection".
 */
const DEFAULT_WEB_URL = __DEV__ ? 'http://localhost:3000' : 'https://getclickfix.com';

/**
 * Origin the app calls `/v1/*` under with a Bearer token: in production the
 * Next.js BFF passthrough (`https://<host>/api/mobile`), which forwards the
 * request to Go unchanged; locally the Go API itself also works.
 */
export const API_URL = normalizeApiUrl(process.env.EXPO_PUBLIC_API_URL, `${DEFAULT_WEB_URL}/api/mobile`);

/** Panel origin, for links that open the web (business page, settings). */
export const WEB_URL = (process.env.EXPO_PUBLIC_WEB_URL?.trim() || DEFAULT_WEB_URL).replace(/\/+$/, '');

/**
 * Optional Centrifugo socket override (`wss://rt.<host>/connection/websocket`).
 * Normally the API names it with the connection token (`CENTRIFUGO_WS_URL`).
 */
export const REALTIME_URL = process.env.EXPO_PUBLIC_REALTIME_URL?.trim() || null;

/**
 * App release (`X-App-Version`). In Expo Go the native version is Expo Go's
 * own, so the app.json version is used there.
 */
export const APP_VERSION =
  (Constants.executionEnvironment === ExecutionEnvironment.StoreClient ? Constants.expoConfig?.version : undefined) ??
  Application.nativeApplicationVersion ??
  '0.0.0';

export const PLATFORM = Platform.OS === 'android' ? 'android' : 'ios';
