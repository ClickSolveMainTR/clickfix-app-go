"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { Replayer } from "rrweb";

import type { ReplayEvent } from "../services/replay.service";

/** rrweb event types the stage cares about (`EventType` in @rrweb/types). */
export const EVENT_FULL_SNAPSHOT = 2;
export const EVENT_META = 4;

/**
 * Builds an rrweb Replayer inside `root`. rrweb is imported on demand so the
 * inbox bundle does not carry it. The replay iframe is rrweb's sandboxed one
 * (no scripts); captured data is already masked by the widget and re-masked
 * by the API, and the player never unmasks anything.
 */
export async function createReplayer(
  root: HTMLElement,
  events: ReplayEvent[],
  live: boolean,
): Promise<Replayer> {
  const { Replayer } = await import("rrweb");
  return new Replayer(events as ConstructorParameters<typeof Replayer>[0], {
    root,
    liveMode: live,
    showWarning: false,
    mouseTail: false,
    skipInactive: !live,
    triggerFocus: false,
    // Structural replay: external CSS is never captured, so give the
    // skeleton a readable baseline.
    insertStyleRules: [
      "body { font-family: system-ui, sans-serif; color: #111; background: #fff; }",
      "img, video, canvas, iframe { background: #e5e7eb; }",
    ],
  });
}

/**
 * Scales the recorded viewport down to the available width (and an optional
 * max height). Returns the frame ref, the computed scale and a `bind` that
 * hooks a replayer's resize events.
 */
export function useFittedViewport(maxHeight: number) {
  const frame = useRef<HTMLDivElement | null>(null);
  const [size, setSize] = useState<{ width: number; height: number } | null>(
    null,
  );
  const [avail, setAvail] = useState(0);
  const replayer = useRef<Replayer | null>(null);

  useEffect(() => {
    const el = frame.current;
    if (!el) return;
    const ro = new ResizeObserver(([entry]) =>
      setAvail(entry?.contentRect.width ?? 0),
    );
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const scale =
    size && avail > 0
      ? Math.min(1, avail / size.width, maxHeight / size.height)
      : 1;

  useEffect(() => {
    const wrapper = replayer.current?.wrapper;
    if (!wrapper) return;
    wrapper.style.transformOrigin = "top left";
    wrapper.style.transform = `scale(${scale})`;
  }, [scale, size]);

  const bind = useCallback((r: Replayer | null) => {
    replayer.current = r;
    if (!r) {
      setSize(null);
      return;
    }
    r.on("resize", (d) => {
      const dim = d as { width: number; height: number };
      if (dim.width > 0 && dim.height > 0)
        setSize({ width: dim.width, height: dim.height });
    });
  }, []);

  return { frame, size, scale, bind };
}

export function formatClock(ms: number) {
  const total = Math.max(0, Math.floor(ms / 1000));
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}
