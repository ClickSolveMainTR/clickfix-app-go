/**
 * Widget settings schema v2 (CF-150), shared by the SDK runtime and the
 * panel editor. It mirrors backend/internal/platform/channels/widget
 * (settings.go + normalize.go): `normalize` applies the same defaults and
 * bounds, leniently (invalid values fall back; the API reports them as
 * issues on draft save and refuses them on publish).
 *
 * No DOM access here: the editor imports this file on the server too.
 */

export const SCHEMA_VERSION = 2;
export const LOCALES = ["tr", "en"] as const;
export type Locale = (typeof LOCALES)[number];
export type Localized = Partial<Record<Locale, string>>;

export const THEMES = ["light", "dark", "auto"] as const;
export const FONTS = ["system", "inherit", "rounded", "serif", "mono"] as const;
export const LAUNCHER_ICONS = [
  "chat",
  "help",
  "headset",
  "sparkles",
  "wave",
  "custom",
] as const;
export const LAUNCHER_SHAPES = ["circle", "rounded", "pill"] as const;
export const PATTERNS = ["none", "dots", "grid", "waves", "diagonal"] as const;
export const REPLY_MODES = ["auto", "manual", "hidden"] as const;
export const REPLY_VALUES = ["minutes", "hour", "hours", "day"] as const;
export const CARD_TYPES = [
  "conversation",
  "help_search",
  "articles",
  "status",
  "link",
  "booking",
  "channels",
] as const;
export const CSAT_SCALES = ["stars", "emoji", "thumbs"] as const;
export const OFFLINE_MODES = ["form", "message", "hide"] as const;
export const ATTACHMENT_TYPES = [
  "image",
  "pdf",
  "document",
  "spreadsheet",
  "video",
  "audio",
  "archive",
  "text",
] as const;
export const TRIGGER_FREQUENCIES = ["once", "session", "always"] as const;
export const VISITOR_KINDS = ["any", "new", "returning"] as const;
export const DEVICE_KINDS = ["any", "desktop", "mobile"] as const;
export const TARGET_DEVICES = ["all", "desktop", "mobile"] as const;
export const IDENTIFIED_KINDS = ["any", "yes", "no"] as const;
export const URL_OPS = [
  "contains",
  "equals",
  "starts_with",
  "ends_with",
  "glob",
  "not_contains",
] as const;
export const COUNTRY_MODES = ["all", "allow", "block"] as const;
export const PERSISTENCES = ["local", "session", "memory"] as const;
export const PRECHAT_TYPES = [
  "text",
  "textarea",
  "email",
  "phone",
  "number",
  "select",
  "checkbox",
] as const;
export const FIELD_MODES = ["required", "optional", "hidden"] as const;
export const DAY_KEYS = [
  "sun",
  "mon",
  "tue",
  "wed",
  "thu",
  "fri",
  "sat",
] as const;

export const LIMITS = {
  triggers: 20,
  quickReplies: 8,
  homeCards: 12,
  preChatFields: 10,
  mappings: 50,
  urlRules: 20,
  customCSS: 8000,
} as const;

export type FieldMode = (typeof FIELD_MODES)[number];
export type DayKey = (typeof DAY_KEYS)[number];
export type Period = { start: string; end: string };
export type URLRule = { op: (typeof URL_OPS)[number]; value: string };

export type PreChatField = {
  key: string;
  label: Localized;
  type: (typeof PRECHAT_TYPES)[number];
  required: boolean;
  options: string[];
  attribute: string;
};

export type Launcher = {
  icon: (typeof LAUNCHER_ICONS)[number];
  image_url: string;
  label: Localized;
  size: number;
  shape: (typeof LAUNCHER_SHAPES)[number];
  offset_x: number;
  offset_y: number;
};

export type Appearance = {
  theme: (typeof THEMES)[number];
  launcher_color: string;
  header_color: string;
  bubble_color: string;
  font: (typeof FONTS)[number];
  radius: number;
  launcher: Launcher;
  logo_url: string;
  pattern: (typeof PATTERNS)[number];
  z_index: number;
  custom_css: string;
  show_branding: boolean;
};

