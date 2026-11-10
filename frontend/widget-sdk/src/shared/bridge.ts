/**
 * Webview bridge (CF-199): the postMessage contract between a native host
 * (Flutter, React Native, iOS WKWebView, Android WebView) or a parent
 * frame and the widget running inside the webview. Contract:
 * docs/webview-bridge.md. The transport-independent part lives here so
 * it can be unit tested without a DOM; widget.ts wires it to the SDK.
 *
 * Host -> widget messages are "commands" (`type` starts with `clickfix:`);
 * widget -> host messages are `clickfix:ready`, `clickfix:event`,
 * `clickfix:link` and, for commands that carry an `id`, `clickfix:result`.
 */

export const BRIDGE_VERSION = 1;

/** Largest accepted inbound message (bytes of its JSON text). */
export const MAX_MESSAGE_BYTES = 64 * 1024;

export type BridgeIdentity = {
  user_id: string;
  user_hash: string;
  name?: string;
  email?: string;
  phone?: string;
  company?: { id?: string; name?: string; domain?: string };
  attributes?: Record<string, string | number | boolean>;
};

export type BridgeCommand =
  | { type: "clickfix:identify"; id?: string; user: BridgeIdentity }
  | {
      type: "clickfix:track";
      id?: string;
      name: string;
      properties?: Record<string, unknown>;
    }
  | {
      type: "clickfix:open";
      id?: string;
      view?: string;
      conversation?: string;
    }
  | { type: "clickfix:close"; id?: string }
  | { type: "clickfix:update"; id?: string; update: Record<string, unknown> }
  | { type: "clickfix:consent"; id?: string; granted: boolean }
  | { type: "clickfix:reset"; id?: string }
  | { type: "clickfix:get-state"; id?: string };

export type BridgeCommandType = BridgeCommand["type"];

export const COMMAND_TYPES: readonly BridgeCommandType[] = [
  "clickfix:identify",
  "clickfix:track",
  "clickfix:open",
  "clickfix:close",
  "clickfix:update",
  "clickfix:consent",
  "clickfix:reset",
  "clickfix:get-state",
];

/** Widget -> host messages. */
export type BridgeOutbound =
  | {
      type: "clickfix:ready";
      bridge_version: number;
      protocol_version: number;
      sdk_version: string;
      widget_id: string;
      visitor_id: string;
    }
  | { type: "clickfix:event"; event: string; payload: unknown }
  | { type: "clickfix:link"; url: string }
  | {
      type: "clickfix:result";
      id: string;
      ok: boolean;
      data?: unknown;
      error?: { code: string; message: string };
    };

/** SDK events forwarded to the host as `clickfix:event`. */
export const FORWARDED_EVENTS: readonly string[] = [
  "ready",
  "open",
  "close",
  "view",
  "message",
  "message:sent",
  "conversation:started",
  "conversation:ended",
  "unread",
  "identify",
  "csat",
  "trigger",
  "attributes",
  "consent",
  "visibility",
  "reset",
  "track",
  "error",
];

export type ParseResult =
  | { ok: true; command: BridgeCommand }
  | { ok: false; id?: string; code: string; message: string };

const isRecord = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null && !Array.isArray(v);

const str = (v: unknown, max: number): string | undefined =>
  typeof v === "string" && v.length > 0 && v.length <= max ? v : undefined;

const scalar = (v: unknown) =>
  typeof v === "string" || typeof v === "number" || typeof v === "boolean";

function fail(code: string, message: string, id?: string): ParseResult {
  return { ok: false, id, code, message };
}

/**
 * Validates one inbound message (a JSON string or an already parsed
 * object). Rejections never echo the message content, so a bad
 * `user_hash` cannot leak through the result/console path.
 */
