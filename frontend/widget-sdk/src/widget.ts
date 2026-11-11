/**
 * ClickFix web widget SDK (CF-55, CF-150).
 *
 * One script tag on a customer's site:
 *
 *   <script src="https://<app>/widget.js" data-widget="<public id>" async></script>
 *
 * or the queue snippet (calls made before the script loads are replayed):
 *
 *   <script>
 *     window.ClickFix = window.ClickFix || function () {
 *       (window.ClickFix.q = window.ClickFix.q || []).push(arguments);
 *     };
 *     ClickFix("init", { widgetId: "<public id>" });
 *   </script>
 *   <script src="https://<app>/widget.js" async></script>
 *
 * The SDK talks to `https://<app>/api/v1/widget/<public id>/...` (CORS, a
 * visitor token in Authorization), renders inside a Shadow DOM so the host
 * page's CSS never leaks in, follows the visitor's private realtime channel
 * (Centrifugo, JSON protocol) and falls back to polling.
 *
 * Public contract (methods, events, payloads): public/widget.d.ts.
 * Bundled with esbuild (widget-sdk/build.mjs) into public/widget.js; no
 * runtime dependencies. `data-preview` turns the script into the editor's
 * live preview (no network; settings arrive by postMessage).
 * `data-bridge` (or `boot({ bridge })`) turns on the webview bridge for
 * native apps (CF-199, docs/webview-bridge.md).
 */
import { Widget } from "./runtime/widget";
import {
  acceptsMessageEvent,
  createBridge,
  detectTransport,
  FORWARDED_EVENTS,
  type Bridge,
  type BridgeCommand,
} from "./shared/bridge";
import type { PreviewState } from "./runtime/preview";
import type { Handler, IdentifyInput, InitOptions } from "./runtime/types";

type Queued = IArguments | unknown[];
type GlobalFn = ((...args: unknown[]) => unknown) & {
  q?: Queued[];
  __clickfix?: boolean;
} & Record<string, unknown>;