export type Texts = {
  greeting_title: string;
  greeting_text: string;
  team_intro: string;
  offline_message: string;
  away_message: string;
  placeholder: string;
};
export const TEXT_KEYS: (keyof Texts)[] = [
  "greeting_title",
  "greeting_text",
  "team_intro",
  "offline_message",
  "away_message",
  "placeholder",
];
export const TEXT_LIMITS: Record<keyof Texts, number> = {
  greeting_title: 80,
  greeting_text: 300,
  team_intro: 200,
  offline_message: 300,
  away_message: 300,
  placeholder: 80,
};

export type HomeCard = {
  id: string;
  type: (typeof CARD_TYPES)[number];
  enabled: boolean;
  title: Localized;
  description: Localized;
  url: string;
};

export type TriggerConditions = {
  url: URLRule[];
  time_on_page_s: number;
  scroll_pct: number;
  exit_intent: boolean;
  visitor: (typeof VISITOR_KINDS)[number];
  device: (typeof DEVICE_KINDS)[number];
  utm_source: string;
  utm_medium: string;
  utm_campaign: string;
  identified: (typeof IDENTIFIED_KINDS)[number];
  languages: string[];
  countries: string[];
};

export type Trigger = {
  id: string;
  name: string;
  enabled: boolean;
  message: Localized;
  delay_s: number;
  frequency: (typeof TRIGGER_FREQUENCIES)[number];
  open: boolean;
  conditions: TriggerConditions;
};

export type Targeting = {
  show_on: URLRule[];
  hide_on: URLRule[];
  devices: (typeof TARGET_DEVICES)[number];
  countries: { mode: (typeof COUNTRY_MODES)[number]; list: string[] };
};

export type AttributeMapEntry = {
  key: string;
  target: string;
  anonymous: boolean;
};

export type WidgetSettings = {
  schema_version: number;
  color: string;
  position: "left" | "right";
  locale: "auto" | Locale;
  greeting_title: string;
  greeting_text: string;
  show_team: boolean;
  team_member_uuids: string[];
  prechat: {
    enabled: boolean;
    name: FieldMode;
    email: FieldMode;
    phone: FieldMode;
    fields: PreChatField[];
    consent: {
      enabled: boolean;
      required: boolean;
      text: Localized;
      url: string;
    };
  };
  business_hours: {
    enabled: boolean;
    timezone: string;
    days: Partial<Record<DayKey, Period[]>>;
  };
  offline_message: string;
  allowed_domains: string[];
  require_identity: boolean;
  file_upload: boolean;
  csat: boolean;
  appearance: Appearance;
  texts: Record<Locale, Texts>;
  reply_time: {
    mode: (typeof REPLY_MODES)[number];
    value: (typeof REPLY_VALUES)[number];
  };
  home: { cards: HomeCard[] };
  quick_replies: Localized[];
  csat_options: {
    scale: (typeof CSAT_SCALES)[number];
    comment: boolean;
    question: Localized;
  };
  offline: { mode: (typeof OFFLINE_MODES)[number]; collect_email: boolean };
  away_mode: boolean;
  channels: {
    chat: boolean;
    email: string;
    whatsapp: string;
    instagram: string;
    telegram: string;
    messenger: string;
    phone: string;
    callback: boolean;
    video: boolean;
  };
  ai: { enabled: boolean; human_button: boolean; handoff_keywords: string[] };
  attachments: {
    max_mb: number;
    types: (typeof ATTACHMENT_TYPES)[number][];
  };
  features: {
    emoji: boolean;
    typing: boolean;
    read_receipts: boolean;
    sound: boolean;
    notifications: boolean;
    markdown: boolean;
    history: boolean;
  };
  triggers: Trigger[];
  targeting: Targeting;
  privacy: {
    consent_required: boolean;
    cookieless: boolean;
    consent_text: Localized;
    privacy_url: string;
    persistence: (typeof PERSISTENCES)[number];
  };
  rate_limit: { messages_per_minute: number };
  mobile: { fullscreen: boolean };
  attribute_mapping: AttributeMapEntry[];
};

export const DEFAULT_CARD_TYPES: HomeCard["type"][] = [
  "conversation",
  "help_search",
  "articles",
  "booking",
  "channels",
];

const emptyTexts = (): Texts => ({
  greeting_title: "",
  greeting_text: "",
  team_intro: "",
  offline_message: "",
  away_message: "",
  placeholder: "",
});