export function parseCommand(raw: unknown): ParseResult {
  let data: unknown = raw;
  if (typeof raw === "string") {
    if (raw.length > MAX_MESSAGE_BYTES)
      return fail("TOO_LARGE", `message exceeds ${MAX_MESSAGE_BYTES} bytes`);
    try {
      data = JSON.parse(raw);
    } catch {
      return fail("INVALID_JSON", "message is not valid JSON");
    }
  }
  if (!isRecord(data))
    return fail("INVALID_MESSAGE", "message is not an object");
  const type = str(data.type, 64);
  const id = str(data.id, 128);
  if (!type || !type.startsWith("clickfix:"))
    return fail("INVALID_MESSAGE", "type must start with clickfix:", id);
  if (!(COMMAND_TYPES as readonly string[]).includes(type))
    return fail("UNKNOWN_COMMAND", `unknown command ${type}`, id);
  const kind = type as BridgeCommandType;
  switch (kind) {
    case "clickfix:identify": {
      const u = data.user;
      if (!isRecord(u))
        return fail("INVALID_MESSAGE", "identify needs user", id);
      const user_id = str(u.user_id, 200);
      const user_hash = str(u.user_hash, 128);
      if (!user_id || !user_hash)
        return fail(
          "INVALID_MESSAGE",
          "identify needs user.user_id and user.user_hash (computed on your server)",
          id,
        );
      const user: BridgeIdentity = { user_id, user_hash };
      for (const k of ["name", "email", "phone"] as const) {
        const v = str(u[k], 255);
        if (v) user[k] = v;
      }
      if (isRecord(u.company)) {
        const c: BridgeIdentity["company"] = {};
        for (const k of ["id", "name", "domain"] as const) {
          const v = str(u.company[k], 255);
          if (v) c[k] = v;
        }
        if (Object.keys(c).length) user.company = c;
      }
      if (isRecord(u.attributes)) {
        const attrs: Record<string, string | number | boolean> = {};
        for (const [k, v] of Object.entries(u.attributes))
          if (k.length <= 100 && scalar(v)) attrs[k] = v;
        if (Object.keys(attrs).length) user.attributes = attrs;
      }
      return { ok: true, command: { type: kind, id, user } };
    }
    case "clickfix:track": {
      const name = str(data.name, 100);
      if (!name) return fail("INVALID_MESSAGE", "track needs name", id);
      const properties = isRecord(data.properties)
        ? data.properties
        : undefined;
      return { ok: true, command: { type: kind, id, name, properties } };
    }
    case "clickfix:open": {
      const view = str(data.view, 32);
      const conversation = str(data.conversation, 64);
      return { ok: true, command: { type: kind, id, view, conversation } };
    }
    case "clickfix:update": {
      if (!isRecord(data.update))
        return fail("INVALID_MESSAGE", "update needs update object", id);
      return { ok: true, command: { type: kind, id, update: data.update } };
    }
    case "clickfix:consent":
      if (typeof data.granted !== "boolean")
        return fail("INVALID_MESSAGE", "consent needs granted boolean", id);
      return { ok: true, command: { type: kind, id, granted: data.granted } };
    case "clickfix:close":
    case "clickfix:reset":
    case "clickfix:get-state":
      return { ok: true, command: { type: kind, id } };
  }
  return fail("UNKNOWN_COMMAND", `unknown command ${type}`, id);
}

export type Send = (message: BridgeOutbound) => void;

/**
 * Picks the host transport. Native webviews expose one of these objects;
 * a parent frame is used only when the integrator named its origin
 * (never "*"). Returns null when no host is reachable.
 */
