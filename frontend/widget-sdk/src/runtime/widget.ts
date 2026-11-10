/**
 * The widget runtime: one instance per page. It bootstraps the visitor
 * session, renders the launcher, the proactive bubble and the panel inside
 * a Shadow DOM, follows the visitor's realtime channel (polling as the
 * fallback), queues messages while offline and emits the public SDK events
 * (public/widget.d.ts).
 */
import {
  acceptList,
  attachmentKind,
  normalize,
  pick,
  text as settingsText,
  type HomeCard,
  type Locale,
  type Trigger,
} from "../shared/schema";
import {
  detectLocale,
  format,
  RTL,
  STRINGS,
  type StringKey,
} from "../shared/i18n";
import { sanitizeCSS } from "../shared/css";
import { identifyFailure } from "../shared/identify";
import { plain } from "../shared/markdown";
import {
  deviceOf,
  eligible,
  visible as targetVisible,
  type PageContext,
} from "../shared/targeting";
import { Api, ApiError, isNetworkError, type Transport } from "./api";
import { Capture, type ReplayPolicy, type ReplaySession } from "./capture";
import { el, icon, renderText, type Child, type IconName } from "./dom";
import {
  AssistantClient,
  assistantAllowed,
  assistantHref,
  type AssistantAction,
  type AssistantMessage,
} from "./assistant";
import { askPermission, chime, notify } from "./notify";
import { FakeApi, type PreviewState } from "./preview";
import { Realtime, type RealtimeTokens } from "./realtime";
import { Store } from "./store";
import { CSS, themeVars } from "./styles";
import { TriggerEngine, watchNavigation } from "./triggers";
import type {
  BookingLink,
  Bootstrap,
  Conversation,
  FormCondition,
  FormField,
  Handler,
  HelpResult,
  IdentifyInput,
  InitOptions,
  Message,
  PublicSettings,
  TrackedEvent,
  View,
  Visitor,
} from "./types";

export const SDK_VERSION = "2.0.0";
export const PROTOCOL_VERSION = 1;

const EMOJIS =
  "😀 😂 😊 😍 🙏 👍 👎 👏 🎉 🔥 ❤️ 😢 😮 😅 🤔 🙌 ✅ ❌ ⭐ 💡 📦 🚚 💳 📞 ⏰ 👋 🤝 😉 😎 🥳 😡 💬".split(
    " ",
  );
const CSAT_EMOJI = ["😠", "🙁", "😐", "🙂", "😍"];

type QueuedSend = {
  text: string;
  file: File | null;
  message: Message;
  intent: string;
};
type Proactive = { text: string; sender: string; trigger?: Trigger };

function uuid(): string {
  const c = window.crypto as Crypto & { randomUUID?: () => string };
  if (typeof c.randomUUID === "function") return c.randomUUID();
  const b = new Uint8Array(16);
  c.getRandomValues(b);
  b[6] = (b[6]! & 0x0f) | 0x40;
  b[8] = (b[8]! & 0x3f) | 0x80;
  const h = Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}

