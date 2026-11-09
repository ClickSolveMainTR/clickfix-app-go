/**
 * ClickFix's own support widget in the panel (CF-193). The API gives the
 * support organization's widget id and the member's identity signature
 * (computed server-side; the identity secret never reaches the browser).
 * The widget then runs with the assistant tab for the active organization.
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
  assistant: { organization: string };
};

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const HASH = /^[0-9a-f]{64}$/;

/** Init options of the widget, or null when it should not load. */
export function supportWidgetInit(
  data: SupportWidgetData | null | undefined,
  user: SupportWidgetUser | null | undefined,
  organization: string,
  locale: string,
): SupportWidgetInit | null {
  if (
    !data?.available ||
    !user ||
    !organization ||
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
    assistant: { organization },
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
