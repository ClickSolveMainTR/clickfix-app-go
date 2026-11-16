/**
 * ClickFix Masaüstü web köprüsü (CF-236, docs/design/masaustu.md §4).
 *
 * The desktop app (Tauri, `desktop/apps/agent`) loads this web app and, on
 * the app origin only, defines `window.__CLICKFIX_DESKTOP__` before any page
 * script runs. Every function here is a no-op outside the desktop app: in a
 * normal browser nothing is called and nothing is listened to.
 *
 * App code must use this module instead of reading `window.__TAURI_INTERNALS__`
 * itself. Commands (Rust, `remote-app` capability, app origin only):
 * `notify`, `set_badge`, `set_presence`, `open_external`, `version`,
 * `sign_out`. Events from the app arrive as the `clickfix:desktop` DOM event
 * (`command-palette`, `presence`, `sign-out`).
 *
 * Contract version 1; only backward compatible additions (new command, new
 * field, new event). Unknown events are ignored. Free of imports so it runs
 * under `node --test`.
 */

export const DESKTOP_BRIDGE_VERSION = 1;
export const DESKTOP_EVENT = "clickfix:desktop";

export type DesktopPresence = "online" | "away" | "busy" | "offline";

export type DesktopInfo = {
  bridge: number;
  version: string;
  os: "macos" | "windows" | "linux" | string;
};

export type DesktopCommand =
  | { type: "command-palette" }
  | { type: "presence"; status: DesktopPresence }
  | { type: "sign-out" };

export type DesktopNotification = {
  title: string;
  body?: string | null;
  /** In-app path opened when the notification is clicked (`/inbox?c=…`). */
  path?: string | null;
};

type Invoke = (cmd: string, args?: Record<string, unknown>) => Promise<unknown>;

type DesktopWindow = {
  __CLICKFIX_DESKTOP__?: Partial<DesktopInfo>;
  __TAURI_INTERNALS__?: { invoke?: Invoke };
  addEventListener?: (type: string, fn: (e: Event) => void) => void;
  removeEventListener?: (type: string, fn: (e: Event) => void) => void;
};

const PRESENCE: readonly string[] = ["online", "away", "busy", "offline"];

function currentWindow(): DesktopWindow | null {
  return typeof window === "undefined"
    ? null
    : (window as unknown as DesktopWindow);
}

function invoker(): Invoke | null {
  const w = currentWindow();
  const info = w?.__CLICKFIX_DESKTOP__;
  const invoke = w?.__TAURI_INTERNALS__?.invoke;
  if (!info || info.bridge !== DESKTOP_BRIDGE_VERSION) return null;
  return typeof invoke === "function" ? invoke : null;
}

/** Runs inside ClickFix Masaüstü (bridge version 1)? */
export function isDesktop(): boolean {
  return invoker() !== null;
}

/** What the app announced (no IPC call); null outside the desktop app. */
export function desktopInfo(): DesktopInfo | null {
  if (!isDesktop()) return null;
  const info = currentWindow()!.__CLICKFIX_DESKTOP__!;
  return {
    bridge: DESKTOP_BRIDGE_VERSION,
    version: String(info.version ?? ""),
    os: String(info.os ?? ""),
  };
}

async function call<T>(
  cmd: string,
  args?: Record<string, unknown>,
): Promise<T | null> {
  const invoke = invoker();
  if (!invoke) return null;
  try {
    return (await invoke(cmd, args)) as T;
  } catch {
    // The app logs its own errors; the web app never breaks on them.
    return null;
  }
}

/** Only in-app paths are passed on (`/…`, never `//host` or a URL). */
export function desktopPath(path: string | null | undefined): string | null {
  const p = (path ?? "").trim();
  if (!p.startsWith("/") || p.startsWith("//") || p.includes("\\")) {
    return null;
  }
  return p.length <= 2048 ? p : null;
}

/**
 * Native notification. The app shows it only while its window is not in
 * front (the in-app toast covers that case) and returns whether it did.
 */
export async function notify(n: DesktopNotification): Promise<boolean> {
  const title = n.title?.trim();
  if (!title) return false;
  const shown = await call<boolean>("notify", {
    title,
    body: n.body?.trim() || null,
    path: desktopPath(n.path),
  });
  return shown === true;
}

/** Unread assigned conversations: dock badge, taskbar overlay, tray tooltip. */
export async function setBadge(count: number): Promise<void> {
  const n = Number.isFinite(count) ? Math.max(0, Math.floor(count)) : 0;
  await call("set_badge", { count: Math.min(n, 9999) });
}

/** The agent's status, shown as the checked item in the tray menu. */
export async function setPresence(
  status: DesktopPresence | null,
): Promise<void> {
  await call("set_presence", {
    status: status && PRESENCE.includes(status) ? status : null,
  });
}

/**
 * Opens an http(s) / mailto link in the system browser. Returns false outside
 * the desktop app so the caller can fall back to `window.open`.
 */
export async function openExternal(url: string): Promise<boolean> {
  if (!isDesktop()) return false;
  await call("open_external", { url });
  return true;
}

export async function version(): Promise<DesktopInfo | null> {
  return call<DesktopInfo>("version");
}

/** Signs the desktop app out too (its keychain session) after a web logout. */
export async function signOut(): Promise<void> {
  await call("sign_out");
}

/** Validates a `clickfix:desktop` event detail. */
export function parseDesktopCommand(detail: unknown): DesktopCommand | null {
  if (!detail || typeof detail !== "object") return null;
  const d = detail as Record<string, unknown>;
  switch (d.type) {
    case "command-palette":
      return { type: "command-palette" };
    case "sign-out":
      return { type: "sign-out" };
    case "presence":
      return typeof d.status === "string" && PRESENCE.includes(d.status)
        ? { type: "presence", status: d.status as DesktopPresence }
        : null;
    default:
      return null;
  }
}

/**
 * Listens to commands from the app (tray, global shortcut). Outside the
 * desktop app no listener is added. Returns the unsubscribe function.
 */
export function onDesktopCommand(
  handler: (command: DesktopCommand) => void,
): () => void {
  const w = currentWindow();
  if (!isDesktop() || !w?.addEventListener) return () => {};
  const listener = (e: Event) => {
    const command = parseDesktopCommand((e as CustomEvent).detail);
    if (command) handler(command);
  };
  w.addEventListener(DESKTOP_EVENT, listener);
  return () => w.removeEventListener?.(DESKTOP_EVENT, listener);
}
