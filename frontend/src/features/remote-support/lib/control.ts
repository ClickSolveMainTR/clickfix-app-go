/**
 * Remote support control events, protocol version 1 (CF-237). The agent
 * console encodes them, the ClickFix Destek sidecar (CF-238) decodes and
 * applies them behind the control gate. The format is documented in
 * docs/design/masaustu.md ("Kontrol olayı biçimi"); keep both in step.
 *
 * Dependency free so it can be unit tested with node --test.
 */

/** Major version. A receiver drops events whose `v` it does not know. */
export const CONTROL_PROTOCOL_VERSION = 1;

/** LiveKit data topic of control events (reliable, to the customer only). */
export const CONTROL_TOPIC = "clickfix.rs.control.v1";

/** Largest encoded event a receiver accepts. */
export const CONTROL_MAX_BYTES = 1024;

/** Pointer moves are coalesced to at most this many per second. */
export const POINTER_MOVE_MAX_PER_SECOND = 60;

export type Modifiers = {
  alt: boolean;
  ctrl: boolean;
  meta: boolean;
  shift: boolean;
};

/** 0 primary (left), 1 middle, 2 secondary (right): DOM `button` values. */
export type PointerButton = 0 | 1 | 2;

type Base = {
  v: typeof CONTROL_PROTOCOL_VERSION;
  /** Monotonic per agent connection, starts at 1. */
  seq: number;
  /** Agent clock, ms since the Unix epoch (diagnostics only). */
  t: number;
};

export type ControlEvent =
  | (Base & { type: "pointer.move"; x: number; y: number })
  | (Base & {
      type: "pointer.down" | "pointer.up";
      x: number;
      y: number;
      button: PointerButton;
    })
  | (Base & { type: "wheel"; x: number; y: number; dx: number; dy: number })
  | (Base & {
      type: "key.down" | "key.up";
      code: string;
      key: string;
      mods: Modifiers;
      repeat: boolean;
    })
  | (Base & { type: "release" });

export type ControlEventType = ControlEvent["type"];

/** Distributes Omit over the union so each variant keeps its own fields. */
type DistributiveOmit<T, K extends PropertyKey> = T extends unknown
  ? Omit<T, K>
  : never;

export type ControlEventInput = DistributiveOmit<ControlEvent, keyof Base>;

const TYPES: ReadonlySet<string> = new Set([
  "pointer.move",
  "pointer.down",
  "pointer.up",
  "wheel",
  "key.down",
  "key.up",
  "release",
]);

/** Wheel deltas are pixels, clamped to this magnitude. */
export const WHEEL_MAX = 1200;

const round4 = (n: number) => Math.round(n * 10_000) / 10_000;
const clamp = (n: number, min: number, max: number) =>
  Math.min(max, Math.max(min, n));

export type Box = { left: number; top: number; width: number; height: number };

/**
 * Where a `videoW × videoH` frame is drawn inside an element of
 * `boxW × boxH` with `object-fit: contain` (letterboxed, centred).
 */
export function contentBox(
  boxW: number,
  boxH: number,
  videoW: number,
  videoH: number,
): Box | null {
  if (!(boxW > 0 && boxH > 0 && videoW > 0 && videoH > 0)) return null;
  const scale = Math.min(boxW / videoW, boxH / videoH);
  const width = videoW * scale;
  const height = videoH * scale;
  return {
    left: (boxW - width) / 2,
    top: (boxH - height) / 2,
    width,
    height,
  };
}

/**
 * Normalizes a point relative to the video element (CSS px from its top
 * left corner) to the shared screen: 0..1 on both axes, 4 decimals.
 * Returns null on the letterbox bars (no event is sent there).
 */
export function normalizePoint(
  px: number,
  py: number,
  boxW: number,
  boxH: number,
  videoW: number,
  videoH: number,
): { x: number; y: number } | null {
  const box = contentBox(boxW, boxH, videoW, videoH);
  if (!box) return null;
  const x = (px - box.left) / box.width;
  const y = (py - box.top) / box.height;
  if (x < 0 || x > 1 || y < 0 || y > 1) return null;
  return { x: round4(x), y: round4(y) };
}

/** Maps a normalized point back to screen pixels (what the sidecar does). */
export function toScreen(
  x: number,
  y: number,
  screenW: number,
  screenH: number,
): { x: number; y: number } {
  return {
    x: Math.round(clamp(x, 0, 1) * (screenW - 1)),
    y: Math.round(clamp(y, 0, 1) * (screenH - 1)),
  };
}

