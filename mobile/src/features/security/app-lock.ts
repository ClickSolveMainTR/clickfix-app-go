import * as LocalAuthentication from 'expo-local-authentication';
import { AppState } from 'react-native';
import { create } from 'zustand';

import { i18n } from '@/core/i18n';
import { readJSON, writeJSON } from '@/core/storage';

import { type LockSettings, type LockTimeout, parseLockSettings, shouldLock } from './lock-core';

/**
 * Biometric app lock (CF-82): with the lock on, the app asks for Face ID /
 * Touch ID / fingerprint (or the device passcode) on launch and when it
 * comes back after the chosen time away. The session itself stays signed
 * in; the lock only covers the screen. Stored on the phone, per device.
 */

const KEY = 'security.lock';

type AppLock = LockSettings & { locked: boolean };

const initial = parseLockSettings(readJSON(KEY));
export const useAppLock = create<AppLock>(() => ({ ...initial, locked: initial.enabled }));

function persist() {
  const { enabled, timeoutMin } = useAppLock.getState();
  writeJSON(KEY, { enabled, timeoutMin });
}

/** Whether this phone can do a biometric check (hardware present and something enrolled). */
export async function biometricsAvailable(): Promise<boolean> {
  try {
    return (await LocalAuthentication.hasHardwareAsync()) && (await LocalAuthentication.isEnrolledAsync());
  } catch {
    return false;
  }
}

/** One biometric (or passcode) check; true when it passed. */
export async function authenticate(): Promise<boolean> {
  try {
    const r = await LocalAuthentication.authenticateAsync({
      promptMessage: i18n.t('security:unlock_prompt'),
      cancelLabel: i18n.t('common:cancel'),
      disableDeviceFallback: false,
    });
    return r.success;
  } catch {
    return false;
  }
}

/** Turns the lock on after a successful check (so nobody locks themselves out), or off. */
export async function setLockEnabled(enabled: boolean): Promise<boolean> {
  if (enabled) {
    if (!(await biometricsAvailable())) return false;
    if (!(await authenticate())) return false;
  }
  useAppLock.setState({ enabled, locked: false });
  persist();
  return true;
}

export function setLockTimeout(timeoutMin: LockTimeout) {
  useAppLock.setState({ timeoutMin });
  persist();
}

export async function unlock() {
  if (await authenticate()) useAppLock.setState({ locked: false });
}

/** Sign-out: nothing left to protect until the next sign-in. */
export function releaseLock() {
  useAppLock.setState({ locked: false });
}

let started = false;
let backgroundAt: number | null = null;

/** Locks when the app returns after the timeout. */
export function startAppLock() {
  if (started) return;
  started = true;
  AppState.addEventListener('change', (s) => {
    if (s === 'background') {
      backgroundAt = Date.now();
    } else if (s === 'active' && backgroundAt != null) {
      if (shouldLock(useAppLock.getState(), backgroundAt, Date.now())) useAppLock.setState({ locked: true });
      backgroundAt = null;
    }
  });
}