/** Settings of a new widget (Go `Defaults`). */
export function defaults(): WidgetSettings {
  return {
    schema_version: SCHEMA_VERSION,
    color: "#4f46e5",
    position: "right",
    locale: "auto",
    greeting_title: "",
    greeting_text: "",
    show_team: true,
    team_member_uuids: [],
    prechat: {
      enabled: true,
      name: "optional",
      email: "optional",
      phone: "hidden",
      fields: [],
      consent: { enabled: false, required: true, text: {}, url: "" },
    },
    business_hours: { enabled: false, timezone: "Europe/Istanbul", days: {} },
    offline_message: "",
    allowed_domains: [],
    require_identity: false,
    file_upload: true,
    csat: true,
    appearance: {
      theme: "light",
      launcher_color: "",
      header_color: "",
      bubble_color: "",
      font: "system",
      radius: 16,
      launcher: {
        icon: "chat",
        image_url: "",
        label: {},
        size: 56,
        shape: "circle",
        offset_x: 20,
        offset_y: 20,
      },
      logo_url: "",
      pattern: "none",
      z_index: 2147483000,
      custom_css: "",
      show_branding: true,
    },
    texts: { tr: emptyTexts(), en: emptyTexts() },
    reply_time: { mode: "auto", value: "minutes" },
    home: {
      cards: DEFAULT_CARD_TYPES.map((t) => ({
        id: t,
        type: t,
        enabled: true,
        title: {},
        description: {},
        url: "",
      })),
    },
    quick_replies: [],
    csat_options: { scale: "stars", comment: true, question: {} },
    offline: { mode: "form", collect_email: true },
    away_mode: false,
    channels: {
      chat: true,
      email: "",
      whatsapp: "",
      instagram: "",
      telegram: "",
      messenger: "",
      phone: "",
      callback: false,
      video: false,
    },
    ai: { enabled: true, human_button: true, handoff_keywords: [] },
    attachments: {
      max_mb: 10,
      types: ["image", "pdf", "document", "spreadsheet", "text"],
    },
    features: {
      emoji: true,
      typing: true,
      read_receipts: true,
      sound: true,
      notifications: false,
      markdown: true,
      history: true,
    },
    triggers: [],
    targeting: {
      show_on: [],
      hide_on: [],
      devices: "all",
      countries: { mode: "all", list: [] },
    },
    privacy: {
      consent_required: false,
      cookieless: false,
      consent_text: {},
      privacy_url: "",
      persistence: "local",
    },
    rate_limit: { messages_per_minute: 30 },
    mobile: { fullscreen: true },
    attribute_mapping: [],
  };
}

// ---------------------------------------------------------------------------
// Lenient readers

type Obj = Record<string, unknown>;
const COLOR_RE = /^#[0-9a-f]{6}$/i;
const CLOCK_RE = /^([01]\d|2[0-3]):[0-5]\d$/;
const KEY_RE = /^[a-z][a-z0-9_]{0,39}$/;
const CC_RE = /^[A-Z]{2}$/;
const HANDLE_RE = /^[A-Za-z0-9_.]{2,64}$/;

const obj = (v: unknown): Obj | null =>
  v && typeof v === "object" && !Array.isArray(v) ? (v as Obj) : null;
const arr = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);
const clip = (s: string, n: number) => Array.from(s).slice(0, n).join("");
export function str(v: unknown, max = 1000): string {
  return typeof v === "string" ? clip(v.trim(), max) : "";
}
const bool = (v: unknown, d: boolean) => (typeof v === "boolean" ? v : d);
function oneOf<T extends string>(v: unknown, list: readonly T[], d: T): T {
  return typeof v === "string" && (list as readonly string[]).includes(v)
    ? (v as T)
    : d;
}
function int(v: unknown, lo: number, hi: number, d: number): number {
  return typeof v === "number" && Number.isInteger(v) && v >= lo && v <= hi
    ? v
    : d;
}
const color = (v: unknown) =>
  typeof v === "string" && COLOR_RE.test(v.trim())
    ? v.trim().toLowerCase()
    : "";
export const isColor = (v: string) => COLOR_RE.test(v);
export const isClock = (v: string) => CLOCK_RE.test(v);
export const isKey = (v: string) => KEY_RE.test(v);

