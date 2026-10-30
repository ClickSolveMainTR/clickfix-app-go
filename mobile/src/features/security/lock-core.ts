/**
 * App lock rules (CF-82), free of app imports so they run under `node --test`.
 */

/** Minutes in the background before the lock asks again; 0 = every time the app leaves the screen. */
export const LOCK_TIMEOUTS = [0, 1, 5, 15] as const;
export type LockTimeout = (typeof LOCK_TIMEOUTS)[number];

export type LockSettings = { enabled: boolean; timeoutMin: LockTimeout };
export const DEFAULT_LOCK: LockSettings = { enabled: false, timeoutMin: 1 };

/** Reads stored settings, falling back to the defaults for anything unknown. */
export function parseLockSettings(raw: unknown): LockSettings {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Partial<LockSettings>;
  const timeoutMin = (LOCK_TIMEOUTS as readonly number[]).includes(Number(r.timeoutMin))
    ? (Number(r.timeoutMin) as LockTimeout)
    : DEFAULT_LOCK.timeoutMin;
  return { enabled: r.enabled === true, timeoutMin };
}

/**
 * Whether coming back to the foreground needs the biometric check:
 * the lock is on and the app was away at least the timeout. An unknown
 * background time (a cold start) always locks.
 */
export function shouldLock(s: LockSettings, backgroundAt: number | null, now: number): boolean {
  if (!s.enabled) return false;
  if (backgroundAt == null) return true;
  return now - backgroundAt >= s.timeoutMin * 60_000;
}
