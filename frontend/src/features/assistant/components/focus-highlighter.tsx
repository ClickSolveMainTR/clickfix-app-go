"use client";

import { useSearchParams } from "next/navigation";
import { Suspense, useEffect } from "react";

import {
  findGuideTarget,
  usePrefersReducedMotion,
} from "@/features/guides/lib/dom";

const TARGET = /^[a-z0-9][a-z0-9_.-]{0,59}$/;
const RING = ["ring-2", "ring-primary", "ring-offset-2", "rounded-md"];

/**
 * "Go to the page and highlight" (CF-184): `?focus=<target>` scrolls to
 * the element with `data-guide="<target>"` (the guide tours' anchor) and
 * rings it for a few seconds. The page may still be loading, so the
 * target is looked up for a short while; an unknown target is ignored.
 */
function Highlighter() {
  const focus = useSearchParams().get("focus") ?? "";
  const reduced = usePrefersReducedMotion();
  useEffect(() => {
    if (!TARGET.test(focus)) return;
    let tries = 0;
    let clear: ReturnType<typeof setTimeout> | undefined;
    let ringed: HTMLElement | null = null;
    const unring = () => {
      ringed?.classList.remove(...RING);
      ringed?.removeAttribute("data-assistant-focus");
      ringed = null;
    };
    const find = setInterval(() => {
      const el = findGuideTarget(focus);
      if (!el && ++tries < 30) return;
      clearInterval(find);
      if (!el) return;
      el.scrollIntoView({
        block: "center",
        behavior: reduced ? "auto" : "smooth",
      });
      ringed = el;
      el.classList.add(...RING);
      el.setAttribute("data-assistant-focus", "true");
      if (el.tabIndex >= 0 || el.matches("input,select,textarea,button,a")) {
        el.focus({ preventScroll: true });
      }
      clear = setTimeout(unring, 4000);
    }, 100);
    return () => {
      clearInterval(find);
      if (clear) clearTimeout(clear);
      unring();
    };
  }, [focus, reduced]);
  return null;
}

export function FocusHighlighter() {
  return (
    <Suspense fallback={null}>
      <Highlighter />
    </Suspense>
  );
}
