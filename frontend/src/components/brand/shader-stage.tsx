"use client";

import { useInView } from "motion/react";
import { Component, useRef, useSyncExternalStore, type ReactNode } from "react";

import { brand } from "@/config/brand";
import { usePrefersReducedMotion } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** Large screen with a fine pointer and no data saver: worth a WebGL canvas. */
const CAPABLE = "(min-width: 1024px) and (pointer: fine)";

let engaged = false;
let idle = false;
const listeners = new Set<() => void>();
const ENGAGE_EVENTS = ["pointermove", "pointerdown", "keydown", "wheel"];

function notify() {
  for (const listener of listeners) listener();
}

function onEngage() {
  engaged = true;
  for (const name of ENGAGE_EVENTS) window.removeEventListener(name, onEngage);
  notify();
}

function subscribe(listener: () => void) {
  if (listeners.size === 0 && !engaged) {
    for (const name of ENGAGE_EVENTS) {
      window.addEventListener(name, onEngage, { passive: true });
    }
  }
  if (!idle) {
    // The canvas waits until the page has loaded and gone quiet, so it never
    // competes with the first paint or hydration.
    const start = () => {
      const ric = window.requestIdleCallback ?? ((cb) => setTimeout(cb, 1));
      ric(
        () => {
          idle = true;
          notify();
        },
        { timeout: 3000 },
      );
    };
    if (document.readyState === "complete") start();
    else window.addEventListener("load", start, { once: true });
  }
  listeners.add(listener);
  return () => listeners.delete(listener);
}

let webgl: boolean | undefined;

/**
 * Whether this browser can create a WebGL 2 context at all (blocklisted GPU,
 * disabled hardware acceleration, some VMs). Paper Shaders throws
 * "WebGL is not supported in this browser" otherwise (CLICKFIX-FRONT-END-2).
 * Probed once; the probe context is released straight away.
 */
function supportsWebGL(): boolean {
  if (webgl !== undefined) return webgl;
  try {
    const canvas = document.createElement("canvas");
    // Paper Shaders needs WebGL 2 and throws from an async effect, out of
    // reach of an error boundary, so the probe has to run first.
    const gl = canvas.getContext("webgl2");
    webgl = gl !== null;
    gl?.getExtension("WEBGL_lose_context")?.loseContext();
  } catch {
    webgl = false;
  }
  return webgl;
}

function snapshot() {
  const capable =
    window.matchMedia(CAPABLE).matches &&
    !(navigator as Navigator & { connection?: { saveData?: boolean } })
      .connection?.saveData;
  if (!capable || !idle) return "off";
  if (!supportsWebGL()) return "static";
  return engaged ? "live" : "still";
}

/** Static stand-in for the shader: the same ink and greens as a CSS gradient. */
const STATIC_GRADIENT = `radial-gradient(ellipse at 20% 30%, ${brand.shader.mesh[3]}cc 0%, transparent 55%), radial-gradient(ellipse at 80% 70%, ${brand.shader.mesh[1]} 0%, transparent 60%), radial-gradient(ellipse at 65% 20%, ${brand.shader.mesh[5]}1f 0%, transparent 40%)`;

function StaticGradient() {
  return (
    <div
      className="size-full"
      style={{ backgroundImage: STATIC_GRADIENT }}
      data-shader-fallback=""
    />
  );
}

/** A shader that still fails (context lost, driver bug) falls back quietly. */
class ShaderBoundary extends Component<
  { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: unknown) {
    console.warn("[shader] falling back to a static gradient", error);
  }

  render() {
    return this.state.failed ? <StaticGradient /> : this.props.children;
  }
}

/**
 * Holds a WebGL shader background (CF-144 Core Web Vitals pass). The canvas
 * is created only on large screens with a fine pointer, after the page has
 * loaded and gone idle, when the section is within 300px of the viewport.
 * It animates only after the visitor first interacts, while it is on screen
 * and motion is allowed; until then it draws one still frame. Phones keep
 * the `className` background (ink): no canvas, no battery cost, no layout
 * shift. Without WebGL a static CSS gradient stands in for the canvas.
 */
export function ShaderStage({
  className,
  children,
}: {
  className?: string;
  children: (active: boolean) => ReactNode;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const near = useInView(ref, { once: true, margin: "300px 0px" });
  const visible = useInView(ref);
  const reducedMotion = usePrefersReducedMotion();
  const mode = useSyncExternalStore(subscribe, snapshot, () => "off");

  return (
    <div
      ref={ref}
      aria-hidden
      className={cn("pointer-events-none absolute inset-0", className)}
    >
      {near && mode === "static" ? <StaticGradient /> : null}
      {near && (mode === "still" || mode === "live") ? (
        <ShaderBoundary>
          {children(mode === "live" && visible && !reducedMotion)}
        </ShaderBoundary>
      ) : null}
    </div>
  );
}