const validEmail = (v: string) =>
  /^[^\s@<>,;]+@[^\s@<>,;]+\.[^\s@<>,;]+$/.test(v.trim());

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export class Widget {
  readonly api: Transport;
  readonly store: Store;
  private capture: Capture;
  private replayPolicy: ReplayPolicy | null = null;
  private boot: Bootstrap | null = null;
  private s: PublicSettings | null = null;
  private locale: "tr" | "en" = "tr";
  private dir: "ltr" | "rtl" = "ltr";
  private isOpen = false;
  private view: View = "home";
  private conversations: Conversation[] = [];
  private active: Conversation | null = null;
  private messages: Message[] = [];
  private agentTyping = false;
  private typingTimer: number | undefined;
  private lastTypingSent = 0;
  private typingStop: number | undefined;
  private pollTimer: number | undefined;
  private realtime: Realtime | null = null;
  private handlers: Record<string, Handler[]> = {};
  private events: TrackedEvent[] = [];
  private unread = 0;
  private draft = "";
  private profile = { name: "", email: "", phone: "" };
  private consentChecked = false;
  private formValues: Record<string, unknown> = {};
  private error = "";
  private ready = false;
  private ratingScore = 0;
  private help: HelpResult | null = null;
  private helpQuery = "";
  private helpLoading = false;
  private helpTimer: number | undefined;
  private helpSeq = 0;
  private booking: BookingLink | null = null;
  private bookingLoading = false;
  private proactive: Proactive | null = null;
  private intro: Proactive | null = null;
  private emojiOpen = false;
  private notice = "";
  private outbox: QueuedSend[] = [];
  private pendingAttrs: Record<string, string | number | boolean> = {};
  private consentGiven = false;
  private hiddenBy = ""; // targeting reason
  private shown = true; // ClickFix('show' | 'hide')
  private launcherHidden: boolean;
  private triggers: TriggerEngine;
  private stopNav: (() => void) | null = null;
  private lastSignature = "";
  private pollPending = false;
  private host: HTMLElement | null = null;
  private shadow: ShadowRoot | null = null;
  private rootEl: HTMLDivElement | null = null;
  private panelEl: HTMLDivElement | null = null;
  private launcherEl: HTMLButtonElement | null = null;
  private bubbleEl: HTMLDivElement | null = null;
  private styleEl: HTMLStyleElement | null = null;
  private opts: InitOptions;
  private pendingIdentify: IdentifyInput | null;
  private unbind: (() => void)[] = [];
  // ClickFix Asistan mode (CF-193): panel origin only.
  private assistant: AssistantClient | null = null;
  private aMessages: AssistantMessage[] = [];
  private aBusy = false;
  private aDraft = "";
  private aError = "";

  constructor(
    readonly widgetId: string,
    opts: InitOptions,
    private appOrigin: string,
    previewState: PreviewState | null = null,
  ) {
    this.opts = opts;
    this.api = previewState
      ? new FakeApi(previewState)
      : new Api(`${appOrigin}/api/v1/widget/${encodeURIComponent(widgetId)}`);
    this.store = new Store(`clickfix:${widgetId}:`);
    if (!previewState) this.api.token = this.store.peek("token") ?? "";
    this.consentGiven =
      opts.consent === true || this.store.peek("consent") === "1";
    this.pendingIdentify = opts.user ?? null;
    this.pendingAttrs = { ...(opts.attributes ?? {}) };
    this.launcherHidden = Boolean(opts.hideLauncher);
    this.capture = new Capture({
      api: this.api,
      t: (k) => this.t(k),
      emit: (e, p) => this.emit(e, p),
      appOrigin,
    });
    this.triggers = new TriggerEngine(this.store, (t) => this.fireTrigger(t));
    if (
      !previewState &&
      opts.assistant &&
      assistantAllowed(appOrigin, window.location?.origin ?? "", opts.assistant)
    )
      this.assistant = new AssistantClient(
        opts.assistant.organization,
        () => this.locale,
      );
  }

  t(key: StringKey): string {
    return STRINGS[this.locale][key];
  }

  log(...args: unknown[]) {
    if (this.opts.debug) console.info("[ClickFix]", ...args);
  }

  private get preview() {
    return this.api instanceof FakeApi ? this.api : null;
  }

  // -- lifecycle --------------------------------------------------------------

  async start() {
    try {
      await this.bootstrap();
    } catch (err) {
      this.log("bootstrap failed", err);
      this.emit("error", {
        code: err instanceof ApiError ? err.code : "BOOTSTRAP_FAILED",
        message: String(err),
      });
      return;
    }
    this.mount();
    if (this.preview) {
      this.applyPreview();
    } else {
      if (this.pendingIdentify) {
        const input = this.pendingIdentify;
        this.pendingIdentify = null;
        // identify() already warned and emitted "error"; boot carries on
        // with the anonymous visitor.
        await this.identify(input).catch(() => undefined);
      }
      if (Object.keys(this.pendingAttrs).length) void this.flushAttributes();
      this.startRealtime();
      this.schedulePoll();
      this.watchPage();
      const online = () => {
        this.notice = "";
        this.realtime?.kick();
        void this.flushOutbox();
        this.render();
      };
      const offline = () => {
        this.notice = this.t("networkOffline");
        this.render();
      };
      window.addEventListener("online", online);
      window.addEventListener("offline", offline);
      this.unbind.push(() => window.removeEventListener("online", online));
      this.unbind.push(() => window.removeEventListener("offline", offline));
    }
    this.ready = true;
    this.emit("ready", this.readyPayload());
  }

  private readyPayload() {
    return {
      protocol_version: PROTOCOL_VERSION,
      sdk_version: SDK_VERSION,
      widget_id: this.widgetId,
      visitor_id: this.boot?.visitor.id ?? "",
    };
  }

  private async bootstrap() {
    const data =
      this.api instanceof FakeApi
        ? this.api.bootstrap()
        : await this.api.call<Bootstrap>("POST", "/bootstrap", {
            page_url: location.href.slice(0, 2000),
            referrer: document.referrer.slice(0, 500),
            language: (navigator.language || "").slice(0, 2),
            sdk_version: SDK_VERSION,
          });
    this.boot = data;
    this.applySettings();
    const privacy = this.s!.privacy;
    if (privacy.cookieless) this.store.clear();
    this.store.use(
      privacy.consent_required && !this.consentGiven
        ? "memory"
        : privacy.persistence,
    );
    this.setVisitor(data.visitor);
    this.setConversations(data.conversations);
    const ctx = data.context;
    this.hiddenBy = ctx && !ctx.visible ? ctx.hidden_reason || "hidden" : "";
    if (!this.preview)
      this.replayPolicy = await this.api
        .call<ReplayPolicy>("GET", "/replay-policy")
        .catch(() => null);
  }

  /** Settings with the runtime overrides (init options, update, preview). */
  private applySettings() {
    if (!this.boot) return;
    const raw = this.boot.widget.settings;
    // Settings from the API are complete; normalize again for drafts and
    // older servers (missing v2 groups get their defaults).
    const n = normalize(raw) as unknown as PublicSettings;
    const s: PublicSettings = {
      ...n,
      ai: raw.ai ?? { enabled: true, human_button: true },
      attribute_keys: raw.attribute_keys ?? [],
    };
    if (this.opts.color && /^#[0-9a-f]{6}$/i.test(this.opts.color))
      s.color = this.opts.color.toLowerCase();
    if (this.opts.position) s.position = this.opts.position;
    if (this.opts.theme)
      s.appearance = { ...s.appearance, theme: this.opts.theme };
    this.s = s;
    const loc = this.opts.locale ?? (s.locale === "auto" ? "" : s.locale);
    const code = (loc || "").toLowerCase().slice(0, 2);
    this.locale =
      code === "tr" || code === "en"
        ? code
        : detectLocale(navigator.languages ?? [navigator.language]);
    this.dir = this.opts.dir ?? (RTL.includes(code) ? "rtl" : "ltr");
  }

  private setVisitor(v: Visitor) {
    this.api.token = v.token;
    if (!this.preview) this.store.set("token", v.token);
    if (this.boot) this.boot.visitor = v;
    if (v.contact) {
      this.profile.name = this.profile.name || v.contact.name;
      this.profile.email = this.profile.email || v.contact.email || "";
      this.profile.phone = this.profile.phone || v.contact.phone || "";
    }
  }

  private setConversations(list: Conversation[]) {
    const before = new Map(this.conversations.map((c) => [c.uuid, c]));
    this.conversations = list;
    for (const c of list) {
      const old = before.get(c.uuid);
      if (old && old.open && !c.open)
        this.emit("conversation:ended", {
          conversation_uuid: c.uuid,
          status: c.status,
        });
    }
    if (this.active) {
      const fresh = list.find((c) => c.uuid === this.active!.uuid);
      if (fresh) this.active = fresh;
    }
    const unread = list.reduce((n, c) => n + c.unread_count, 0);
    if (unread !== this.unread) {
      this.unread = unread;
      this.emit("unread", { count: unread });
      this.syncCustomLaunchers();
    }
  }

  destroy() {
    void this.capture.stopCapture(true);
    this.realtime?.stop();
    this.triggers.stop();
    this.stopNav?.();
    for (const u of this.unbind.splice(0)) u();
    window.clearTimeout(this.pollTimer);
    this.host?.remove();
    this.host = null;
  }

  // -- page targeting and triggers --------------------------------------------

  private pageContext(url = location.href): PageContext {
    const b = this.boot!;
    return {
      url,
      device: b.context?.device ?? deviceOf(navigator.userAgent),
      country: b.context?.country ?? "",
      language: (navigator.language || "").slice(0, 2),
      returning: b.context?.returning ?? false,
      identified: b.visitor.verified,
    };
  }

  private watchPage() {
    const s = this.s!;
    this.triggers.start(
      s.triggers,
      this.pageContext(),
      this.boot?.context?.eligible_triggers,
    );
    this.stopNav = watchNavigation((url) => {
      const ctx = this.pageContext(url);
      const v = targetVisible(s.targeting, ctx);
      const server = this.boot?.context;
      // Country and offline decisions come from the server only.
      const keep =
        server &&
        !server.visible &&
        (server.hidden_reason === "country" ||
          server.hidden_reason === "offline");
      const was = this.hiddenBy;
      this.hiddenBy = keep ? this.hiddenBy : v.visible ? "" : v.reason;
      if (was !== this.hiddenBy)
        this.emit("visibility", {
          visible: !this.hiddenBy,
          reason: this.hiddenBy,
        });
      this.triggers.start(
        s.triggers.filter((t) => eligible(t, ctx)),
        ctx,
      );
      this.render();
    });
  }

  private fireTrigger(t: Trigger): boolean {
    if (this.isOpen || this.hiddenBy || !this.shown) return false;
    const text = pick(t.message, this.locale as Locale);
    if (!text) return false;
    this.showMessage(text, { open: t.open, trigger: t });
    this.emit("trigger", { id: t.id, name: t.name, message: text });
    return true;
  }

  /** Programmatic proactive message (also used by triggers). */
  showMessage(
    text: string,
    opts: { open?: boolean; sender?: string; trigger?: Trigger } = {},
  ) {
    const msg = String(text ?? "")
      .trim()
      .slice(0, 1000);
    if (!msg) return;
    const sender =
      opts.sender ?? this.boot?.widget.team[0]?.name ?? this.t("team");
    this.proactive = { text: msg, sender, trigger: opts.trigger };
    if (this.s?.features.sound && !this.preview) chime();
    if (opts.open) this.openProactive();
    else this.render();
  }

  private openProactive() {
    const p = this.proactive;
    this.proactive = null;
    if (!p) return;
    this.intro = p;
    this.isOpen = true;
    void this.openConversation(null);
    this.emit("open", { view: "chat" });
  }

  // -- public commands --------------------------------------------------------

  open(view?: unknown) {
    if (!this.panelEl) return;
    const target =
      typeof view === "string"
        ? view
        : (view as { view?: string; conversation?: string } | undefined)?.view;
    const conv = (view as { conversation?: string } | undefined)?.conversation;
    this.proactive = null;
    if (!this.isOpen) {
      this.isOpen = true;
      this.emit("open", { view: target ?? this.view });
      if (!this.preview) void this.refresh();
    }
    if (conv) {
      const c = this.conversations.find((x) => x.uuid === conv);
      if (c) void this.openConversation(c);
    } else if (target === "new" || target === "chat")
      void this.openConversation(null);
    else if (target === "home" || target === "messages" || target === "help")
      this.go(target);
    this.render();
    this.focusFirst();
  }

  close() {
    if (this.isOpen) {
      this.isOpen = false;
      this.emojiOpen = false;
      this.render();
      this.emit("close", {});
      this.launcherEl?.focus();
    }
  }

  toggle() {
    if (this.isOpen) this.close();
    else this.open();
  }

  /** Shows or hides the whole widget (launcher, bubble and panel). */
  setShown(on: boolean) {
    this.shown = on;
    if (!on) this.isOpen = false;
    this.render();
  }

  setLauncher(on: boolean) {
    this.launcherHidden = !on;
    this.render();
  }

  isPanelOpen() {
    return this.isOpen;
  }

  unreadCount() {
    return this.unread;
  }

  visitorId() {
    return this.boot?.visitor.id ?? "";
  }

  state() {
    return {
      open: this.isOpen,
      view: this.view,
      visible: this.shown && !this.hiddenBy,
      hidden_reason: this.hiddenBy,
      unread: this.unread,
      locale: this.locale,
      online: this.boot?.widget.online ?? false,
      conversation_uuid: this.active?.uuid ?? null,
      visitor_id: this.visitorId(),
      verified: this.boot?.visitor.verified ?? false,
    };
  }

  on(event: string, handler: Handler) {
    if (typeof handler !== "function") return;
    (this.handlers[event] ??= []).push(handler);
    if (event === "ready" && this.ready) handler(this.readyPayload());
  }

  once(event: string, handler: Handler) {
    const wrap: Handler = (p) => {
      this.off(event, wrap);
      handler(p);
    };
    this.on(event, wrap);
  }

  off(event: string, handler?: Handler) {
    if (!handler) delete this.handlers[event];
    else
      this.handlers[event] = (this.handlers[event] ?? []).filter(
        (h) => h !== handler,
      );
  }

  emit(event: string, payload: unknown) {
    this.log("event", event, payload);
    const aliases: Record<string, string> = {
      conversation: "conversation:started",
      rating: "csat",
    };
    for (const name of [event, aliases[event]]) {
      if (!name) continue;
      for (const h of this.handlers[name] ?? []) {
        try {
          h(payload);
        } catch (err) {
          console.error("[ClickFix] handler error", err);
        }
      }
    }
  }

  track(name: string, properties?: Record<string, unknown>) {
    if (typeof name !== "string" || !name.trim()) return;
    this.events.push({
      name: name.trim().slice(0, 100),
      properties,
      at: new Date().toISOString(),
    });
    if (this.events.length > 20) this.events.shift();
    this.emit("track", { name, properties });
  }

  async identify(input: IdentifyInput) {
    if (!input || !input.user_id) return;
    if (!this.boot) {
      this.pendingIdentify = input;
      return;
    }
    if (this.preview) return;
    const { attributes, ...body } = input;
    let v: Visitor;
    try {
      v = await this.api.call<Visitor>("POST", "/identify", body);
    } catch (err) {
      // Visible without `debug`: otherwise the user silently stays
      // anonymous. The text never carries user_hash or other input.
      const failure = identifyFailure(err);
      console.warn("[ClickFix] " + failure.message);
      this.emit("error", failure);
      throw err;
    }
    this.setVisitor(v);
    this.emit("identify", { verified: v.verified });
    if (attributes) await this.setAttributes(attributes);
    this.realtime?.stop();
    this.startRealtime();
    await this.refresh();
  }

  /** Update: profile fields, attributes and runtime overrides at once. */
  async update(input: Record<string, unknown>) {
    if (!input || typeof input !== "object") return;
    const {
      name,
      email,
      phone,
      attributes,
      locale,
      theme,
      position,
      color,
      hideLauncher,
      dir,
      ...rest
    } = input;
    const overrides: Partial<InitOptions> = {};
    if (typeof locale === "string") overrides.locale = locale;
    if (theme === "light" || theme === "dark" || theme === "auto")
      overrides.theme = theme;
    if (position === "left" || position === "right")
      overrides.position = position;
    if (typeof color === "string") overrides.color = color;
    if (dir === "ltr" || dir === "rtl") overrides.dir = dir;
    if (Object.keys(overrides).length) {
      this.opts = { ...this.opts, ...overrides };
      this.applySettings();
      this.restyle();
    }
    if (typeof hideLauncher === "boolean") this.setLauncher(!hideLauncher);
    const profile: Record<string, string> = {};
    for (const [k, v] of Object.entries({ name, email, phone }))
      if (typeof v === "string" && v.trim()) profile[k] = v.trim();
    if (Object.keys(profile).length) await this.updateProfile(profile);
    const attrs = {
      ...(attributes as Record<string, string | number | boolean> | undefined),
      ...pickScalars(rest),
    };
    if (Object.keys(attrs).length) await this.setAttributes(attrs);
  }

  async updateProfile(input: {
    name?: string;
    email?: string;
    phone?: string;
  }) {
    if (input.name) this.profile.name = input.name;
    if (input.email) this.profile.email = input.email;
    if (input.phone) this.profile.phone = input.phone;
    if (!this.boot || this.preview) return;
    const v = await this.api.call<Visitor>("PATCH", "/profile", input);
    this.setVisitor(v);
  }

  async setAttributes(attrs: Record<string, string | number | boolean>) {
    Object.assign(this.pendingAttrs, pickScalars(attrs));
    if (!this.boot || this.preview) return;
    await this.flushAttributes();
  }

  private async flushAttributes() {
    const attrs = { ...this.pendingAttrs };
    if (!Object.keys(attrs).length) return;
    try {
      const out = await this.api.call<{
        applied: string[];
        ignored: string[];
        pending: boolean;
      }>("PATCH", "/attributes", { attributes: attrs });
      if (!out.pending)
        for (const k of Object.keys(attrs)) delete this.pendingAttrs[k];
      this.emit("attributes", out);
    } catch (err) {
      this.log("attributes failed", err);
    }
  }

  setLocale(locale: string) {
    void this.update({ locale });
  }

  setTheme(theme: string) {
    void this.update({ theme });
  }

  /** Consent from the site's own consent manager (or the visitor). */
  consent(granted: boolean) {
    this.consentGiven = granted;
    const p = this.s?.privacy;
    if (granted) {
      this.store.use(p?.persistence ?? "local");
      this.store.set("consent", "1");
      if (this.boot) this.store.set("token", this.api.token);
    } else {
      this.store.clear();
      this.store.use("memory");
    }
    this.emit("consent", { granted });
    this.render();
  }

  /** Opens a new conversation, optionally with a prefilled or sent message. */
  startConversation(message?: string, opts: { send?: boolean } = {}) {
    this.open();
    void this.openConversation(null).then(() => {
      if (!message) return;
      if (opts.send) void this.send(String(message), null);
      else {
        this.draft = String(message).slice(0, 10000);
        this.lastSignature = "";
        this.render();
        this.focusComposer();
      }
    });
  }

  /** Opens the help tab, searching for query when given. */
  searchHelp(query = "") {
    this.open("help");
    this.helpQuery = String(query).slice(0, 200);
    void this.loadHelp(this.helpQuery);
  }

  /** Opens a help center article (https only) in a new tab. */
  openArticle(url: string) {
    try {
      const u = new URL(String(url), location.href);
      if (u.protocol === "https:" || u.hostname === "localhost")
        window.open(u.toString(), "_blank", "noopener");
    } catch {
      /* ignore */
    }
  }

  /** Forget the visitor (e.g. on logout): the next bootstrap is a stranger. */
  async reset() {
    await this.capture.stopCapture(true);
    this.store.clear();
    this.api.token = "";
    this.active = null;
    this.messages = [];
    this.profile = { name: "", email: "", phone: "" };
    this.pendingAttrs = {};
    this.outbox = [];
    this.realtime?.stop();
    if (this.preview) return;
    await this.bootstrap();
    this.startRealtime();
    this.view = "home";
    this.lastSignature = "";
    this.render();
    this.emit("reset", {});
  }

  startReplay() {
    if (!this.captureAllowed) return Promise.resolve();
    return this.capture.startReplay(this.active?.uuid ?? null);
  }

  /** init({ capture: false }) turns co-browsing and replay off on this page. */
  private get captureAllowed() {
    return this.opts.capture !== false;
  }

  stopCapture(revoke: boolean) {
    return this.capture.stopCapture(revoke);
  }

  // -- data ---------------------------------------------------------------------

  private async loadHelp(query: string) {
    const seq = ++this.helpSeq;
    this.helpLoading = true;
    try {
      const q = query.trim();
      const out = await this.api.call<HelpResult>(
        "GET",
        `/help/search?locale=${this.locale}${q ? `&q=${encodeURIComponent(q)}` : ""}`,
      );
      if (seq !== this.helpSeq) return;
      this.help = out;
    } catch (err) {
      if (seq !== this.helpSeq) return;
      this.log("help search failed", err);
      if (this.help === null)
        this.help = { available: false, help_center_url: null, items: [] };
    } finally {
      if (seq === this.helpSeq) this.helpLoading = false;
    }
    this.render();
  }

  private onHelpInput(value: string) {
    this.helpQuery = value;
    window.clearTimeout(this.helpTimer);
    this.helpTimer = window.setTimeout(
      () => void this.loadHelp(this.helpQuery),
      300,
    );
  }

  private async loadBooking() {
    this.bookingLoading = true;
    try {
      this.booking = await this.api.call<BookingLink>("GET", "/booking");
    } catch {
      this.booking = { available: false, url: null };
    } finally {
      this.bookingLoading = false;
    }
    this.render();
  }

  private async refresh() {
    if (this.preview) return;
    try {
      const list = await this.api.call<{ items: Conversation[] }>(
        "GET",
        "/conversations",
      );
      this.setConversations(list.items);
      if (this.active && this.view === "chat") await this.loadMessages(true);
      void this.flushOutbox();
      this.render();
    } catch (err) {
      await this.recover(err);
    }
  }

  /** An expired token renews through bootstrap (backend source of truth). */
  private async recover(err: unknown) {
    if (err instanceof ApiError && err.status === 401) {
      try {
        await this.bootstrap();
        this.render();
      } catch (e) {
        this.log("re-bootstrap failed", e);
      }
    } else this.log("request failed", err);
  }

  private async loadMessages(incremental: boolean) {
    if (!this.active) return;
    const known = this.messages.filter(
      (m) => !m.pending && !m.failed && !m.queued && !m.local,
    );
    const after =
      incremental && known.length
        ? `?after=${known[known.length - 1]!.uuid}`
        : "";
    const page = await this.api.call<{
      conversation: Conversation;
      items: Message[];
      has_more: boolean;
    }>("GET", `/conversations/${this.active.uuid}/messages${after}`);
    if (page.conversation?.uuid) this.active = page.conversation;
    if (!after) {
      const local = this.messages.filter(
        (m) => m.queued || m.failed || m.pending,
      );
      this.messages = [...page.items, ...local];
    } else {
      for (const m of page.items) {
        const i = this.messages.findIndex(
          (x) =>
            x.uuid === m.uuid || (m.client_id && x.client_id === m.client_id),
        );
        if (i >= 0) this.messages[i] = m;
        else {
          this.messages.push(m);
          if (m.direction === "out") this.onAgentMessage(m);
        }
      }
    }
    if (
      this.isOpen &&
      this.active.unread_count > 0 &&
      document.visibilityState !== "hidden"
    ) {
      await this.api
        .call("POST", `/conversations/${this.active.uuid}/read`)
        .catch(() => undefined);
      this.active.unread_count = 0;
      const c = this.conversations.find((x) => x.uuid === this.active!.uuid);
      if (c) c.unread_count = 0;
      this.setConversations(this.conversations);
    }
  }

  private onAgentMessage(m: Message) {
    this.emit("message", {
      conversation_uuid: this.active?.uuid,
      message_uuid: m.uuid,
      text: m.content,
      sender_type: m.sender_type ?? "user",
    });
    const background = document.visibilityState === "hidden" || !this.isOpen;
    if (!background) return;
    if (this.s?.features.sound) chime();
    if (this.s?.features.notifications)
      notify(m.sender_name || this.t("newMessage"), plain(m.content), () =>
        this.open(),
      );
  }

  private async openConversation(c: Conversation | null) {
    this.active = c;
    this.messages = [];
    this.view = "chat";
    this.error = "";
    this.ratingScore = 0;
    this.emojiOpen = false;
    if (c) this.intro = null;
    this.lastSignature = "";
    this.render();
    if (c) {
      try {
        await this.loadMessages(false);
      } catch (err) {
        await this.recover(err);
      }
      this.render();
    }
    this.focusComposer();
  }

  private go(view: View) {
    this.view = view;
    this.active = null;
    this.error = "";
    this.emojiOpen = false;
    this.emit("view", { view });
    this.render();
  }

  // -- pre-chat -------------------------------------------------------------

  private offline(): boolean {
    return !this.boot!.widget.online;
  }

  /**
   * The pre-chat questions are the request form (CF-198). The widget only
   * adds an e-mail field of its own while offline with "leave a message"
   * on, when the form does not already ask for one.
   */
  private needsOfflineEmail(): boolean {
    const s = this.s!;
    if (this.active) return false;
    const contact = this.boot!.visitor.contact;
    const form = new Set(this.formFields().map((f) => f.key));
    return (
      this.offline() &&
      s.offline.collect_email &&
      !contact?.email &&
      !form.has("email")
    );
  }

  private formFields(): FormField[] {
    const form = this.boot?.request_form;
    if (!form || this.active) return [];
    const contact = this.boot!.visitor.contact;
    return form.fields.filter((f) => {
      if (f.key === "message") return false;
      if (f.key === "name" && contact?.name) return false;
      if (f.key === "email" && contact?.email) return false;
      return this.conditionHolds(f.condition);
    });
  }

  private conditionHolds(c: FormCondition | null): boolean {
    if (!c) return true;
    const v = this.formValues[c.field];
    const set = Array.isArray(v)
      ? v.length > 0
      : v !== undefined && v !== null && v !== "";
    switch (c.operator) {
      case "is_set":
        return set;
      case "is_not_set":
        return !set;
      case "equals":
        return set && String(v) === String(c.value);
      case "not_equals":
        return !set || String(v) !== String(c.value);
      case "in":
        return (
          set &&
          Array.isArray(c.value) &&
          c.value.map(String).includes(String(v))
        );
    }
    return true;
  }

  private formPayload(): Record<string, unknown> {
    const out: Record<string, unknown> = {};
    for (const f of this.formFields()) {
      const v = this.formValues[f.key];
      if (v === undefined || v === "" || (Array.isArray(v) && !v.length))
        continue;
      out[f.key] = f.type === "number" || f.type === "money" ? Number(v) : v;
    }
    return out;
  }

  /** First problem of the new-conversation form, "" when it can be sent. */
  private preChatError(): string {
    const s = this.s!;
    const pc = s.prechat;
    if (
      this.needsOfflineEmail() &&
      (!this.profile.email.trim() || !validEmail(this.profile.email))
    )
      return this.t("invalidEmail");
    if (
      pc.enabled &&
      pc.consent.enabled &&
      pc.consent.required &&
      !this.consentChecked
    )
      return (
        pick(pc.consent.text, this.locale as Locale) || this.t("consentDefault")
      );
    const missing = this.formFields().find((f) => {
      if (!f.required) return false;
      const v = this.formValues[f.key];
      return (
        v === undefined ||
        v === "" ||
        v === false ||
        (Array.isArray(v) && !v.length)
      );
    });
    if (missing) return `${missing.label} (${this.t("required")})`;
    return "";
  }

  // -- sending --------------------------------------------------------------

  private async send(
    text: string,
    file: File | null,
    retryOf?: Message,
    intent = "",
  ) {
    const s = this.s!;
    text = text.trim();
    if (!text && !file) return;
    if (file) {
      if (
        file.size >
        Math.min(this.boot!.limits.max_file_bytes, s.attachments.max_mb << 20)
      ) {
        this.error = this.t("fileTooLarge");
        this.render();
        return;
      }
      const kind = attachmentKind(file.type, file.name);
      if (!kind || !(s.attachments.types as string[]).includes(kind)) {
        this.error = this.t("fileType");
        this.render();
        return;
      }
    }
    if (!this.active && !retryOf) {
      const problem = this.preChatError();
      if (problem) {
        this.error = problem;
        this.draft = text;
        this.render();
        return;
      }
    }
    if (this.preview) {
      this.messages.push({
        uuid: uuid(),
        direction: "in",
        sender_name: "",
        kind: "text",
        content: text,
        status: "sent",
        client_id: null,
        attachments: [],
        created_at: new Date().toISOString(),
        read_at: null,
      });
      this.draft = "";
      this.render();
      return;
    }
    if (s.features.notifications) askPermission();
    this.error = "";
    const clientId = retryOf?.client_id ?? uuid();
    const optimistic: Message = retryOf ?? {
      uuid: clientId,
      direction: "in",
      sender_name: "",
      kind: file && !text ? "media" : "text",
      content: text,
      status: "pending",
      client_id: clientId,
      attachments: file
        ? [
            {
              uuid: clientId,
              filename: file.name,
              content_type: file.type,
              size_bytes: file.size,
              url: "",
            },
          ]
        : [],
      created_at: new Date().toISOString(),
      read_at: null,
    };
    optimistic.pending = true;
    optimistic.failed = false;
    optimistic.queued = false;
    if (!retryOf) this.messages.push(optimistic);
    this.draft = "";
    this.render();
    if (navigator.onLine === false) {
      this.queue({ text, file, message: optimistic, intent });
      return;
    }
    await this.deliver({ text, file, message: optimistic, intent });
  }

  private queue(item: QueuedSend) {
    item.message.pending = false;
    item.message.queued = true;
    if (!this.outbox.includes(item)) this.outbox.push(item);
    this.notice = this.t("networkOffline");
    this.render();
  }

  private async flushOutbox() {
    if (!this.outbox.length || navigator.onLine === false) return;
    const items = this.outbox.splice(0);
    for (const item of items) {
      item.message.queued = false;
      item.message.pending = true;
      this.render();
      await this.deliver(item);
    }
  }

  private async deliver(item: QueuedSend) {
    const { text, file, message: optimistic, intent } = item;
    const clientId = optimistic.client_id!;
    const first = !this.active;
    const path = this.active
      ? `/conversations/${this.active.uuid}/messages`
      : "/conversations";
    const fields: Record<string, string> = {
      text,
      client_id: clientId,
      page_url: location.href.slice(0, 500),
    };
    if (intent) fields.intent = intent;
    let form: { form_uuid: string; values: Record<string, unknown> } | null =
      null;
    let consent: boolean | undefined;
    if (first) {
      if (this.boot?.request_form) {
        const values = this.formPayload();
        form = { form_uuid: this.boot.request_form.uuid, values };
        if (!this.profile.name && typeof values.name === "string")
          this.profile.name = values.name;
        if (!this.profile.email && typeof values.email === "string")
          this.profile.email = values.email;
      }
      if (this.profile.name) fields.name = this.profile.name;
      if (this.profile.email) fields.email = this.profile.email;
      if (this.profile.phone) fields.phone = this.profile.phone;
      if (this.s!.prechat.enabled && this.s!.prechat.consent.enabled)
        consent = this.consentChecked;
    }
    const attributes = Object.keys(this.pendingAttrs).length
      ? { ...this.pendingAttrs }
      : undefined;
    const events = [...this.events];
    if (first && this.intro?.trigger)
      events.push({
        name: `proactive:${this.intro.trigger.id}`,
        at: new Date().toISOString(),
      });
    try {
      let body: unknown;
      if (file) {
        const fd = new FormData();
        for (const [k, v] of Object.entries(fields)) fd.append(k, v);
        if (form) fd.append("form", JSON.stringify(form));
        if (consent !== undefined) fd.append("consent", String(consent));
        if (attributes) fd.append("attributes", JSON.stringify(attributes));
        fd.append("file", file, file.name);
        body = fd;
      } else {
        body = {
          ...fields,
          form: form ?? undefined,
          consent,
          attributes,
          events: events.length ? events : undefined,
        };
      }
      const sent = await this.api.call<{
        conversation_uuid: string;
        message: Message;
        created: boolean;
      }>("POST", path, body);
      this.events = [];
      if (attributes)
        for (const k of Object.keys(attributes)) delete this.pendingAttrs[k];
      if (form) this.formValues = {};
      const i = this.messages.indexOf(optimistic);
      if (i >= 0) this.messages[i] = sent.message;
      if (!this.active || this.active.uuid !== sent.conversation_uuid) {
        const list = await this.api.call<{ items: Conversation[] }>(
          "GET",
          "/conversations",
        );
        this.setConversations(list.items);
        this.active =
          list.items.find((c) => c.uuid === sent.conversation_uuid) ??
          this.active;
        if (sent.created)
          this.emit("conversation", {
            conversation_uuid: sent.conversation_uuid,
          });
      }
      if (
        this.boot &&
        !this.boot.visitor.contact &&
        (this.profile.name || this.profile.email)
      ) {
        this.boot.visitor.contact = {
          name: this.profile.name,
          email: this.profile.email || null,
          phone: this.profile.phone || null,
        };
      }
      this.notice = "";
      this.emit("message:sent", {
        conversation_uuid: sent.conversation_uuid,
        message_uuid: sent.message.uuid,
      });
    } catch (err) {
      optimistic.pending = false;
      if (isNetworkError(err)) {
        this.queue(item);
        return;
      }
      optimistic.failed = true;
      if (err instanceof ApiError) {
        const code = err.code;
        if (code === "WIDGET_IDENTITY_REQUIRED")
          this.error = this.t("identityRequired");
        else if (code === "WIDGET_FILE_TOO_LARGE")
          this.error = this.t("fileTooLarge");
        else if (code === "WIDGET_FILE_TYPE") this.error = this.t("fileType");
        else if (
          err.status === 422 ||
          code === "WIDGET_CONSENT_REQUIRED" ||
          (err.status === 400 && first)
        ) {
          this.messages = this.messages.filter((m) => m !== optimistic);
          this.draft = text;
          this.error =
            code === "WIDGET_CONSENT_REQUIRED"
              ? this.t("consentDefault")
              : this.t("formInvalid");
        } else if (code === "WIDGET_CONVERSATION_CLOSED") {
          this.messages = this.messages.filter((m) => m !== optimistic);
          await this.refresh();
        } else this.error = this.t("error");
        if (err.status === 401) await this.recover(err);
        this.emit("error", { code, message: err.message });
      } else this.error = this.t("error");
    }
    this.render();
  }

  private sendTyping(on: boolean) {
    if (!this.active || !this.s?.features.typing || this.preview) return;
    const now = Date.now();
    if (on && now - this.lastTypingSent < 3000) return;
    this.lastTypingSent = on ? now : 0;
    void this.api
      .call("POST", `/conversations/${this.active.uuid}/typing`, { typing: on })
      .catch(() => undefined);
  }

  private async rate(score: number, comment: string) {
    if (!this.active) return;
    if (this.preview) {
      this.active.rating = score;
      this.active.can_rate = false;
      this.render();
      return;
    }
    try {
      const r = await this.api.call<{ score: number; existing: boolean }>(
        "POST",
        `/conversations/${this.active.uuid}/rating`,
        { score, comment },
      );
      this.active.rating = r.score;
      this.active.can_rate = false;
      this.emit("rating", {
        conversation_uuid: this.active.uuid,
        score: r.score,
        existing: r.existing,
      });
    } catch (err) {
      this.error = this.t("error");
      await this.recover(err);
    }
    this.render();
  }

  // -- realtime and polling ---------------------------------------------------

  private startRealtime() {
    if (
      !this.boot?.realtime.enabled ||
      typeof WebSocket === "undefined" ||
      this.preview
    )
      return;
    this.realtime = new Realtime(
      () => this.api.call<RealtimeTokens>("POST", "/realtime-token"),
      (data) => this.onRealtime(data),
      (connected) => {
        this.log("realtime", connected ? "connected" : "disconnected");
        if (connected) void this.refresh();
        this.schedulePoll();
      },
    );
    this.realtime.start();
  }

  private onRealtime(data: unknown) {
    const ev = data as {
      type?: string;
      conversation_uuid?: string;
      typing?: boolean;
      session?: ReplaySession;
    };
    if (
      ev.type === "cobrowse.requested" &&
      ev.session &&
      ev.conversation_uuid
    ) {
      if (!this.captureAllowed) return;
      const conversation = ev.conversation_uuid;
      const session = ev.session;
      void this.api
        .call<ReplayPolicy>("GET", "/replay-policy")
        .then((policy) => {
          if (policy.cobrowse_enabled)
            this.capture.askCapture(conversation, session, policy);
        })
        .catch(() => {});
      return;
    }
    if (ev.type === "typing") {
      if (!this.s?.features.typing) return;
      this.agentTyping = Boolean(ev.typing);
      window.clearTimeout(this.typingTimer);
      if (this.agentTyping)
        this.typingTimer = window.setTimeout(
          () => ((this.agentTyping = false), this.render()),
          8000,
        );
      this.render();
      return;
    }
    if (ev.type === "message.created" || ev.type === "conversation.updated") {
      this.agentTyping = false;
      void this.refresh();
    }
  }

  private schedulePoll() {
    if (this.preview) return;
    window.clearTimeout(this.pollTimer);
    const live = this.realtime?.connected;
    const inChat = this.isOpen && this.view === "chat" && this.active;
    const delay = inChat
      ? live
        ? 30000
        : 4000
      : live
        ? 120000
        : this.isOpen
          ? 10000
          : 30000;
    this.pollTimer = window.setTimeout(async () => {
      if (document.visibilityState !== "hidden") await this.refresh();
      this.schedulePoll();
    }, delay);
  }

  private schedulePollSoon() {
    if (this.pollPending || this.preview) return;
    this.pollPending = true;
    window.setTimeout(() => {
      this.pollPending = false;
      this.schedulePoll();
    }, 0);
  }

  // -- preview ----------------------------------------------------------------

  private applyPreview() {
    const p = this.preview!;
    switch (p.view) {
      case "closed":
        break;
      case "proactive": {
        const t = this.s!.triggers[0];
        this.showMessage(
          t ? pick(t.message, this.locale as Locale) : this.t("text"),
          { trigger: t },
        );
        break;
      }
      case "consent":
        this.consentGiven = false;
        this.s!.privacy = { ...this.s!.privacy, consent_required: true };
        this.isOpen = true;
        break;
      case "messages":
      case "help":
        this.isOpen = true;
        this.view = p.view;
        if (p.view === "help") void this.loadHelp("");
        break;
      case "chat":
      case "csat":
        this.isOpen = true;
        void this.openConversation(this.conversations[0] ?? null);
        break;
      case "prechat":
        this.isOpen = true;
        void this.openConversation(null);
        break;
      case "callback":
        this.isOpen = true;
        this.view = "callback";
        break;
      default:
        this.isOpen = true;
    }
    if (p.view !== "consent") this.consentGiven = true;
    this.render();
  }

  // -- rendering --------------------------------------------------------------

  private mount() {
    if (this.host) return;
    if (!document.body) {
      document.addEventListener("DOMContentLoaded", () => this.mount(), {
        once: true,
      });
      return;
    }
    this.host = el("div", {
      id: "clickfix-widget",
      "data-sdk-version": SDK_VERSION,
    });
    this.shadow = this.host.attachShadow({ mode: "open" });
    this.styleEl = el("style");
    this.rootEl = el("div", { class: "root" });
    this.panelEl = el("div", {
      class: "panel",
      role: "dialog",
      "aria-modal": "false",
      hidden: true,
    });
    this.bubbleEl = el("div", { hidden: true });
    this.launcherEl = el("button", {
      class: "launcher",
      type: "button",
      onclick: () => this.toggle(),
    });
    this.rootEl.append(this.panelEl, this.bubbleEl, this.launcherEl);
    this.shadow.append(this.styleEl, this.rootEl);
    this.rootEl.addEventListener("keydown", (e) => {
      if ((e as KeyboardEvent).key === "Escape" && this.isOpen) {
        e.stopPropagation();
        this.close();
      }
    });
    document.body.append(this.host);
    this.bindCustomLaunchers();
    this.restyle();
  }

  private bindCustomLaunchers() {
    const selector = this.opts.customLauncherSelector;
    const onClick = (e: Event) => {
      const target = e.target as Element | null;
      const hit =
        target?.closest?.("[data-clickfix-launcher]") ??
        (selector ? target?.closest?.(selector) : null);
      if (hit) {
        e.preventDefault();
        this.toggle();
      }
    };
    document.addEventListener("click", onClick);
    this.unbind.push(() => document.removeEventListener("click", onClick));
    this.syncCustomLaunchers();
  }

  private syncCustomLaunchers() {
    const sel = ["[data-clickfix-launcher]", this.opts.customLauncherSelector]
      .filter(Boolean)
      .join(",");
    try {
      document.querySelectorAll(sel).forEach((n) => {
        n.setAttribute("data-clickfix-unread", String(this.unread));
        n.setAttribute("aria-expanded", String(this.isOpen));
      });
    } catch {
      /* invalid selector */
    }
  }

  /** Rebuilds the theme (settings or overrides changed). */
  private restyle() {
    if (!this.rootEl || !this.styleEl || !this.s) return;
    const s = this.s;
    const custom = sanitizeCSS(s.appearance.custom_css).css;
    this.styleEl.textContent =
      CSS + (custom ? `\n/* custom */\n${custom}` : "");
    for (const [k, v] of Object.entries(themeVars(s, this.opts.zIndex)))
      this.rootEl.style.setProperty(k, v);
    this.rootEl.setAttribute("dir", this.dir);
    this.rootEl.setAttribute("lang", this.locale);
    this.lastSignature = "";
    this.render();
  }

  render() {
    if (
      !this.panelEl ||
      !this.launcherEl ||
      !this.rootEl ||
      !this.s ||
      !this.boot
    )
      return;
    this.schedulePollSoon();
    const s = this.s;
    const hidden = !this.shown || Boolean(this.hiddenBy && !this.preview);
    this.rootEl.hidden = hidden;
    this.rootEl.className = [
      "root",
      s.position,
      `theme-${s.appearance.theme}`,
      s.mobile.fullscreen ? "fullscreen" : "",
      this.isOpen ? "open" : "",
      this.launcherHidden ? "no-launcher" : "",
      this.preview ? "preview-frame" : "",
    ]
      .filter(Boolean)
      .join(" ");
    this.renderLauncher();
    this.renderBubble();
    this.syncCustomLaunchers();
    this.panelEl.hidden = !this.isOpen;
    if (!this.isOpen) {
      this.lastSignature = "";
      return;
    }
    const signature = JSON.stringify([
      this.view,
      this.aMessages,
      this.aBusy,
      this.aError,
      this.active,
      this.messages,
      this.conversations,
      this.agentTyping,
      this.error,
      this.notice,
      this.ratingScore,
      this.boot.widget.online,
      this.boot.visitor.contact,
      this.help,
      this.helpLoading,
      this.booking,
      this.emojiOpen,
      this.consentGiven,
      this.consentChecked,
      this.intro?.text,
      this.locale,
      this.replayPolicy?.replay_enabled,
    ]);
    if (signature === this.lastSignature) return;
    this.lastSignature = signature;
    const ta = this.shadow?.querySelector(
      "textarea.compose",
    ) as HTMLTextAreaElement | null;
    const hadFocus = ta !== null && this.shadow?.activeElement === ta;
    if (ta) this.draft = ta.value;
    const hs = this.shadow?.querySelector(
      "input.help-input",
    ) as HTMLInputElement | null;
    const helpFocus = hs !== null && this.shadow?.activeElement === hs;
    const aTa = this.shadow?.querySelector(
      "textarea.a-compose",
    ) as HTMLTextAreaElement | null;
    const aFocus = aTa !== null && this.shadow?.activeElement === aTa;
    const titleId = "cf-title";
    this.panelEl.setAttribute("aria-labelledby", titleId);
    const needsConsent = s.privacy.consent_required && !this.consentGiven;
    let parts: Child[];
    if (needsConsent)
      parts = [this.header(this.boot.widget.name), this.consentView()];
    else if (this.view === "chat") parts = [this.chatHeader(), ...this.chat()];
    else if (this.view === "callback")
      parts = [
        this.header(this.t("channelCallback"), true),
        this.callbackView(),
      ];
    else if (this.view === "messages")
      parts = [
        this.header(this.t("tabMessages")),
        this.messagesView(),
        this.tabs(),
      ];
    else if (this.view === "help")
      parts = [this.header(this.t("tabHelp")), this.helpView(), this.tabs()];
    else if (this.view === "assistant" && this.assistant)
      parts = [
        this.header(this.t("tabAssistant")),
        this.assistantView(),
        this.assistantComposer(),
        this.tabs(),
      ];
    else parts = [this.homeView(), this.tabs()];
    this.panelEl.replaceChildren(...(parts.filter(Boolean) as Node[]));
    if (this.view === "chat" && !needsConsent) {
      const body = this.panelEl.querySelector(".body");
      if (body) body.scrollTop = body.scrollHeight;
      if (hadFocus) this.focusComposer();
    } else if (this.view === "assistant") {
      const body = this.panelEl.querySelector(".body");
      if (body) body.scrollTop = body.scrollHeight;
      if (aFocus)
        (
          this.panelEl.querySelector(
            "textarea.a-compose",
          ) as HTMLTextAreaElement | null
        )?.focus();
    } else {
      if (this.help === null && !this.helpLoading) void this.loadHelp("");
      if (this.booking === null && !this.bookingLoading)
        void this.loadBooking();
      if (helpFocus) {
        const next = this.panelEl.querySelector(
          "input.help-input",
        ) as HTMLInputElement | null;
        if (next) {
          next.focus();
          next.selectionStart = next.selectionEnd = next.value.length;
        }
      }
    }
  }

  private renderLauncher() {
    const s = this.s!;
    const l = s.appearance.launcher;
    const label = pick(l.label, this.locale as Locale);
    const b = this.launcherEl!;
    b.className = `launcher ${l.shape}${label && !this.isOpen ? " labelled" : ""}`;
    b.hidden = this.launcherHidden;
    b.setAttribute(
      "aria-label",
      this.isOpen ? this.t("close") : label || this.t("launcher"),
    );
    b.setAttribute("aria-expanded", String(this.isOpen));
    const glyph = this.isOpen
      ? icon("close")
      : l.icon === "custom" && l.image_url
        ? el("img", { src: l.image_url, alt: "" })
        : icon((l.icon === "custom" ? "chat" : l.icon) as IconName);
    b.replaceChildren(glyph);
    if (label && !this.isOpen)
      b.append(el("span", { class: "label", text: label }));
    if (this.unread > 0 && !this.isOpen)
      b.append(
        el("span", {
          class: "badge",
          "aria-hidden": "true",
          text: this.unread > 9 ? "9+" : String(this.unread),
        }),
      );
  }

  private renderBubble() {
    const p = this.proactive;
    const box = this.bubbleEl!;
    if (!p || this.isOpen) {
      box.hidden = true;
      box.replaceChildren();
      return;
    }
    box.hidden = false;
    box.className = "proactive";
    box.setAttribute("role", "status");
    box.replaceChildren(
      el("div", { class: "who" }, [
        this.avatarStack(1),
        el("span", { text: p.sender }),
      ]),
      el("div", { class: "text" }, [
        renderText(p.text, this.s!.features.markdown),
      ]),
      el(
        "button",
        {
          class: "x",
          type: "button",
          "aria-label": this.t("dismiss"),
          onclick: (e: Event) => {
            e.stopPropagation();
            this.proactive = null;
            this.render();
          },
        },
        [icon("close")],
      ),
    );
    box.onclick = () => {
      this.openProactive();
    };
  }

  private focusComposer() {
    window.setTimeout(() => {
      const ta = this.shadow?.querySelector(
        "textarea.compose",
      ) as HTMLTextAreaElement | null;
      if (!ta || this.preview) return;
      ta.focus();
      ta.selectionStart = ta.selectionEnd = ta.value.length;
    }, 0);
  }

  private focusFirst() {
    if (this.preview) return;
    window.setTimeout(() => {
      const target = this.panelEl?.querySelector<HTMLElement>(
        "textarea.compose, input, button.primary, button, a",
      );
      target?.focus();
    }, 0);
  }

  // -- pieces -----------------------------------------------------------------

  private copy(
    key:
      | "greeting_title"
      | "greeting_text"
      | "team_intro"
      | "offline_message"
      | "away_message"
      | "placeholder",
  ): string {
    return settingsText(this.s! as never, this.locale as Locale, key);
  }

  private avatarStack(max = 3): HTMLElement | null {
    const team = this.boot!.widget.team;
    if (!this.s!.show_team || !team.length) return null;
    return el(
      "div",
      { class: "avatars", "aria-hidden": "true" },
      team
        .slice(0, max)
        .map((m) =>
          el("span", { class: "avatar", title: m.name }, [
            m.avatar_url
              ? el("img", { src: m.avatar_url, alt: "" })
              : m.initials || "•",
          ]),
        ),
    );
  }

  private statusLine(): string {
    const b = this.boot!;
    if (b.widget.online) {
      const v = b.widget.reply?.value;
      const key = (
        {
          minutes: "replyMinutes",
          hour: "replyHour",
          hours: "replyHours",
          day: "replyDay",
        } as const
      )[v as "minutes"];
      return key ? this.t(key) : this.t("online");
    }
    const next = b.widget.reply?.next_open_at;
    if (next) {
      try {
        const time = new Date(next).toLocaleString(
          this.locale === "tr" ? "tr-TR" : "en-US",
          {
            weekday: "short",
            hour: "2-digit",
            minute: "2-digit",
          },
        );
        return format(this.t("backAt"), { time });
      } catch {
        /* fall through */
      }
    }
    return this.s!.away_mode
      ? this.copy("away_message") || this.t("away")
      : this.t("offline");
  }

  private closeButton(): HTMLElement {
    return el(
      "button",
      {
        class: "icon-btn",
        type: "button",
        "aria-label": this.t("close"),
        onclick: () => this.close(),
      },
      [icon("close")],
    );
  }

  private header(title: string, back = false): HTMLElement {
    return el("div", { class: "header" }, [
      back
        ? el(
            "button",
            {
              class: "icon-btn",
              type: "button",
              "aria-label": this.t("back"),
              onclick: () => this.go("home"),
            },
            [icon("back")],
          )
        : null,
      el("div", { class: "grow" }, [el("h2", { id: "cf-title", text: title })]),
      this.closeButton(),
    ]);
  }

  private chatHeader(): HTMLElement {
    const b = this.boot!;
    const logo = this.s!.appearance.logo_url;
    return el("div", { class: "header" }, [
      el(
        "button",
        {
          class: "icon-btn",
          type: "button",
          "aria-label": this.t("back"),
          onclick: () => {
            this.intro = null;
            this.go(
              this.s!.features.history && this.conversations.length
                ? "messages"
                : "home",
            );
            void this.refresh();
          },
        },
        [icon("back")],
      ),
      logo
        ? el("img", { class: "logo", src: logo, alt: "" })
        : this.avatarStack(),
      el("div", { class: "grow" }, [
        el("h2", { id: "cf-title", text: b.widget.name }),
        el("p", { class: "sub" }, [
          el("span", { class: `status-dot${b.widget.online ? "" : " off"}` }),
          el("span", {
            class: "ellipsis",
            text: this.agentTyping
              ? `${this.t("team")} ${this.t("typing")}`
              : this.statusLine(),
          }),
        ]),
      ]),
      this.closeButton(),
    ]);
  }

  private consentView(): HTMLElement {
    const p = this.s!.privacy;
    const text =
      pick(p.consent_text, this.locale as Locale) || this.t("consentDefault");
    return el("div", { class: "body" }, [
      el("div", { class: "consent" }, [
        el("h3", { text: this.t("consentTitle") }),
        el("p", { text }),
        el("button", {
          class: "primary",
          type: "button",
          text: this.t("consentAccept"),
          onclick: () => this.consent(true),
        }),
        p.privacy_url
          ? el(
              "a",
              {
                class: "link-strong",
                href: p.privacy_url,
                target: "_blank",
                rel: "noopener noreferrer",
              },
              [this.t("privacy"), icon("external")],
            )
          : null,
      ]),
    ]);
  }

  private tabs(): HTMLElement | null {
    const s = this.s!;
    const helpOn =
      Boolean(this.help?.available) &&
      s.home.cards.some(
        (c) => c.enabled && (c.type === "help_search" || c.type === "articles"),
      );
    const list: [View, StringKey, IconName][] = [["home", "tabHome", "home"]];
    if (s.features.history && s.channels.chat)
      list.push(["messages", "tabMessages", "messages"]);
    if (helpOn) list.push(["help", "tabHelp", "help"]);
    if (this.assistant) list.push(["assistant", "tabAssistant", "sparkles"]);
    if (list.length < 2) return null;
    return el(
      "div",
      { class: "tabs", role: "tablist" },
      list.map(([view, label, ic]) =>
        el(
          "button",
          {
            class: "tab",
            type: "button",
            role: "tab",
            "aria-selected": String(this.view === view),
            onclick: () => this.go(view),
          },
          [
            icon(ic),
            el("span", { text: this.t(label) }),
            view === "messages" && this.unread > 0
              ? el("span", { class: "count", text: String(this.unread) })
              : null,
          ],
        ),
      ),
    );
  }

  private homeView(): HTMLElement {
    const s = this.s!;
    const logo = s.appearance.logo_url;
    const hero = el("div", { class: "hero" }, [
      el("div", { class: "top" }, [
        logo
          ? el("img", {
              class: "logo",
              src: logo,
              alt: "",
              style: "height:28px;max-width:140px;object-fit:contain",
            })
          : el("span"),
        el("div", { style: "display:flex;align-items:center;gap:8px" }, [
          this.avatarStack(),
          this.closeButton(),
        ]),
      ]),
      el("h2", {
        id: "cf-title",
        text: this.copy("greeting_title") || this.t("title"),
      }),
      el("p", { text: this.copy("greeting_text") || this.t("text") }),
      this.copy("team_intro")
        ? el("p", { style: "font-size:13px", text: this.copy("team_intro") })
        : null,
    ]);
    const cards = s.home.cards
      .filter((c) => c.enabled)
      .map((c) => this.homeCard(c));
    return el("div", { class: "body home" }, [
      hero,
      el("div", { class: "cards" }, [
        this.notice
          ? el("div", { class: "notice", role: "status", text: this.notice })
          : null,
        ...cards,
        s.appearance.show_branding
          ? el("div", { class: "powered", text: this.t("poweredBy") })
          : null,
      ]),
    ]);
  }

  private cardTitle(c: HomeCard, fallback: string) {
    return pick(c.title, this.locale as Locale) || fallback;
  }

  private homeCard(c: HomeCard): HTMLElement | null {
    const s = this.s!;
    const desc = pick(c.description, this.locale as Locale);
    switch (c.type) {
      case "conversation": {
        if (!s.channels.chat) return null;
        const offline = this.offline();
        const blocked = offline && s.offline.mode === "message";
        const recent = this.conversations.find((x) => x.open);
        return el("div", { class: "card" }, [
          recent
            ? el(
                "button",
                {
                  class: "list-item",
                  type: "button",
                  onclick: () => void this.openConversation(recent),
                },
                [
                  this.avatarStack(1),
                  el("div", { class: "grow" }, [
                    el("div", {
                      class: "ellipsis",
                      text:
                        plain(recent.last_message_preview) ||
                        `#${recent.number}`,
                    }),
                    el("div", {
                      class: "muted",
                      text: this.t("yourConversations"),
                    }),
                  ]),
                  recent.unread_count
                    ? el("span", {
                        class: "dot",
                        "aria-label": String(recent.unread_count),
                      })
                    : null,
                ],
              )
            : null,
          blocked
            ? el("div", {
                class: "notice",
                text: this.copy("offline_message") || this.t("offline"),
              })
            : el(
                "button",
                {
                  class: "card-btn",
                  type: "button",
                  onclick: () => void this.openConversation(null),
                },
                [
                  el("div", { class: "grow" }, [
                    el("h3", {
                      text: this.cardTitle(c, this.t("newConversation")),
                    }),
                    el("p", {
                      class: "muted",
                      text: desc || this.statusLine(),
                    }),
                  ]),
                  el("span", { class: "go" }, [icon("send")]),
                ],
              ),
        ]);
      }
      case "help_search": {
        if (!this.help?.available) return null;
        return el("div", { class: "card", role: "search" }, [
          el("label", { class: "search" }, [
            icon("search"),
            el("span", {
              class: "sr-only",
              text: this.cardTitle(c, this.t("helpTitle")),
            }),
            el("input", {
              class: "help-input",
              type: "search",
              maxlength: "200",
              autocomplete: "off",
              placeholder: this.cardTitle(c, this.t("helpTitle")),
              onkeydown: (e: Event) => {
                const k = e as KeyboardEvent;
                if (k.key === "Enter")
                  this.searchHelp((k.target as HTMLInputElement).value);
              },
            }),
          ]),
        ]);
      }
      case "articles": {
        const items = this.help?.available ? this.help.items.slice(0, 4) : [];
        if (!items.length) return null;
        return el("div", { class: "card" }, [
          el("h3", { text: this.cardTitle(c, this.t("helpPopular")) }),
          ...items.map((a) =>
            el(
              "a",
              {
                class: "list-item article",
                href: a.url,
                target: "_blank",
                rel: "noopener noreferrer",
              },
              [
                el("div", { class: "grow" }, [el("strong", { text: a.title })]),
                icon("arrow"),
                el("span", { class: "sr-only", text: this.t("helpOpens") }),
              ],
            ),
          ),
        ]);
      }
      case "status":
      case "link":
        return el("div", { class: "card" }, [
          el(
            "a",
            {
              class: "card-btn",
              href: c.url,
              target: "_blank",
              rel: "noopener noreferrer",
            },
            [
              el("span", { class: "go" }, [
                icon(c.type === "status" ? "status" : "link"),
              ]),
              el("div", { class: "grow" }, [
                el("h3", {
                  text: this.cardTitle(
                    c,
                    c.type === "status" ? this.t("statusTitle") : c.url,
                  ),
                }),
                desc || c.type === "status"
                  ? el("p", {
                      class: "muted",
                      text: desc || this.t("statusAction"),
                    })
                  : null,
              ]),
              icon("external"),
              el("span", { class: "sr-only", text: this.t("helpOpens") }),
            ],
          ),
        ]);
      case "booking": {
        const b = this.booking;
        if (!b?.available || !b.url) return null;
        return el("div", { class: "card" }, [
          el(
            "a",
            {
              class: "card-btn",
              href: b.url,
              target: "_blank",
              rel: "noopener noreferrer",
            },
            [
              el("span", { class: "go" }, [icon("calendar")]),
              el("div", { class: "grow" }, [
                el("h3", { text: this.cardTitle(c, this.t("bookTitle")) }),
                el("p", { class: "muted", text: desc || this.t("bookAction") }),
              ]),
              icon("external"),
            ],
          ),
        ]);
      }
      case "channels": {
        const links = this.channelLinks();
        if (!links.length) return null;
        return el("div", { class: "card" }, [
          el("h3", { text: this.cardTitle(c, this.t("channelsTitle")) }),
          desc ? el("p", { class: "muted", text: desc }) : null,
          el("div", { class: "channels" }, links),
        ]);
      }
    }
    return null;
  }

  private channelLinks(): HTMLElement[] {
    const ch = this.s!.channels;
    const out: HTMLElement[] = [];
    const link = (href: string, label: StringKey, ic: IconName) =>
      out.push(
        el(
          "a",
          {
            class: "channel",
            href,
            target: "_blank",
            rel: "noopener noreferrer",
          },
          [icon(ic), this.t(label)],
        ),
      );
    if (ch.whatsapp)
      link(`https://wa.me/${ch.whatsapp}`, "channelWhatsapp", "whatsapp");
    if (ch.instagram)
      link(
        `https://ig.me/m/${encodeURIComponent(ch.instagram)}`,
        "channelInstagram",
        "instagram",
      );
    if (ch.telegram)
      link(
        `https://t.me/${encodeURIComponent(ch.telegram)}`,
        "channelTelegram",
        "telegram",
      );
    if (ch.messenger)
      link(
        `https://m.me/${encodeURIComponent(ch.messenger)}`,
        "channelMessenger",
        "messages",
      );
    if (ch.email)
      out.push(
        el("a", { class: "channel", href: `mailto:${ch.email}` }, [
          icon("mail"),
          this.t("channelEmail"),
        ]),
      );
    if (ch.phone)
      out.push(
        el("a", { class: "channel", href: `tel:${ch.phone}` }, [
          icon("phone"),
          this.t("channelPhone"),
        ]),
      );
    if (ch.callback)
      out.push(
        el(
          "button",
          {
            class: "channel",
            type: "button",
            onclick: () => this.go("callback"),
          },
          [icon("phone"), this.t("channelCallback")],
        ),
      );
    if (ch.video)
      out.push(
        el(
          "button",
          {
            class: "channel",
            type: "button",
            onclick: () => this.startRequest("video"),
          },
          [icon("video"), this.t("channelVideo")],
        ),
      );
    return out;
  }

  private startRequest(kind: "video" | "callback", phone = "") {
    const text =
      kind === "video"
        ? this.t("videoMessage")
        : format(this.t("callbackMessage"), { phone });
    if (phone) this.profile.phone = phone;
    void this.openConversation(null).then(() =>
      this.send(text, null, undefined, kind),
    );
  }

  private callbackView(): HTMLElement {
    const input = el("input", {
      type: "tel",
      autocomplete: "tel",
      maxlength: "30",
      value: this.profile.phone,
    });
    return el("div", { class: "body pad" }, [
      el("div", { class: "card" }, [
        el("label", { class: "field" }, [
          el("span", { text: this.t("callbackPrompt") }),
          input,
        ]),
        this.error
          ? el("p", { class: "error", role: "alert", text: this.error })
          : null,
        el("button", {
          class: "primary",
          type: "button",
          text: this.t("callbackSend"),
          onclick: () => {
            const v = input.value.trim();
            if (v.replace(/\D/g, "").length < 6) {
              this.error = `${this.t("phone")} (${this.t("required")})`;
              this.render();
              return;
            }
            this.error = "";
            this.startRequest("callback", v);
          },
        }),
      ]),
    ]);
  }

  private messagesView(): HTMLElement {
    const list = this.conversations;
    return el("div", { class: "body pad" }, [
      el("div", { class: "card" }, [
        list.length ? null : el("p", { class: "muted", text: this.t("empty") }),
        ...list.map((c) =>
          el(
            "button",
            {
              class: "list-item",
              type: "button",
              onclick: () => void this.openConversation(c),
            },
            [
              this.avatarStack(1),
              el("div", { class: "grow" }, [
                el("div", {
                  class: "ellipsis",
                  text: plain(c.last_message_preview) || `#${c.number}`,
                }),
                el("div", {
                  class: "muted",
                  text: `#${c.number} · ${this.time(c.last_message_at)}`,
                }),
              ]),
              c.unread_count > 0
                ? el("span", {
                    class: "dot",
                    "aria-label": String(c.unread_count),
                  })
                : null,
            ],
          ),
        ),
      ]),
      this.s!.channels.chat &&
      !(this.offline() && this.s!.offline.mode === "message")
        ? el(
            "button",
            {
              class: "primary",
              type: "button",
              onclick: () => void this.openConversation(null),
            },
            [this.t("newConversation"), icon("send")],
          )
        : null,
    ]);
  }

  // -- assistant (CF-193) ----------------------------------------------------

  private assistantView(): HTMLElement {
    const items: Child[] = [
      el("div", { class: "card" }, [
        el("p", { class: "muted", text: this.t("assistantIntro") }),
      ]),
    ];
    for (const m of this.aMessages) {
      if (m.role === "tool") continue;
      const mine = m.role === "user";
      const bubble = el("div", { class: "msg" });
      if (m.content) bubble.append(renderText(m.content, !mine));
      items.push(el("div", { class: `row ${mine ? "in" : "out"}` }, [bubble]));
      for (const a of m.actions ?? []) items.push(this.assistantCard(a));
    }
    if (this.aBusy)
      items.push(
        el("p", {
          class: "muted",
          "aria-live": "polite",
          text: this.t("assistantThinking"),
        }),
      );
    const last = [...this.aMessages].reverse().find((m) => m.role !== "user");
    if (!this.aBusy && last?.suggestions?.length)
      items.push(
        el(
          "div",
          { class: "chips" },
          last.suggestions.slice(0, 4).map((q) =>
            el("button", {
              class: "chip",
              type: "button",
              text: q,
              onclick: () => void this.assistantSend(q),
            }),
          ),
        ),
      );
    items.push(
      el(
        "button",
        {
          class: "secondary a-human",
          type: "button",
          onclick: () => this.startConversation(),
        },
        [icon("headset"), el("span", { text: this.t("assistantHuman") })],
      ),
    );
    return el(
      "div",
      { class: "body pad a-body", "aria-live": "polite" },
      items,
    );
  }

  private assistantCard(a: AssistantAction): HTMLElement {
    const href = a.status === "done" ? assistantHref(a.focus) : null;
    const state: Record<string, StringKey> = {
      done: "assistantDone",
      rejected: "assistantRejected",
      failed: "assistantFailed",
    };
    return el("div", { class: "card a-card" }, [
      el("h3", { text: a.title }),
      a.summary ? el("p", { class: "muted", text: a.summary }) : null,
      a.verification?.message
        ? el("p", { class: "muted", text: a.verification.message })
        : null,
      a.status === "proposed"
        ? el("div", { class: "chips" }, [
            el("button", {
              class: "secondary",
              type: "button",
              text: this.t("assistantConfirm"),
              disabled: this.aBusy,
              onclick: () => void this.assistantAct(a, true),
            }),
            el("button", {
              class: "secondary",
              type: "button",
              text: this.t("assistantReject"),
              disabled: this.aBusy,
              onclick: () => void this.assistantAct(a, false),
            }),
          ])
        : state[a.status]
          ? el("p", { class: "muted", text: this.t(state[a.status]!) })
          : null,
      href
        ? el("a", { class: "card-btn go", href }, [
            el("span", { class: "grow", text: this.t("assistantOpenPage") }),
            icon("arrow"),
          ])
        : null,
    ]);
  }

  private assistantComposer(): HTMLElement {
    const placeholder = this.t("assistantPlaceholder");
    const ta = el("textarea", {
      class: "compose a-compose",
      rows: 1,
      placeholder,
      "aria-label": placeholder,
      maxlength: 4000,
    });
    ta.value = this.aDraft;
    ta.addEventListener("input", () => {
      this.aDraft = ta.value;
    });
    ta.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
        e.preventDefault();
        void this.assistantSend(ta.value);
      }
    });
    return el("div", { class: "footer" }, [
      this.aError
        ? el("p", { class: "error", role: "alert", text: this.aError })
        : null,
      el("div", { class: "composer" }, [
        ta,
        el(
          "button",
          {
            class: "icon-btn send",
            type: "button",
            "aria-label": this.t("send"),
            onclick: () => void this.assistantSend(ta.value),
          },
          [icon("send")],
        ),
      ]),
    ]);
  }

  private async assistantSend(text: string) {
    const content = text.trim();
    if (!content || this.aBusy || !this.assistant) return;
    this.aDraft = "";
    this.aError = "";
    this.aBusy = true;
    this.aMessages.push({ role: "user", content });
    this.render();
    try {
      this.aMessages.push(
        await this.assistant.send(
          content.slice(0, 4000),
          window.location.pathname,
        ),
      );
    } catch {
      this.aError = this.t("assistantError");
    } finally {
      this.aBusy = false;
      this.render();
    }
  }

  private async assistantAct(a: AssistantAction, confirm: boolean) {
    if (!this.assistant || this.aBusy || a.status !== "proposed") return;
    this.aBusy = true;
    this.render();
    try {
      if (confirm) {
        const res = await this.assistant.confirm(a.id);
        const done = res.actions?.find((x) => x.id === a.id);
        Object.assign(a, done ?? { status: "done" });
      } else {
        await this.assistant.reject(a.id);
        a.status = "rejected";
      }
    } catch {
      a.status = "failed";
    } finally {
      this.aBusy = false;
      this.render();
    }
  }

  private helpView(): HTMLElement {
    const help = this.help;
    const searching = this.helpQuery.trim() !== "";
    const items = (help?.items ?? []).map((a) =>
      el(
        "a",
        {
          class: "list-item article",
          href: a.url,
          target: "_blank",
          rel: "noopener noreferrer",
        },
        [
          el("div", { class: "grow" }, [
            el("strong", { text: a.title }),
            a.excerpt ? el("span", { text: a.excerpt }) : null,
          ]),
          el("span", { class: "sr-only", text: this.t("helpOpens") }),
        ],
      ),
    );
    return el("div", { class: "body pad" }, [
      el("div", { class: "card", role: "search" }, [
        el("label", { class: "search" }, [
          icon("search"),
          el("span", { class: "sr-only", text: this.t("helpTitle") }),
          el("input", {
            class: "help-input",
            type: "search",
            maxlength: "200",
            autocomplete: "off",
            placeholder: this.t("helpSearch"),
            value: this.helpQuery,
            oninput: (e: Event) =>
              this.onHelpInput((e.target as HTMLInputElement).value),
          }),
        ]),
      ]),
      el(
        "div",
        {
          class: "card",
          "aria-live": "polite",
          "aria-busy": this.helpLoading ? "true" : undefined,
        },
        [
          items.length
            ? el("h3", {
                text: searching ? this.t("helpResults") : this.t("helpPopular"),
              })
            : null,
          ...items,
          searching && !this.helpLoading && !items.length
            ? el("p", { class: "muted", text: this.t("helpEmpty") })
            : null,
          help?.help_center_url
            ? el(
                "a",
                {
                  class: "link-strong",
                  href: help.help_center_url,
                  target: "_blank",
                  rel: "noopener noreferrer",
                },
                [this.t("helpAll"), icon("external")],
              )
            : null,
        ],
      ),
    ]);
  }

  private time(iso: string): string {
    try {
      return new Date(iso).toLocaleTimeString(
        this.locale === "tr" ? "tr-TR" : "en-US",
        { hour: "2-digit", minute: "2-digit" },
      );
    } catch {
      return "";
    }
  }

  private day(iso: string): string {
    try {
      return new Date(iso).toLocaleDateString(
        this.locale === "tr" ? "tr-TR" : "en-US",
        { day: "numeric", month: "long" },
      );
    } catch {
      return "";
    }
  }

  private chat(): Child[] {
    const s = this.s!;
    const msgs = el("div", {
      class: "msgs",
      role: "log",
      "aria-live": "polite",
      "aria-relevant": "additions",
    });
    let lastDay = "";
    const list = [...this.messages];
    if (this.intro && !this.active)
      list.unshift({
        uuid: "intro",
        direction: "out",
        sender_name: this.intro.sender,
        kind: "text",
        content: this.intro.text,
        status: "sent",
        client_id: null,
        attachments: [],
        created_at: new Date().toISOString(),
        read_at: null,
        local: true,
      });
    const lastIn = [...list]
      .reverse()
      .find(
        (m) => m.direction === "in" && !m.pending && !m.failed && !m.queued,
      );
    for (const m of list) {
      const d = this.day(m.created_at);
      if (d !== lastDay) {
        msgs.append(el("div", { class: "day", text: d }));
        lastDay = d;
      }
      const bubble = el("div", {
        class: `msg${m.pending ? " pending" : ""}${m.failed ? " failed" : ""}`,
      });
      if (m.content) bubble.append(renderText(m.content, s.features.markdown));
      for (const a of m.attachments) {
        const href = a.url
          ? a.url.startsWith("/v1/")
            ? `${this.appOrigin}/api${a.url}`
            : a.url
          : "";
        const link = el("a", {
          class: "file",
          href: href || undefined,
          target: "_blank",
          rel: "noopener noreferrer",
        });
        if (href && a.content_type.startsWith("image/"))
          link.append(
            el("img", { src: href, alt: a.filename, loading: "lazy" }),
          );
        link.append(`${a.filename} · ${formatBytes(a.size_bytes)}`);
        bubble.append(link);
      }
      if (m.failed) {
        bubble.title = this.t("failed");
        bubble.setAttribute("role", "button");
        bubble.tabIndex = 0;
        bubble.addEventListener(
          "click",
          () => void this.send(m.content, null, m),
        );
      }
      const bot = m.sender_type === "bot";
      const who =
        m.direction === "in"
          ? this.t("you")
          : m.sender_name || (bot ? this.t("bot") : this.t("team"));
      msgs.append(
        el("div", { class: `row ${m.direction}` }, [
          m.direction === "out"
            ? el("span", { class: "avatar", "aria-hidden": "true" }, [
                bot
                  ? icon("sparkles")
                  : (m.sender_name || "•").slice(0, 2).toUpperCase(),
              ])
            : null,
          bubble,
        ]),
      );
      let meta = `${who} · ${this.time(m.created_at)}`;
      if (m.failed) meta = this.t("failed");
      else if (m.queued) meta = this.t("queued");
      else if (m === lastIn && s.features.read_receipts)
        meta += ` · ${m.read_at || m.status === "read" ? this.t("read") : this.t("delivered")}`;
      msgs.append(el("div", { class: `meta ${m.direction}`, text: meta }));
    }
    if (this.agentTyping && s.features.typing)
      msgs.append(
        el("div", { class: "row out" }, [
          el(
            "span",
            {
              class: "typing",
              "aria-label": `${this.t("team")} ${this.t("typing")}`,
            },
            [el("i"), el("i"), el("i")],
          ),
        ]),
      );
    return [el("div", { class: "body" }, [msgs]), this.footer()];
  }

  private ratingForm(): HTMLElement {
    const o = this.s!.csat_options;
    const question =
      pick(o.question, this.locale as Locale) || this.t("rateTitle");
    const values =
      o.scale === "thumbs"
        ? ([
            [5, "👍", this.t("rateYes")],
            [1, "👎", this.t("rateNo")],
          ] as const)
        : [1, 2, 3, 4, 5].map(
            (i) =>
              [
                i,
                o.scale === "emoji" ? CSAT_EMOJI[i - 1]! : "★",
                String(i),
              ] as const,
          );
    const scale = el("div", {
      class: `scale ${o.scale}`,
      role: "radiogroup",
      "aria-label": question,
    });
    for (const [score, glyph, label] of values)
      scale.append(
        el("button", {
          class: (
            o.scale === "stars"
              ? score <= this.ratingScore
              : score === this.ratingScore
          )
            ? "on"
            : "",
          type: "button",
          role: "radio",
          "aria-checked": String(score === this.ratingScore),
          "aria-label": label,
          text: glyph,
          onclick: () => {
            this.ratingScore = score;
            if (!o.comment) void this.rate(score, "");
            else this.render();
          },
        }),
      );
    const comment = el("textarea", {
      placeholder: this.t("rateComment"),
      maxlength: 2000,
      "aria-label": this.t("rateComment"),
    });
    return el("div", { class: "rating card" }, [
      el("div", { text: question }),
      scale,
      o.comment ? comment : null,
      o.comment
        ? el("button", {
            class: "primary",
            type: "button",
            disabled: this.ratingScore === 0,
            text: this.t("rateSend"),
            onclick: () => void this.rate(this.ratingScore, comment.value),
          })
        : null,
    ]);
  }

  private footer(): HTMLElement {
    const s = this.s!;
    const c = this.active;
    if (c && !c.open) {
      const parts: Child[] = [
        el("div", { class: "notice", text: this.t("closed") }),
      ];
      if (c.rating)
        parts.push(
          el("div", { class: "rating muted", text: this.t("rateThanks") }),
        );
      else if (c.can_rate && s.csat) parts.push(this.ratingForm());
      parts.push(
        el("button", {
          class: "primary",
          type: "button",
          onclick: () => void this.openConversation(null),
          text: this.t("startNew"),
        }),
      );
      return el("div", { class: "footer" }, parts);
    }
    const offline = this.offline();
    const fields: Child[] = [];
    if (this.notice)
      fields.push(
        el("div", { class: "notice", role: "status", text: this.notice }),
      );
    if (!c && offline)
      fields.push(
        el("div", {
          class: "notice",
          text:
            this.copy(s.away_mode ? "away_message" : "offline_message") ||
            this.t(s.away_mode ? "away" : "offline"),
        }),
      );
    if (!c && offline && s.offline.mode === "message")
      return el("div", { class: "footer" }, fields);
    if (!c) {
      const form = this.boot!.request_form;
      if (form?.description)
        fields.push(el("div", { class: "notice", text: form.description }));
      for (const f of this.formFields()) fields.push(this.renderFormField(f));
      fields.push(...this.preChatFields());
    }
    if (!c && !this.draft && !this.messages.length && s.quick_replies.length)
      fields.push(
        el(
          "div",
          {
            class: "chips",
            style: "margin-bottom:8px",
            role: "group",
            "aria-label": this.t("quickReplies"),
          },
          s.quick_replies.map((q) => {
            const label = pick(q, this.locale as Locale);
            return el("button", {
              class: "chip",
              type: "button",
              text: label,
              onclick: () => void this.send(label, null),
            });
          }),
        ),
      );
    const lastOut = [...this.messages]
      .reverse()
      .find((m) => m.direction === "out");
    if (
      c &&
      s.ai.enabled &&
      s.ai.human_button &&
      lastOut?.sender_type === "bot"
    )
      fields.push(
        el(
          "button",
          {
            class: "secondary human",
            type: "button",
            onclick: () =>
              void this.send(this.t("human"), null, undefined, "human"),
          },
          [icon("user"), this.t("humanButton")],
        ),
      );
    if (this.emojiOpen && s.features.emoji)
      fields.push(
        el(
          "div",
          { class: "emoji-grid", role: "group", "aria-label": this.t("emoji") },
          EMOJIS.map((e) =>
            el("button", {
              type: "button",
              text: e,
              "aria-label": e,
              onclick: () => {
                const ta = this.shadow?.querySelector(
                  "textarea.compose",
                ) as HTMLTextAreaElement | null;
                const start = ta?.selectionStart ?? this.draft.length;
                const end = ta?.selectionEnd ?? this.draft.length;
                const value = ta?.value ?? this.draft;
                this.draft = value.slice(0, start) + e + value.slice(end);
                this.emojiOpen = false;
                this.lastSignature = "";
                this.render();
                this.focusComposer();
              },
            }),
          ),
        ),
      );
    const placeholder = this.copy("placeholder") || this.t("placeholder");
    const ta = el("textarea", {
      class: "compose",
      rows: 1,
      placeholder,
      "aria-label": placeholder,
      maxlength: this.boot!.limits.max_text_length,
    });
    ta.value = this.draft;
    ta.addEventListener("input", () => {
      this.draft = ta.value;
      ta.style.height = "auto";
      ta.style.height = `${Math.min(ta.scrollHeight, 120)}px`;
      this.sendTyping(true);
      window.clearTimeout(this.typingStop);
      this.typingStop = window.setTimeout(() => this.sendTyping(false), 4000);
    });
    ta.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
        e.preventDefault();
        void this.send(ta.value, null);
      }
    });
    const uploads = s.file_upload && s.attachments.types.length > 0;
    const fileInput = el("input", {
      type: "file",
      class: "sr-only",
      tabindex: -1,
      "aria-hidden": "true",
      accept: acceptList(s.attachments.types),
    });
    fileInput.addEventListener("change", () => {
      const f = fileInput.files?.[0] ?? null;
      if (f) void this.send(ta.value, f);
      fileInput.value = "";
    });
    return el("div", { class: "footer" }, [
      ...fields,
      this.error
        ? el("p", { class: "error", role: "alert", text: this.error })
        : null,
      el("div", { class: "composer" }, [
        uploads
          ? el(
              "button",
              {
                class: "icon-btn",
                type: "button",
                "aria-label": this.t("attach"),
                title: this.t("attach"),
                onclick: () => fileInput.click(),
              },
              [icon("clip")],
            )
          : null,
        s.features.emoji
          ? el(
              "button",
              {
                class: "icon-btn",
                type: "button",
                "aria-label": this.t("emoji"),
                "aria-expanded": String(this.emojiOpen),
                title: this.t("emoji"),
                onclick: () => {
                  this.draft = ta.value;
                  this.emojiOpen = !this.emojiOpen;
                  this.render();
                },
              },
              [icon("smile")],
            )
          : null,
        this.active && this.replayPolicy?.replay_enabled && this.captureAllowed
          ? el("button", {
              type: "button",
              class: "icon-btn",
              title: this.t("captureReplay"),
              "aria-label": this.t("captureReplay"),
              text: "◉",
              onclick: () => void this.startReplay(),
            })
          : null,
        uploads ? fileInput : null,
        ta,
        el(
          "button",
          {
            class: "icon-btn send",
            type: "button",
            "aria-label": this.t("send"),
            onclick: () => void this.send(ta.value, null),
          },
          [icon("send")],
        ),
      ]),
      s.appearance.show_branding
        ? el("div", { class: "powered", text: this.t("poweredBy") })
        : null,
    ]);
  }

  private preChatFields(): Node[] {
    const s = this.s!;
    const pc = s.prechat;
    const out: Node[] = [];
    const req = (on: boolean) => (on ? ` (${this.t("required")})` : "");
    if (this.needsOfflineEmail()) {
      const node = el("input", {
        type: "email",
        autocomplete: "email",
        value: this.profile.email,
        maxlength: 254,
      });
      node.addEventListener("input", () => (this.profile.email = node.value));
      out.push(
        el("label", { class: "field" }, [
          el("span", { text: this.t("email") + req(true) }),
          node,
        ]),
      );
    }
    if (!pc.enabled) return out;
    if (pc.consent.enabled) {
      const box = el("input", { type: "checkbox" });
      box.checked = this.consentChecked;
      box.addEventListener("change", () => (this.consentChecked = box.checked));
      const text =
        pick(pc.consent.text, this.locale as Locale) ||
        this.t("consentDefault");
      out.push(
        el("label", { class: "field check" }, [
          box,
          el("span", {}, [
            text + req(pc.consent.required),
            pc.consent.url ? " " : null,
            pc.consent.url
              ? el("a", {
                  href: pc.consent.url,
                  target: "_blank",
                  rel: "noopener noreferrer",
                  text: this.t("privacy"),
                })
              : null,
          ]),
        ]),
      );
    }
    return out;
  }

  private renderFormField(f: FormField): Node {
    const label =
      (f.label || f.key) + (f.required ? ` (${this.t("required")})` : "");
    const set = (v: unknown, rerender: boolean) => {
      this.formValues[f.key] = v;
      if (rerender) {
        this.lastSignature = "";
        this.render();
      }
    };
    const cur = this.formValues[f.key];
    let control: HTMLElement;
    if (f.type === "boolean") {
      const box = el("input", { type: "checkbox" });
      box.checked = cur === true;
      box.addEventListener("change", () => set(box.checked, true));
      return el("label", { class: "field check" }, [
        box,
        el("span", { text: label }),
      ]);
    }
    if (f.type === "select") {
      const sel = el("select", {}, [
        el("option", { value: "", text: this.t("choose") }),
        ...f.options.map((o) =>
          el("option", { value: o.value, text: o.label }),
        ),
      ]);
      sel.value = typeof cur === "string" ? cur : "";
      sel.addEventListener("change", () => set(sel.value, true));
      control = sel;
    } else if (f.type === "multiselect") {
      const picked = Array.isArray(cur) ? (cur as string[]) : [];
      control = el(
        "div",
        { class: "choices" },
        f.options.map((o) => {
          const box = el("input", { type: "checkbox" });
          box.checked = picked.includes(o.value);
          box.addEventListener("change", () =>
            set(
              box.checked
                ? [...picked, o.value]
                : picked.filter((x) => x !== o.value),
              true,
            ),
          );
          return el("label", { class: "check" }, [
            box,
            el("span", { text: o.label }),
          ]);
        }),
      );
    } else {
      const types: Record<string, string> = {
        email: "email",
        phone: "tel",
        url: "url",
        number: "number",
        money: "number",
        date: "date",
      };
      const input =
        f.type === "textarea"
          ? el("textarea", { rows: 2, placeholder: f.placeholder })
          : el("input", {
              type: types[f.type] ?? "text",
              placeholder: f.placeholder,
              maxlength: 500,
            });
      input.value = typeof cur === "string" ? cur : "";
      input.addEventListener("input", () => set(input.value, false));
      input.addEventListener("change", () => set(input.value, true));
      control = input;
    }
    return el("label", { class: "field" }, [
      el("span", { text: label }),
      control,
      f.help ? el("small", { text: f.help }) : null,
    ]);
  }
}

function pickScalars(
  v: Record<string, unknown>,
): Record<string, string | number | boolean> {
  const out: Record<string, string | number | boolean> = {};
  for (const [k, x] of Object.entries(v ?? {}))
    if (
      typeof x === "string" ||
      typeof x === "number" ||
      typeof x === "boolean"
    )
      out[k] = x;
  return out;
}
