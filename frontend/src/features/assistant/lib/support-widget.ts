/**
 * ClickFix's own support widget in the panel (CF-193). The API gives the
 * support organization's widget id and the member's identity signature
 * (computed server-side; the identity secret never reaches the browser).
 * In the panel the assistant drawer is the single entry: the widget has
 * no launcher and opens only from the drawer, for a conversation with the
 * support team. Co-browsing and session replay stay off on panel pages.
 */
export type SupportWidgetData = {
  available: boolean;
  widget_id?: string;
  user_id?: string;
  user_hash?: string;
};

export type SupportWidgetUser = { name: string; email: string };

export type SupportWidgetInit = {
  widgetId: string;
  locale: string;
  user: { user_id: string; user_hash: string; name: string; email: string };
  hideLauncher: true;
  capture: false;
};

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const HASH = /^[0-9a-f]{64}$/;

/** Init options of the widget, or null when it should not load. */
export function supportWidgetInit(
  data: SupportWidgetData | null | undefined,
  user: SupportWidgetUser | null | undefined,
  locale: string,
): SupportWidgetInit | null {
  if (
    !data?.available ||
    !user ||
    !UUID.test(data.widget_id ?? "") ||
    !data.user_id ||
    !HASH.test(data.user_hash ?? "")
  )
    return null;
  return {
    widgetId: data.widget_id!,
    locale,
    user: {
      user_id: data.user_id,
      user_hash: data.user_hash!,
      name: user.name,
      email: user.email,
    },
    hideLauncher: true,
    capture: false,
  };
}

type ClickFixFn = ((...args: unknown[]) => unknown) & { q?: unknown[] };

/**
 * Loads /widget.js from the panel's own origin (the queue snippet of the
 * SDK) and initializes it; returns the cleanup that shuts it down.
 */
export function mountSupportWidget(init: SupportWidgetInit): () => void {
  const win = window as unknown as Window & { ClickFix?: ClickFixFn };
  if (!win.ClickFix) {
    const fn: ClickFixFn = (...args: unknown[]) => {
      (fn.q = fn.q || []).push(args);
    };
    win.ClickFix = fn;
  }
  win.ClickFix("init", init);
  let script = document.querySelector<HTMLScriptElement>(
    "script[data-clickfix-support]",
  );
  if (!script) {
    script = document.createElement("script");
    script.src = "/widget.js";
    script.async = true;
    script.dataset.clickfixSupport = "true";
    document.body.appendChild(script);
  }
  return () => win.ClickFix?.("shutdown");
}

/** Opens the support widget on a new conversation (from the drawer). */
export function openSupportChat(): boolean {
  const fn = (window as unknown as { ClickFix?: ClickFixFn }).ClickFix;
  if (!fn) return false;
  fn("startConversation");
  return true;
}