/** https URLs (http only for localhost); everything else is dropped. */
export function safeURL(v: unknown): string {
  const s = str(v, 1000);
  if (!s) return "";
  try {
    const u = new URL(s);
    if (u.username || u.password) return "";
    if (u.protocol === "https:") return u.toString();
    if (
      u.protocol === "http:" &&
      (u.hostname === "localhost" || u.hostname === "127.0.0.1")
    )
      return u.toString();
  } catch {
    /* not a URL */
  }
  return "";
}

export function localized(v: unknown, max: number): Localized {
  const out: Localized = {};
  if (typeof v === "string") {
    const s = str(v, max);
    if (s) for (const l of LOCALES) out[l] = s;
    return out;
  }
  const o = obj(v);
  if (o)
    for (const l of LOCALES) {
      const s = str(o[l], max);
      if (s) out[l] = s;
    }
  return out;
}

/** Text of a locale, falling back to the other locales. */
export function pick(l: Localized | undefined, locale: Locale): string {
  if (!l) return "";
  return l[locale] || LOCALES.map((x) => l[x]).find(Boolean) || "";
}

const strList = (v: unknown, max: number, len = 200) =>
  arr(v)
    .map((x) => str(x, len))
    .filter(Boolean)
    .slice(0, max);

function urlRules(v: unknown): URLRule[] {
  return arr(v)
    .map(obj)
    .filter((x): x is Obj => !!x)
    .map((m) => ({
      op: oneOf(m.op, URL_OPS, "contains"),
      value: str(m.value, 300),
    }))
    .filter((r) => r.value)
    .slice(0, LIMITS.urlRules);
}

function countries(v: unknown): string[] {
  const out: string[] = [];
  for (const c of strList(v, 250, 2).map((x) => x.toUpperCase()))
    if (CC_RE.test(c) && !out.includes(c)) out.push(c);
  return out;
}

function digits(v: string): string {
  if (/[^0-9 +\-().]/.test(v)) return "";
  const d = v.replace(/\D/g, "");
  return d.length >= 6 && d.length <= 15 ? d : "";
}

const handle = (v: unknown) => {
  const s = str(v, 65).replace(/^@/, "");
  return HANDLE_RE.test(s) ? s : "";
};

function validTarget(t: string): boolean {
  if (t === "name" || t === "email" || t === "phone") return true;
  return t.startsWith("custom:") && KEY_RE.test(t.slice(7));
}

