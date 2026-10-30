"use client";

import { useSyncExternalStore } from "react";

/** First visible element carrying `data-guide="<target>"`, if any. */
export function findGuideTarget(target: string): HTMLElement | null {
  if (!target || typeof document === "undefined") return null;
  const escaped =
    typeof CSS !== "undefined" && CSS.escape ? CSS.escape(target) : target;
  const nodes = document.querySelectorAll<HTMLElement>(
    `[data-guide="${escaped}"]`,
  );
  for (const node of nodes) {
    const r = node.getBoundingClientRect();
    if (r.width > 0 || r.height > 0) return node;
  }
  return null;
}

const QUERY = "(prefers-reduced-motion: reduce)";

function subscribe(cb: () => void) {
  const mq = window.matchMedia(QUERY);
  mq.addEventListener("change", cb);
  return () => mq.removeEventListener("change", cb);
}

/** The OS asks for reduced motion. */
export function usePrefersReducedMotion(): boolean {
  return useSyncExternalStore(
    subscribe,
    () => window.matchMedia(QUERY).matches,
    () => false,
  );
}
