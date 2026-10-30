/**
 * Proactive message engine: per page view it waits for the browser-side
 * conditions of every eligible trigger (time on page, scroll depth, exit
 * intent, then the delay) and fires each at most as often as its
 * frequency allows. Page-level conditions come from shared/targeting
 * (server-evaluated at bootstrap, re-evaluated on SPA navigation).
 */
import type { Trigger } from "../shared/schema";
import { eligible, type PageContext } from "../shared/targeting";
import type { Store } from "./store";

type Fire = (t: Trigger) => boolean;

export class TriggerEngine {
  private cleanups: (() => void)[] = [];
  private sessionFired = new Set<string>();
  private pageStart = Date.now();

  constructor(
    private store: Store,
    private fire: Fire,
  ) {}

  stop() {
    for (const c of this.cleanups.splice(0)) c();
  }

  /** Starts watching a page view. */
  start(triggers: Trigger[], ctx: PageContext, serverEligible?: string[]) {
    this.stop();
    this.pageStart = Date.now();
    const onceFired = this.store.json<string[]>("triggers", []);
    for (const t of triggers) {
      const ok = serverEligible
        ? serverEligible.includes(t.id)
        : eligible(t, ctx);
      if (!ok) continue;
      if (t.frequency === "once" && onceFired.includes(t.id)) continue;
      if (t.frequency === "session" && this.sessionFired.has(t.id)) continue;
      this.watch(t);
    }
  }

  private markFired(t: Trigger) {
    this.sessionFired.add(t.id);
    if (t.frequency === "once") {
      const list = this.store.json<string[]>("triggers", []);
      if (!list.includes(t.id))
        this.store.setJSON("triggers", [...list, t.id].slice(-100));
    }
  }

  private watch(t: Trigger) {
    const c = t.conditions;
    let done = false;
    let timeOk = c.time_on_page_s === 0;
    let scrollOk = c.scroll_pct === 0;
    let exitOk = !c.exit_intent;
    const timers: number[] = [];
    const maybe = () => {
      if (done || !timeOk || !scrollOk || !exitOk) return;
      done = true;
      timers.push(
        window.setTimeout(() => {
          if (this.fire(t)) this.markFired(t);
        }, t.delay_s * 1000),
      );
    };
    if (!timeOk) {
      const left = c.time_on_page_s * 1000 - (Date.now() - this.pageStart);
      timers.push(
        window.setTimeout(() => ((timeOk = true), maybe()), Math.max(0, left)),
      );
    }
    if (!scrollOk) {
      const onScroll = () => {
        const doc = document.documentElement;
        const max = doc.scrollHeight - window.innerHeight;
        const pct = max <= 0 ? 100 : (window.scrollY / max) * 100;
        if (pct >= c.scroll_pct) {
          scrollOk = true;
          window.removeEventListener("scroll", onScroll);
          maybe();
        }
      };
      window.addEventListener("scroll", onScroll, { passive: true });
      this.cleanups.push(() => window.removeEventListener("scroll", onScroll));
      onScroll();
    }
    if (!exitOk) {
      const onOut = (e: MouseEvent) => {
        if (e.clientY <= 0 && !e.relatedTarget) {
          exitOk = true;
          document.removeEventListener("mouseout", onOut);
          maybe();
        }
      };
      document.addEventListener("mouseout", onOut);
      this.cleanups.push(() => document.removeEventListener("mouseout", onOut));
    }
    this.cleanups.push(() => timers.forEach((x) => window.clearTimeout(x)));
    maybe();
  }
}

/**
 * Calls back on single-page-app navigations (pushState, replaceState,
 * popstate, hashchange). Returns the cleanup.
 */
export function watchNavigation(onChange: (url: string) => void): () => void {
  let last = location.href;
  const check = () => {
    if (location.href !== last) {
      last = location.href;
      onChange(last);
    }
  };
  const h = history as History & { __clickfixPatched?: boolean };
  const listeners = window as unknown as { __clickfixNav?: Set<() => void> };
  listeners.__clickfixNav ??= new Set();
  if (!h.__clickfixPatched) {
    h.__clickfixPatched = true;
    for (const m of ["pushState", "replaceState"] as const) {
      const orig = h[m].bind(h);
      h[m] = (...args: Parameters<History["pushState"]>) => {
        orig(...args);
        listeners.__clickfixNav?.forEach((fn) => fn());
      };
    }
  }
  listeners.__clickfixNav.add(check);
  window.addEventListener("popstate", check);
  window.addEventListener("hashchange", check);
  return () => {
    listeners.__clickfixNav?.delete(check);
    window.removeEventListener("popstate", check);
    window.removeEventListener("hashchange", check);
  };
}