/** Reads stored (or draft) settings with the API's fallbacks. */
export function normalize(raw: unknown): WidgetSettings {
  const s = defaults();
  const r = obj(raw);
  if (!r) return s;
  s.color = color(r.color) || s.color;
  s.position = r.position === "left" ? "left" : "right";
  s.locale = oneOf(r.locale, ["auto", ...LOCALES] as const, "auto");
  s.greeting_title = str(r.greeting_title, 80);
  s.greeting_text = str(r.greeting_text, 300);
  s.offline_message = str(r.offline_message, 300);
  s.show_team = bool(r.show_team, s.show_team);
  s.require_identity = bool(r.require_identity, false);
  s.file_upload = bool(r.file_upload, s.file_upload);
  s.csat = bool(r.csat, s.csat);
  s.away_mode = bool(r.away_mode, false);
  s.team_member_uuids = strList(r.team_member_uuids, 8, 36);
  s.allowed_domains = strList(r.allowed_domains, 50, 255);

  const pc = obj(r.prechat);
  if (pc) {
    s.prechat.enabled = bool(pc.enabled, true);
    s.prechat.name = oneOf(pc.name, FIELD_MODES, "optional");
    s.prechat.email = oneOf(pc.email, FIELD_MODES, "optional");
    s.prechat.phone = oneOf(pc.phone, FIELD_MODES, "hidden");
    const seen = new Set(["name", "email", "phone", "message"]);
    for (const f of arr(pc.fields).map(obj)) {
      if (!f || s.prechat.fields.length >= LIMITS.preChatFields) continue;
      const field: PreChatField = {
        key: str(f.key, 40),
        label: localized(f.label, 100),
        type: oneOf(f.type, PRECHAT_TYPES, "text"),
        required: bool(f.required, false),
        options: strList(f.options, 30, 80),
        attribute: str(f.attribute, 40),
      };
      if (!KEY_RE.test(field.key) || seen.has(field.key)) continue;
      if (!Object.keys(field.label).length) continue;
      if (field.type === "select" && !field.options.length) continue;
      if (field.attribute && !KEY_RE.test(field.attribute))
        field.attribute = "";
      seen.add(field.key);
      s.prechat.fields.push(field);
    }
    const c = obj(pc.consent);
    if (c)
      s.prechat.consent = {
        enabled: bool(c.enabled, false),
        required: bool(c.required, true),
        text: localized(c.text, 500),
        url: safeURL(c.url),
      };
  }

  const bh = obj(r.business_hours);
  if (bh) {
    s.business_hours.enabled = bool(bh.enabled, false);
    s.business_hours.timezone = str(bh.timezone, 64) || "Europe/Istanbul";
    const days = obj(bh.days);
    if (days)
      for (const d of DAY_KEYS) {
        const periods = arr(days[d])
          .map(obj)
          .filter((p): p is Obj => !!p)
          .map((p) => ({ start: str(p.start, 5), end: str(p.end, 5) }))
          .filter(
            (p) =>
              CLOCK_RE.test(p.start) && CLOCK_RE.test(p.end) && p.start < p.end,
          )
          .slice(0, 4);
        if (periods.length) s.business_hours.days[d] = periods;
      }
  }

  const a = obj(r.appearance);
  if (a) {
    const ap = s.appearance;
    ap.theme = oneOf(a.theme, THEMES, ap.theme);
    ap.launcher_color = color(a.launcher_color);
    ap.header_color = color(a.header_color);
    ap.bubble_color = color(a.bubble_color);
    ap.font = oneOf(a.font, FONTS, ap.font);
    ap.radius = int(a.radius, 0, 28, ap.radius);
    ap.logo_url = safeURL(a.logo_url);
    ap.pattern = oneOf(a.pattern, PATTERNS, ap.pattern);
    ap.z_index = int(a.z_index, 1, 2147484000, ap.z_index);
    ap.custom_css = str(a.custom_css, LIMITS.customCSS);
    ap.show_branding = bool(a.show_branding, true);
    const l = obj(a.launcher);
    if (l) {
      const ln = ap.launcher;
      ln.icon = oneOf(l.icon, LAUNCHER_ICONS, ln.icon);
      ln.image_url = safeURL(l.image_url);
      if (ln.icon === "custom" && !ln.image_url) ln.icon = "chat";
      ln.label = localized(l.label, 40);
      ln.size = int(l.size, 40, 80, ln.size);
      ln.shape = oneOf(l.shape, LAUNCHER_SHAPES, ln.shape);
      ln.offset_x = int(l.offset_x, 0, 200, ln.offset_x);
      ln.offset_y = int(l.offset_y, 0, 200, ln.offset_y);
    }
  }

  const texts = obj(r.texts);
  if (texts)
    for (const l of LOCALES) {
      const t = obj(texts[l]) ?? {};
      for (const k of TEXT_KEYS) s.texts[l][k] = str(t[k], TEXT_LIMITS[k]);
    }

  const rt = obj(r.reply_time);
  if (rt) {
    s.reply_time.mode = oneOf(rt.mode, REPLY_MODES, "auto");
    s.reply_time.value = oneOf(rt.value, REPLY_VALUES, "minutes");
  }

  const home = obj(r.home);
  if (home && Array.isArray(home.cards)) {
    const seen = new Set<string>();
    s.home.cards = [];
    for (const c of home.cards.map(obj)) {
      if (!c || s.home.cards.length >= LIMITS.homeCards) continue;
      const type = oneOf(c.type, CARD_TYPES, "link");
      if (!(CARD_TYPES as readonly unknown[]).includes(c.type)) continue;
      const card: HomeCard = {
        id: str(c.id, 40) || type,
        type,
        enabled: bool(c.enabled, true),
        title: localized(c.title, 80),
        description: localized(c.description, 200),
        url: safeURL(c.url),
      };
      if (seen.has(card.id)) continue;
      if ((type === "link" || type === "status") && !card.url) continue;
      if (type === "link" && !Object.keys(card.title).length) continue;
      seen.add(card.id);
      s.home.cards.push(card);
    }
  }

  s.quick_replies = arr(r.quick_replies)
    .map((q) => localized(q, 120))
    .filter((q) => Object.keys(q).length)
    .slice(0, LIMITS.quickReplies);

  const cs = obj(r.csat_options);
  if (cs)
    s.csat_options = {
      scale: oneOf(cs.scale, CSAT_SCALES, "stars"),
      comment: bool(cs.comment, true),
      question: localized(cs.question, 160),
    };

  const off = obj(r.offline);
  if (off)
    s.offline = {
      mode: oneOf(off.mode, OFFLINE_MODES, "form"),
      collect_email: bool(off.collect_email, true),
    };

  const ch = obj(r.channels);
  if (ch) {
    const email = str(ch.email, 254);
    s.channels = {
      chat: bool(ch.chat, true),
      email: /^[^\s@<>,;"]+@[^\s@<>,;"]+\.[^\s@<>,;"]+$/.test(email)
        ? email.toLowerCase()
        : "",
      whatsapp: digits(str(ch.whatsapp, 30)),
      instagram: handle(ch.instagram),
      telegram: handle(ch.telegram),
      messenger: handle(ch.messenger),
      phone: (() => {
        const raw = str(ch.phone, 30);
        const d = digits(raw);
        return d && raw.startsWith("+") ? "+" + d : d;
      })(),
      callback: bool(ch.callback, false),
      video: bool(ch.video, false),
    };
  }

  const ai = obj(r.ai);
  if (ai)
    s.ai = {
      enabled: bool(ai.enabled, true),
      human_button: bool(ai.human_button, true),
      handoff_keywords: strList(ai.handoff_keywords, 30, 60).map((k) =>
        k.toLowerCase(),
      ),
    };

  const at = obj(r.attachments);
  if (at) {
    s.attachments.max_mb = int(at.max_mb, 1, 10, 10);
    if (Array.isArray(at.types))
      s.attachments.types = ATTACHMENT_TYPES.filter((t) =>
        (at.types as unknown[]).includes(t),
      );
  }

  const f = obj(r.features);
  if (f)
    for (const k of Object.keys(
      s.features,
    ) as (keyof WidgetSettings["features"])[])
      s.features[k] = bool(f[k], s.features[k]);

  const ids = new Set<string>();
  for (const t of arr(r.triggers).map(obj)) {
    if (!t || s.triggers.length >= LIMITS.triggers) continue;
    const id = str(t.id, 40);
    const message = localized(t.message, 500);
    if (!id || ids.has(id) || !Object.keys(message).length) continue;
    const c = obj(t.conditions) ?? {};
    ids.add(id);
    s.triggers.push({
      id,
      name: str(t.name, 80),
      enabled: bool(t.enabled, true),
      message,
      delay_s: int(t.delay_s, 0, 3600, 0),
      frequency: oneOf(t.frequency, TRIGGER_FREQUENCIES, "once"),
      open: bool(t.open, false),
      conditions: {
        url: urlRules(c.url),
        time_on_page_s: int(c.time_on_page_s, 0, 3600, 0),
        scroll_pct: int(c.scroll_pct, 0, 100, 0),
        exit_intent: bool(c.exit_intent, false),
        visitor: oneOf(c.visitor, VISITOR_KINDS, "any"),
        device: oneOf(c.device, DEVICE_KINDS, "any"),
        utm_source: str(c.utm_source, 100),
        utm_medium: str(c.utm_medium, 100),
        utm_campaign: str(c.utm_campaign, 100),
        identified: oneOf(c.identified, IDENTIFIED_KINDS, "any"),
        languages: strList(c.languages, 20, 2)
          .filter((l) => l.length === 2)
          .map((l) => l.toLowerCase()),
        countries: countries(c.countries),
      },
    });
  }

  const tg = obj(r.targeting);
  if (tg) {
    const co = obj(tg.countries) ?? {};
    s.targeting = {
      show_on: urlRules(tg.show_on),
      hide_on: urlRules(tg.hide_on),
      devices: oneOf(tg.devices, TARGET_DEVICES, "all"),
      countries: {
        mode: oneOf(co.mode, COUNTRY_MODES, "all"),
        list: countries(co.list),
      },
    };
  }

  const pv = obj(r.privacy);
  if (pv)
    s.privacy = {
      consent_required: bool(pv.consent_required, false),
      cookieless: bool(pv.cookieless, false),
      consent_text: localized(pv.consent_text, 500),
      privacy_url: safeURL(pv.privacy_url),
      persistence: oneOf(pv.persistence, PERSISTENCES, "local"),
    };
  if (s.privacy.cookieless) s.privacy.persistence = "memory";

  const rl = obj(r.rate_limit);
  if (rl)
    s.rate_limit.messages_per_minute = int(rl.messages_per_minute, 5, 120, 30);
  const mob = obj(r.mobile);
  if (mob) s.mobile.fullscreen = bool(mob.fullscreen, true);

  const keys = new Set<string>();
  for (const m of arr(r.attribute_mapping).map(obj)) {
    if (!m || s.attribute_mapping.length >= LIMITS.mappings) continue;
    const e = {
      key: str(m.key, 40),
      target: str(m.target, 50),
      anonymous: bool(m.anonymous, false),
    };
    if (KEY_RE.test(e.key) && validTarget(e.target) && !keys.has(e.key)) {
      keys.add(e.key);
      s.attribute_mapping.push(e);
    }
  }
  return s;
}

/** Copy of a locale: per-locale text, else the legacy top-level value. */
export function text(
  s: WidgetSettings,
  locale: Locale,
  key: keyof Texts,
): string {
  const v = s.texts[locale]?.[key];
  if (v) return v;
  if (key === "greeting_title") return s.greeting_title;
  if (key === "greeting_text") return s.greeting_text;
  if (key === "offline_message") return s.offline_message;
  return "";
}

// ---------------------------------------------------------------------------
// Attachments (Go attachments.go)

const ATTACHMENT_EXTS: Record<(typeof ATTACHMENT_TYPES)[number], string[]> = {
  image: [".png", ".jpg", ".jpeg", ".gif", ".webp", ".heic", ".bmp"],
  pdf: [".pdf"],
  document: [".doc", ".docx", ".odt", ".rtf"],
  spreadsheet: [".xls", ".xlsx", ".ods", ".csv"],
  video: [".mp4", ".mov", ".webm"],
  audio: [".mp3", ".m4a", ".wav", ".ogg"],
  archive: [".zip", ".7z", ".rar", ".gz"],
  text: [".txt", ".log", ".json"],
};
const ATTACHMENT_MIMES: Record<(typeof ATTACHMENT_TYPES)[number], string[]> = {
  image: ["image/"],
  pdf: ["application/pdf"],
  document: [
    "application/msword",
    "application/vnd.openxmlformats-officedocument.wordprocessingml",
    "application/vnd.oasis.opendocument.text",
    "application/rtf",
  ],
  spreadsheet: [
    "application/vnd.ms-excel",
    "application/vnd.openxmlformats-officedocument.spreadsheetml",
    "application/vnd.oasis.opendocument.spreadsheet",
    "text/csv",
  ],
  video: ["video/"],
  audio: ["audio/"],
  archive: [
    "application/zip",
    "application/x-zip-compressed",
    "application/x-7z-compressed",
    "application/x-rar-compressed",
    "application/gzip",
  ],
  text: ["text/plain", "application/json"],
};

export function attachmentKind(
  contentType: string,
  filename: string,
): (typeof ATTACHMENT_TYPES)[number] | "" {
  const dot = filename.lastIndexOf(".");
  const ext = dot >= 0 ? filename.slice(dot).toLowerCase() : "";
  for (const k of ATTACHMENT_TYPES)
    if (ext && ATTACHMENT_EXTS[k].includes(ext)) return k;
  const ct = contentType.split(";")[0]!.trim().toLowerCase();
  for (const k of ATTACHMENT_TYPES)
    if (ATTACHMENT_MIMES[k].some((m) => ct.startsWith(m))) return k;
  return "";
}

/** The file picker `accept` list for the allowed groups. */
export function acceptList(types: readonly string[]): string {
  return ATTACHMENT_TYPES.filter((t) => types.includes(t))
    .flatMap((t) => ATTACHMENT_EXTS[t])
    .join(",");
}