/** DOM WheelEvent deltas → pixels (deltaMode 1 = lines, 2 = pages). */
export function wheelDelta(
  deltaX: number,
  deltaY: number,
  deltaMode: number,
): { dx: number; dy: number } {
  const unit = deltaMode === 1 ? 16 : deltaMode === 2 ? 800 : 1;
  return {
    dx: Math.round(clamp(deltaX * unit, -WHEEL_MAX, WHEEL_MAX)),
    dy: Math.round(clamp(deltaY * unit, -WHEEL_MAX, WHEEL_MAX)),
  };
}

/**
 * System shortcuts that are never sent (and that the sidecar refuses too):
 * secure attention, lock, task manager, log out, force quit.
 */
export function isBlockedCombo(code: string, mods: Modifiers): boolean {
  if (mods.ctrl && mods.alt && (code === "Delete" || code === "Backspace"))
    return true;
  if (mods.ctrl && mods.shift && code === "Escape") return true;
  if (mods.meta && code === "KeyL") return true;
  if (mods.meta && mods.ctrl && code === "KeyQ") return true;
  if (mods.meta && mods.alt && code === "Escape") return true;
  if (mods.meta && mods.shift && code === "KeyQ") return true;
  return false;
}

/** Stamps events with the protocol version, a sequence and the clock. */
export function createSequencer(now: () => number = Date.now) {
  let seq = 0;
  return (event: ControlEventInput): ControlEvent =>
    ({
      v: CONTROL_PROTOCOL_VERSION,
      seq: ++seq,
      t: now(),
      ...event,
    }) as ControlEvent;
}

export function encodeEvent(event: ControlEvent): Uint8Array {
  return new TextEncoder().encode(JSON.stringify(event));
}

const isUnit = (n: unknown): n is number =>
  typeof n === "number" && Number.isFinite(n) && n >= 0 && n <= 1;
const isInt = (n: unknown): n is number =>
  typeof n === "number" && Number.isInteger(n);
const isBool = (b: unknown): b is boolean => typeof b === "boolean";
const isShortString = (s: unknown, max: number): s is string =>
  typeof s === "string" && s.length > 0 && s.length <= max;

/**
 * Parses and validates one event exactly as a receiver must: unknown
 * versions, unknown types, out-of-range values and oversized payloads are
 * dropped (null). Unknown extra fields are ignored.
 */
export function decodeEvent(data: Uint8Array): ControlEvent | null {
  if (data.byteLength === 0 || data.byteLength > CONTROL_MAX_BYTES) return null;
  let raw: unknown;
  try {
    raw = JSON.parse(new TextDecoder().decode(data));
  } catch {
    return null;
  }
  if (!raw || typeof raw !== "object") return null;
  const e = raw as Record<string, unknown>;
  if (e.v !== CONTROL_PROTOCOL_VERSION) return null;
  if (!isInt(e.seq) || (e.seq as number) < 1 || !isInt(e.t)) return null;
  if (typeof e.type !== "string" || !TYPES.has(e.type)) return null;
  const base = { v: CONTROL_PROTOCOL_VERSION, seq: e.seq, t: e.t } as const;
  switch (e.type) {
    case "pointer.move":
      if (!isUnit(e.x) || !isUnit(e.y)) return null;
      return { ...base, type: e.type, x: e.x, y: e.y };
    case "pointer.down":
    case "pointer.up":
      if (!isUnit(e.x) || !isUnit(e.y)) return null;
      if (e.button !== 0 && e.button !== 1 && e.button !== 2) return null;
      return { ...base, type: e.type, x: e.x, y: e.y, button: e.button };
    case "wheel":
      if (!isUnit(e.x) || !isUnit(e.y) || !isInt(e.dx) || !isInt(e.dy))
        return null;
      if (Math.abs(e.dx) > WHEEL_MAX || Math.abs(e.dy) > WHEEL_MAX) return null;
      return { ...base, type: e.type, x: e.x, y: e.y, dx: e.dx, dy: e.dy };
    case "key.down":
    case "key.up": {
      const m = e.mods as Record<string, unknown> | undefined;
      if (!isShortString(e.code, 32) || typeof e.key !== "string") return null;
      if (e.key.length > 32 || !m || typeof m !== "object") return null;
      if (
        !isBool(m.alt) ||
        !isBool(m.ctrl) ||
        !isBool(m.meta) ||
        !isBool(m.shift)
      )
        return null;
      const mods = { alt: m.alt, ctrl: m.ctrl, meta: m.meta, shift: m.shift };
      if (isBlockedCombo(e.code, mods)) return null;
      return {
        ...base,
        type: e.type,
        code: e.code,
        key: e.key,
        mods,
        repeat: e.repeat === true,
      };
    }
    case "release":
      return { ...base, type: "release" };
  }
  return null;
}