(() => {
  const win = window as unknown as Window & { ClickFix?: GlobalFn };
  if (win.ClickFix && win.ClickFix.__clickfix) return;

  const scriptEl =
    (document.currentScript as HTMLScriptElement | null) ??
    (document.querySelector(
      'script[src*="widget.js"]',
    ) as HTMLScriptElement | null);
  const scriptURL = new URL(
    scriptEl?.src || window.location.href,
    window.location.href,
  );
  const APP_ORIGIN = scriptURL.origin;
  const autoWidgetId = scriptEl?.dataset.widget ?? "";
  const previewMode = scriptEl?.dataset.preview === "true";
  const autoBridge = scriptEl?.dataset.bridge === "true";
  let bridge: Bridge | null = null;
  let unbindBridge: (() => void) | null = null;

  let instance: Widget | null = null;
  const early: [string, unknown[]][] = [];
  const earlyHandlers: [string, Handler, boolean][] = [];

  function init(opts: InitOptions = {}, preview: PreviewState | null = null) {
    const id = (
      opts.widgetId ||
      autoWidgetId ||
      (preview ? "preview" : "")
    ).trim();
    if (!id) {
      console.error(
        "[ClickFix] init: widgetId is required (data-widget attribute or init({ widgetId }))",
      );
      return;
    }
    if (instance) {
      instance.log("init called again; keeping the running widget");
      return;
    }
    instance = new Widget(id, opts, APP_ORIGIN, preview);
    for (const [event, handler, once] of earlyHandlers.splice(0))
      if (once) instance.once(event, handler);
      else instance.on(event, handler);
    if (!preview && (opts.bridge || autoBridge)) attachBridge(instance, opts);
    const replay = early.splice(0);
    void instance.start().then(() => {
      for (const [cmd, args] of replay) dispatch(cmd, args);
    });
  }

  /**
   * Webview bridge (CF-199): the native host (or an allowed parent frame)
   * drives the widget with `window.ClickFixBridge.receive(json)` or
   * postMessage, and receives every SDK event, external links and command
   * results through the detected transport. Contract: docs/webview-bridge.md.
   */
  function attachBridge(w: Widget, opts: InitOptions) {
    const origin =
      typeof opts.bridge === "object" ? opts.bridge.origin : undefined;
    const transport = detectTransport(window, origin);
    const send = transport ? transport.send : () => undefined;
    const run = (c: BridgeCommand): unknown => {
      switch (c.type) {
        case "clickfix:identify":
          return w.identify(c.user);
        case "clickfix:track":
          return w.track(c.name, c.properties);
        case "clickfix:open":
          return w.open(
            c.conversation ? { conversation: c.conversation } : c.view,
          );
        case "clickfix:close":
          return w.close();
        case "clickfix:update":
          return w.update(c.update);
        case "clickfix:consent":
          return w.consent(c.granted);
        case "clickfix:reset":
          return w.reset();
        case "clickfix:get-state":
          return w.state();
        case "clickfix:session":
          return w.adoptSession(c.token);
      }
    };
    bridge = createBridge({ send, run });
    const b = bridge;
    w.linkHandler = transport ? (url) => b.link(url) : null;
    const handlers: [string, Handler][] = [];
    for (const event of FORWARDED_EVENTS) {
      const handler: Handler = (payload) => {
        if (event === "ready")
          b.ready(
            payload as {
              protocol_version: number;
              sdk_version: string;
              widget_id: string;
              visitor_id: string;
            },
          );
        b.forward(event, payload);
        // The host may continue the session natively (CF-201).
        if (event === "ready" || event === "identify" || event === "reset")
          b.session({ token: w.api.token, visitor_id: w.state().visitor_id });
      };
      w.on(event, handler);
      handlers.push([event, handler]);
    }
    const onMessage = (e: MessageEvent) => {
      if (!acceptsMessageEvent(e, window, origin)) return;
      const data = e.data as { type?: unknown } | string | null;
      const typed =
        typeof data === "string"
          ? data.startsWith("{")
          : typeof data?.type === "string" && data.type.startsWith("clickfix:");
      if (typed) void b.receive(data);
    };
    window.addEventListener("message", onMessage);
    (win as unknown as Record<string, unknown>).ClickFixBridge = {
      version: b.version,
      transport: transport?.name ?? null,
      receive: (raw: unknown) => b.receive(raw),
    };
    unbindBridge = () => {
      window.removeEventListener("message", onMessage);
      for (const [event, handler] of handlers) w.off(event, handler);
      w.linkHandler = null;
      delete (win as unknown as Record<string, unknown>).ClickFixBridge;
      bridge = null;
      unbindBridge = null;
    };
    w.log("bridge attached", transport?.name ?? "no host transport");
  }

  function dispatch(command: string, args: unknown[]): unknown {
    if (command === "init" || command === "boot")
      return init((args[0] as InitOptions) ?? {});
    if ((command === "on" || command === "once") && !instance) {
      earlyHandlers.push([
        String(args[0]),
        args[1] as Handler,
        command === "once",
      ]);
      return undefined;
    }
    if (!instance) {
      if (command === "getUnreadCount") return 0;
      if (command === "isOpen") return false;
      early.push([command, args]);
      return undefined;
    }
    const w = instance;
    switch (command) {
      case "open":
        return w.open(args[0]);
      case "close":
        return w.close();
      case "toggle":
        return w.toggle();
      case "show":
        return w.setShown(true);
      case "hide":
        return w.setShown(false);
      case "showLauncher":
        return w.setLauncher(true);
      case "hideLauncher":
        return w.setLauncher(false);
      case "identify": {
        // identify(user) or identify(user, hash)
        const input = { ...(args[0] as IdentifyInput) };
        if (typeof args[1] === "string") input.user_hash = args[1];
        return w.identify(input);
      }
      case "update":
        return w.update(args[0] as Record<string, unknown>);
      case "setAttributes":
        return w.setAttributes(
          args[0] as Record<string, string | number | boolean>,
        );
      case "track":
        return w.track(
          String(args[0] ?? ""),
          args[1] as Record<string, unknown> | undefined,
        );
      case "startConversation":
      case "showNewMessage":
        return w.startConversation(
          args[0] as string | undefined,
          (args[1] as { send?: boolean }) ?? {},
        );
      case "showMessage":
        return w.showMessage(
          String(args[0] ?? ""),
          (args[1] as { open?: boolean; sender?: string }) ?? {},
        );
      case "openArticle":
        return w.openArticle(String(args[0] ?? ""));
      case "searchHelp":
        return w.searchHelp(String(args[0] ?? ""));
      case "setLocale":
        return w.setLocale(String(args[0] ?? ""));
      case "setTheme":
        return w.setTheme(String(args[0] ?? ""));
      case "consent":
        return w.consent(args[0] !== false);
      case "getUnreadCount":
        return w.unreadCount();
      case "getVisitorId":
        return w.visitorId();
      case "isOpen":
        return w.isPanelOpen();
      case "getState":
        return w.state();
      case "on":
        return w.on(String(args[0]), args[1] as Handler);
      case "once":
        return w.once(String(args[0]), args[1] as Handler);
      case "off":
        return w.off(String(args[0]), args[1] as Handler | undefined);
      case "startReplay":
        return w.startReplay();
      case "stopCapture":
        return w.stopCapture(Boolean(args[0]));
      case "reset":
        return w.reset();
      case "shutdown":
        unbindBridge?.();
        w.destroy();
        instance = null;
        return undefined;
      default:
        console.warn(`[ClickFix] unknown command "${command}"`);
        return undefined;
    }
  }

  const COMMANDS = [
    "init",
    "boot",
    "open",
    "close",
    "toggle",
    "show",
    "hide",
    "showLauncher",
    "hideLauncher",
    "identify",
    "update",
    "setAttributes",
    "track",
    "startConversation",
    "showNewMessage",
    "showMessage",
    "openArticle",
    "searchHelp",
    "setLocale",
    "setTheme",
    "consent",
    "getUnreadCount",
    "getVisitorId",
    "isOpen",
    "getState",
    "on",
    "once",
    "off",
    "startReplay",
    "stopCapture",
    "reset",
    "shutdown",
  ];

  const queued = win.ClickFix?.q ?? [];
  const api = ((command: string, ...args: unknown[]) =>
    dispatch(command, args)) as GlobalFn;
  api.__clickfix = true;
  api.version = "2.1.0";
  for (const name of COMMANDS)
    api[name] = (...args: unknown[]) => dispatch(name, args);
  win.ClickFix = api;

  if (previewMode) {
    // Editor preview: only same-origin parents may drive it.
    const render = (state: PreviewState) => {
      instance?.destroy();
      instance = null;
      init(
        {
          widgetId: "preview",
          locale: state.locale,
          theme: state.theme,
          dir: state.dir,
        },
        state,
      );
    };
    window.addEventListener("message", (e: MessageEvent) => {
      if (e.origin !== window.location.origin) return;
      const data = e.data as { type?: string; state?: PreviewState } | null;
      if (data?.type === "clickfix:preview" && data.state) render(data.state);
    });
    window.parent?.postMessage(
      { type: "clickfix:preview-ready" },
      window.location.origin,
    );
    return;
  }

  let initQueued = false;
  for (const call of queued) {
    const [command, ...args] = Array.from(call as ArrayLike<unknown>);
    if (command === "init" || command === "boot") initQueued = true;
    dispatch(String(command), args);
  }
  if (!initQueued && autoWidgetId && scriptEl?.dataset.manual !== "true") {
    const go = () => init({ widgetId: autoWidgetId });
    if (document.body) go();
    else document.addEventListener("DOMContentLoaded", go, { once: true });
  }
})();
