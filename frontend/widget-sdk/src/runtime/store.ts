/**
 * Visitor storage with the widget's persistence policy: "local" (survives
 * restarts), "session" (this tab) or "memory" (cookie-less, nothing written
 * to the browser). Until the visitor consents (privacy.consent_required)
 * everything stays in memory; granting consent moves it to the policy's
 * storage.
 */
export type Persistence = "local" | "session" | "memory";

export class Store {
  private memory = new Map<string, string>();
  private mode: Persistence = "memory";

  constructor(private prefix: string) {}

  private backend(): Storage | null {
    try {
      if (this.mode === "local") return window.localStorage;
      if (this.mode === "session") return window.sessionStorage;
    } catch {
      /* blocked storage: memory only */
    }
    return null;
  }

  /** Reads what an earlier visit stored (before settings are known). */
  peek(key: string): string | null {
    for (const area of ["localStorage", "sessionStorage"] as const) {
      try {
        const v = window[area].getItem(this.prefix + key);
        if (v !== null) return v;
      } catch {
        /* ignore */
      }
    }
    return null;
  }

  /** Switches the policy, moving the current values over. */
  use(mode: Persistence) {
    const values = new Map(this.memory);
    const old = this.backend();
    if (old)
      for (let i = 0; i < old.length; i++) {
        const k = old.key(i);
        if (k?.startsWith(this.prefix))
          values.set(k.slice(this.prefix.length), old.getItem(k) ?? "");
      }
    if (mode !== this.mode) this.clearBackend();
    this.mode = mode;
    this.memory.clear();
    for (const [k, v] of values) this.set(k, v);
  }

  get(key: string): string | null {
    const b = this.backend();
    if (b) {
      try {
        return b.getItem(this.prefix + key);
      } catch {
        /* fall through */
      }
    }
    return this.memory.get(key) ?? null;
  }

  set(key: string, value: string) {
    const b = this.backend();
    if (b) {
      try {
        b.setItem(this.prefix + key, value);
        return;
      } catch {
        /* quota or blocked: memory */
      }
    }
    this.memory.set(key, value);
  }

  remove(key: string) {
    this.memory.delete(key);
    for (const area of ["localStorage", "sessionStorage"] as const) {
      try {
        window[area].removeItem(this.prefix + key);
      } catch {
        /* ignore */
      }
    }
  }

  json<T>(key: string, fallback: T): T {
    try {
      const v = this.get(key);
      return v ? (JSON.parse(v) as T) : fallback;
    } catch {
      return fallback;
    }
  }

  setJSON(key: string, value: unknown) {
    this.set(key, JSON.stringify(value));
  }

  /** Forgets everything of this widget (logout, cookie-less switch). */
  clear() {
    this.memory.clear();
    this.clearBackend();
  }

  private clearBackend() {
    for (const area of ["localStorage", "sessionStorage"] as const) {
      try {
        const s = window[area];
        const keys: string[] = [];
        for (let i = 0; i < s.length; i++) {
          const k = s.key(i);
          if (k?.startsWith(this.prefix)) keys.push(k);
        }
        for (const k of keys) s.removeItem(k);
      } catch {
        /* ignore */
      }
    }
  }
}
