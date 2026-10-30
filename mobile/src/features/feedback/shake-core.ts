/**
 * Shake detection from accelerometer samples (in g). Pure, so it runs under
 * `node --test`. A shake is a few hard jolts close together; walking, a
 * bump or setting the phone down give one jolt and never count.
 */

export type Sample = { x: number; y: number; z: number };

export type ShakeOptions = {
  /** Jolt: the acceleration differs from gravity (1 g) by more than this. */
  threshold: number;
  /** Jolts needed inside `windowMs`. */
  hits: number;
  windowMs: number;
  /** After a shake, nothing counts for this long. */
  cooldownMs: number;
};

export const SHAKE_DEFAULTS: ShakeOptions = { threshold: 1.3, hits: 3, windowMs: 900, cooldownMs: 3000 };

/** Returns a detector; feed it every sample, it answers true on the sample that completes a shake. */
export function createShakeDetector(options: Partial<ShakeOptions> = {}) {
  const o = { ...SHAKE_DEFAULTS, ...options };
  let jolts: number[] = [];
  let quietUntil = 0;
  return (s: Sample, now: number): boolean => {
    if (now < quietUntil) return false;
    const g = Math.sqrt(s.x * s.x + s.y * s.y + s.z * s.z);
    if (Math.abs(g - 1) < o.threshold) return false;
    jolts = [...jolts.filter((t) => now - t <= o.windowMs), now];
    if (jolts.length < o.hits) return false;
    jolts = [];
    quietUntil = now + o.cooldownMs;
    return true;
  };
}