export function detectTransport(
  win: unknown,
  parentOrigin?: string,
): { name: string; send: Send } | null {
  const w = win as Record<string, unknown>;
  const post = (obj: unknown): ((s: string) => void) | null => {
    const fn = isRecord(obj) ? obj.postMessage : undefined;
    return typeof fn === "function"
      ? (s: string) => (fn as (s: string) => void).call(obj, s)
      : null;
  };
  const native: [string, ((s: string) => void) | null][] = [
    ["react-native", post(w.ReactNativeWebView)],
    [
      "ios",
      post(
        isRecord(w.webkit) && isRecord(w.webkit.messageHandlers)
          ? w.webkit.messageHandlers.ClickFix
          : undefined,
      ),
    ],
    ["android", post(w.ClickFixAndroid)],
    ["flutter", post(w.ClickFixFlutter)],
  ];
  for (const [name, fn] of native)
    if (fn) return { name, send: (m) => fn(JSON.stringify(m)) };
  const parent = w.parent as Record<string, unknown> | undefined;
  if (
    parentOrigin &&
    parentOrigin !== "*" &&
    /^https?:\/\/[^/]+$/.test(parentOrigin) &&
    parent &&
    parent !== w &&
    typeof parent.postMessage === "function"
  ) {
    const pm = parent.postMessage as (m: unknown, o: string) => void;
    return { name: "frame", send: (m) => pm.call(parent, m, parentOrigin) };
  }
  return null;
}

/**
 * Decides whether a `message` DOM event may drive the bridge: from the
 * parent frame only when it is the configured origin; otherwise only
 * messages the page posted to itself (what native webviews do when they
 * cannot call `ClickFixBridge.receive` directly).
 */
export function acceptsMessageEvent(
  e: { origin: string; source: unknown },
  win: { parent: unknown; location: { origin: string } },
  parentOrigin?: string,
): boolean {
  const framed = win.parent !== win;
  if (e.source === win.parent && framed)
    return !!parentOrigin && parentOrigin !== "*" && e.origin === parentOrigin;
  if (e.source === win || e.source === null)
    return !framed && (e.origin === "" || e.origin === win.location.origin);
  return false;
}

export type Runner = (command: BridgeCommand) => unknown | Promise<unknown>;

export interface Bridge {
  /** Entry point for the host: `window.ClickFixBridge.receive(json)`. */
  receive(raw: unknown): Promise<void>;
  /** Forwards an SDK event to the host. */
  forward(event: string, payload: unknown): void;
  /** Sends the ready handshake. */
  ready(payload: {
    protocol_version: number;
    sdk_version: string;
    widget_id: string;
    visitor_id: string;
  }): void;
  link(url: string): void;
  readonly version: number;
}

function errorOf(err: unknown): { code: string; message: string } {
  const e = err as { code?: unknown; message?: unknown } | null;
  const code =
    e && typeof e.code === "string" && /^[A-Z0-9_]{1,64}$/.test(e.code)
      ? e.code
      : "COMMAND_FAILED";
  // Only the stable code is forwarded; messages may carry server text.
  return { code, message: `command failed (${code})` };
}

export function createBridge(options: { send: Send; run: Runner }): Bridge {
  const { send, run } = options;
  return {
    version: BRIDGE_VERSION,
    async receive(raw) {
      const parsed = parseCommand(raw);
      if (!parsed.ok) {
        if (parsed.id)
          send({
            type: "clickfix:result",
            id: parsed.id,
            ok: false,
            error: { code: parsed.code, message: parsed.message },
          });
        return;
      }
      const { command } = parsed;
      try {
        const data = await run(command);
        if (command.id)
          send({
            type: "clickfix:result",
            id: command.id,
            ok: true,
            ...(data === undefined ? {} : { data }),
          });
      } catch (err) {
        if (command.id)
          send({
            type: "clickfix:result",
            id: command.id,
            ok: false,
            error: errorOf(err),
          });
      }
    },
    forward(event, payload) {
      if (FORWARDED_EVENTS.includes(event))
        send({ type: "clickfix:event", event, payload });
    },
    ready(payload) {
      send({
        type: "clickfix:ready",
        bridge_version: BRIDGE_VERSION,
        ...payload,
      });
    },
    link(url) {
      send({ type: "clickfix:link", url });
    },
  };
}
