/**
 * Widget editor logic (CF-150). The settings schema itself (types,
 * defaults, normalize) is shared with the SDK runtime in
 * widget-sdk/src/shared/schema.ts, the TypeScript twin of
 * backend/internal/platform/channels/widget. Everything here is pure (no
 * React, no DOM except where noted) and covered by settings.test.mjs.
 */
import { contrast } from "@widget/shared/theme";
import { sanitizeCSS } from "@widget/shared/css";
import {
  defaults,
  isClock,
  isColor,
  normalize,
  type DayKey as SharedDayKey,
  type WidgetSettings as SharedSettings,
} from "@widget/shared/schema";

export {
  defaults,
  normalize,
  LOCALES,
  LIMITS,
  type Localized,
  type Locale,
  type Trigger,
  type HomeCard,
  type URLRule,
} from "@widget/shared/schema";

export type DayKey = SharedDayKey;
export type Period = { start: string; end: string };

/** Session replay / co-browse opt-in (CF-126); everything starts off. */
export type ReplaySettings = {
  replay_enabled: boolean;
  cobrowse_enabled: boolean;
  control_enabled: boolean;
  retention_days: number;
  block_selectors: string[];
};

/** Editor settings: the shared schema plus the raw replay block. */
export type WidgetSettings = SharedSettings & { replay: ReplaySettings };

/** Bounds of `Policy`: out-of-range input disables capture server side. */
export const REPLAY_LIMITS = {
  minRetention: 1,
  maxRetention: 30,
  maxSelectors: 20,
  maxSelectorLength: 256,
} as const;

export const DEFAULT_REPLAY: ReplaySettings = {
  replay_enabled: false,
  cobrowse_enabled: false,
  control_enabled: false,
  retention_days: 7,
  block_selectors: [],
};

/** Monday-first week (the brand editor uses the same order). */
export const DAYS: DayKey[] = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"];

export const DEFAULT_SETTINGS: WidgetSettings = {
  ...defaults(),
  replay: DEFAULT_REPLAY,
};

/**
 * Reads stored (or draft) settings with the API's fallbacks. A
 * `business_hours` key of widgets saved before CF-198 is dropped: working
 * hours now come from the SLA schedule.
 */
export function readSettings(raw: unknown): WidgetSettings {
  const s = normalize(raw);
  const r = (raw ?? {}) as Record<string, unknown>;
  return { ...s, replay: readReplay(r.replay) };
}

/** Same fallbacks as `Policy`: invalid retention falls back to 7 days. */
function readReplay(v: unknown): ReplaySettings {
  const r = (v ?? {}) as Record<string, unknown>;
  const days = typeof r.retention_days === "number" ? r.retention_days : 7;
  const strings = (x: unknown) =>
    Array.isArray(x) ? x.filter((i): i is string => typeof i === "string") : [];
  return {
    replay_enabled: r.replay_enabled === true,
    cobrowse_enabled: r.cobrowse_enabled === true,
    control_enabled: r.control_enabled === true,
    retention_days:
      Number.isInteger(days) &&
      days >= REPLAY_LIMITS.minRetention &&
      days <= REPLAY_LIMITS.maxRetention
        ? days
        : 7,
    block_selectors: strings(r.block_selectors),
  };
}

export const isValidColor = isColor;
export const isValidClock = isClock;

// ---------------------------------------------------------------------------
// Immutable path updates ("appearance.launcher.size", "triggers.2.message")

type Indexable = Record<string, unknown> | unknown[];

export function getPath(obj: unknown, path: string): unknown {
  let cur: unknown = obj;
  for (const key of path.split(".")) {
    if (cur === null || typeof cur !== "object") return undefined;
    cur = (cur as Record<string, unknown>)[key];
  }
  return cur;
}

/** Returns a copy of obj with path set to value (missing objects created). */
export function setPath<T>(obj: T, path: string, value: unknown): T {
  const [head, ...rest] = path.split(".");
  const src = (obj ?? {}) as Indexable;
  const copy: Indexable = Array.isArray(src) ? [...src] : { ...src };
  const key = Array.isArray(copy) ? Number(head) : head!;
  const current = (copy as Record<string | number, unknown>)[key];
  (copy as Record<string | number, unknown>)[key] = rest.length
    ? setPath(
        current ?? (/^\d+$/.test(rest[0]!) ? [] : {}),
        rest.join("."),
        value,
      )
    : value;
  return copy as T;
}

// ---------------------------------------------------------------------------
// Issues (server issues of the draft plus instant client checks)

export type Issue = {
  path: string;
  code: string;
  message: string;
  /** Warnings never block publishing. */
  level?: "error" | "warning";
};

/** Issues of one field and everything under it. */
export function issuesAt(issues: Issue[], path: string): Issue[] {
  return issues.filter((i) => i.path === path || i.path.startsWith(path + "."));
}

/** Instant checks while typing; the API's issues stay the authority. */
export function quickChecks(s: WidgetSettings): Issue[] {
  const out: Issue[] = [];
  const err = (path: string, code: string, message: string) =>
    out.push({ path, code, message, level: "error" });
  const warn = (path: string, code: string, message: string) =>
    out.push({ path, code, message, level: "warning" });
  for (const [path, value] of [
    ["color", s.color],
    ["appearance.launcher_color", s.appearance.launcher_color],
    ["appearance.header_color", s.appearance.header_color],
    ["appearance.bubble_color", s.appearance.bubble_color],
  ] as const) {
    if (value && !isColor(value)) err(path, "invalid", "color");
  }
  // WCAG AA for large text (3:1) against white text or the dark panel.
  if (isColor(s.color)) {
    const best = Math.max(
      contrast(s.color, "#ffffff"),
      contrast(s.color, "#111827"),
    );
    if (best < 3) warn("color", "contrast", "contrast");
    const bg = s.appearance.theme === "dark" ? "#111318" : "#ffffff";
    if (contrast(s.color, bg) < 1.6)
      warn("color", "contrast_bg", "contrast_bg");
  }
  if (
    s.appearance.launcher.icon === "custom" &&
    !s.appearance.launcher.image_url
  )
    err("appearance.launcher.image_url", "required", "required");
  const css = sanitizeCSS(s.appearance.custom_css);
  if (css.problems.length)
    warn("appearance.custom_css", "unsafe", css.problems.join("; "));
  s.triggers.forEach((t, i) => {
    if (!t.message.tr && !t.message.en)
      err(`triggers.${i}.message`, "required", "required");
  });
  if (
    s.prechat.consent.enabled &&
    !s.prechat.consent.text.tr &&
    !s.prechat.consent.text.en
  )
    err("prechat.consent.text", "required", "required");
  if (
    s.targeting.countries.mode !== "all" &&
    !s.targeting.countries.list.length
  )
    err("targeting.countries.list", "required", "required");
  if (s.require_identity === true && s.privacy.cookieless)
    warn("privacy.cookieless", "identity", "identity");
  return out;
}

/** True when nothing blocks publishing. */
export function publishable(issues: Issue[]): boolean {
  return !issues.some((i) => (i.level ?? "error") === "error");
}

// ---------------------------------------------------------------------------
// Sections (sidebar navigation with search)

export type SectionId =
  | "appearance"
  | "launcher"
  | "texts"
  | "home"
  | "prechat"
  | "conversation"
  | "channels"
  | "ai"
  | "triggers"
  | "targeting"
  | "privacy"
  | "security"
  | "mobile_apps"
  | "replay"
  | "install";

export type SectionGroup = "design" | "content" | "behaviour" | "security";

export const SECTIONS: {
  id: SectionId;
  group: SectionGroup;
  /** Settings paths the section edits (issue counts). */
  paths: string[];
  /** Extra search words (both languages). */
  keywords: string;
}[] = [
  {
    id: "appearance",
    group: "design",
    paths: [
      "color",
      "position",
      "appearance.theme",
      "appearance.font",
      "appearance.radius",
      "appearance.header_color",
      "appearance.bubble_color",
      "appearance.pattern",
      "appearance.logo_url",
      "appearance.custom_css",
      "appearance.z_index",
      "appearance.show_branding",
      "show_team",
      "mobile",
    ],
    keywords:
      "renk color tema theme dark koyu font yazı köşe radius logo desen pattern css z-index marka branding avatar",
  },
  {
    id: "launcher",
    group: "design",
    paths: ["appearance.launcher", "appearance.launcher_color"],
    keywords:
      "balon launcher ikon icon görsel image konum position boyut size etiket label offset",
  },
  {
    id: "texts",
    group: "content",
    paths: [
      "texts",
      "greeting_title",
      "greeting_text",
      "offline_message",
      "locale",
      "reply_time",
    ],
    keywords:
      "metin text karşılama greeting dil language çeviri translation yanıt süresi reply time",
  },
  {
    id: "home",
    group: "content",
    paths: ["home", "quick_replies"],
    keywords:
      "ana ekran home kart card makale article yardım help durum status bağlantı link hızlı yanıt quick reply soru",
  },
  {
    id: "prechat",
    group: "content",
    paths: ["prechat", "attribute_mapping"],
    keywords:
      "ön sohbet prechat form talep formu request form onay consent kvkk gdpr öznitelik attribute eşleme mapping",
  },
  {
    id: "conversation",
    group: "behaviour",
    paths: [
      "csat",
      "csat_options",
      "file_upload",
      "attachments",
      "features",
      "offline",
      "away_mode",
      "rate_limit",
    ],
    keywords:
      "csat memnuniyet anket dosya ek attachment emoji yazıyor typing okundu read ses sound bildirim notification markdown çevrimdışı offline uzakta away hız rate çalışma saatleri business hours",
  },
  {
    id: "channels",
    group: "behaviour",
    paths: ["channels"],
    keywords:
      "kanal channel whatsapp instagram telegram messenger e-posta email telefon phone geri arama callback video",
  },
  {
    id: "ai",
    group: "behaviour",
    paths: ["ai"],
    keywords: "yapay zeka ai asistan bot devir handoff insan human",
  },
  {
    id: "triggers",
    group: "behaviour",
    paths: ["triggers"],
    keywords:
      "tetikleyici trigger proaktif proactive mesaj kaydırma scroll süre time çıkış exit utm",
  },
  {
    id: "targeting",
    group: "behaviour",
    paths: ["targeting"],
    keywords:
      "hedefleme targeting sayfa page url cihaz device mobil ülke country gizle hide göster show",
  },
  {
    id: "privacy",
    group: "security",
    paths: ["privacy"],
    keywords:
      "gizlilik privacy çerez cookie onay consent kvkk gdpr kalıcılık persistence",
  },
  {
    id: "security",
    group: "security",
    paths: ["allowed_domains", "require_identity"],
    keywords:
      "güvenlik security alan adı domain kimlik identity hmac hash gizli secret",
  },
  {
    id: "mobile_apps",
    group: "security",
    paths: [],
    keywords:
      "mobil mobile uygulama app ios android anahtar key paket package bundle react native flutter sdk",
  },
  {
    id: "replay",
    group: "security",
    paths: ["replay"],
    keywords: "oturum kaydı replay ortak tarama cobrowse",
  },
  {
    id: "install",
    group: "security",
    paths: [],
    keywords:
      "kurulum install kod snippet html next react vue wordpress shopify gtm tag manager test",
  },
];

/** Sections matching a search (title and keywords, accent-insensitive). */
export function searchSections(
  query: string,
  titles: Record<SectionId, string>,
): SectionId[] {
  const fold = (s: string) =>
    s
      .toLocaleLowerCase("tr")
      .normalize("NFD")
      .replace(/[̀-ͯ]/g, "")
      .replace(/ı/g, "i");
  const q = fold(query.trim());
  if (!q) return SECTIONS.map((s) => s.id);
  return SECTIONS.filter((s) =>
    fold(`${titles[s.id]} ${s.keywords}`).includes(q),
  ).map((s) => s.id);
}

/** Issue count per section. */
export function sectionIssues(issues: Issue[], id: SectionId): number {
  const section = SECTIONS.find((s) => s.id === id);
  if (!section) return 0;
  return issues.filter(
    (i) =>
      (i.level ?? "error") === "error" &&
      section.paths.some((p) => i.path === p || i.path.startsWith(p + ".")),
  ).length;
}

/** Stable JSON of settings (unsaved-change detection). */
export function fingerprint(v: unknown): string {
  const sort = (x: unknown): unknown =>
    Array.isArray(x)
      ? x.map(sort)
      : x && typeof x === "object"
        ? Object.fromEntries(
            Object.keys(x as object)
              .sort()
              .map((k) => [k, sort((x as Record<string, unknown>)[k])]),
          )
        : x;
  return JSON.stringify(sort(v));
}

// ---------------------------------------------------------------------------
// Replay helpers (CF-126)

/** One CSS selector per line; blank lines are dropped. */
export function parseSelectors(text: string): string[] {
  return Array.from(
    new Set(
      text
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean),
    ),
  );
}

/**
 * Mirrors the widget recorder: an invalid selector fails closed there, so
 * reject it here. Returns the first offending selector.
 */
export function invalidSelector(list: string[]): string | null {
  for (const s of list) {
    if (s.length > REPLAY_LIMITS.maxSelectorLength) return s;
    if (typeof document !== "undefined") {
      try {
        document.createDocumentFragment().querySelector(s);
      } catch {
        return s;
      }
    }
  }
  return null;
}

/** Splits the allowlist textarea into normalized host patterns. */
export function parseDomains(text: string): string[] {
  return Array.from(
    new Set(
      text
        .split(/[\s,]+/)
        .map((d) => d.trim().toLowerCase())
        .filter(Boolean),
    ),
  );
}

/** A random identity secret (hex, 32 bytes). */
export function generateSecret(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

/** A short random id for triggers, cards and fields. */
export function newId(prefix: string): string {
  const bytes = new Uint8Array(4);
  crypto.getRandomValues(bytes);
  return (
    prefix +
    "_" +
    Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("")
  );
}

// ---------------------------------------------------------------------------
// Install snippets

export const PLATFORMS = [
  "html",
  "nextjs",
  "react",
  "vue",
  "wordpress",
  "shopify",
  "gtm",
  "identify",
] as const;
export type Platform = (typeof PLATFORMS)[number];

/** The embed snippet of a widget for this app origin (unchanged since CF-55). */
export function installSnippet(origin: string, widgetId: string) {
  return `<script src="${origin}/widget.js" data-widget="${widgetId}" async></script>`;
}

/** Install code per platform. */
export function snippet(
  platform: Platform,
  origin: string,
  widgetId: string,
): string {
  const tag = installSnippet(origin, widgetId);
  switch (platform) {
    case "html":
      return `<!-- ClickFix widget: paste before </body> -->\n${tag}`;
    case "nextjs":
      return [
        "// app/layout.tsx",
        'import Script from "next/script";',
        "",
        "export default function RootLayout({ children }: { children: React.ReactNode }) {",
        "  return (",
        '    <html lang="tr">',
        "      <body>",
        "        {children}",
        "        <Script",
        `          src="${origin}/widget.js"`,
        `          data-widget="${widgetId}"`,
        '          strategy="afterInteractive"',
        "        />",
        "      </body>",
        "    </html>",
        "  );",
        "}",
      ].join("\n");
    case "react":
      return [
        'import { useClickFix } from "@clickfix/react-widget";',
        "",
        `const options = { appOrigin: "${origin}", widgetId: "${widgetId}" };`,
        "",
        "export function Support() {",
        "  const { unread, sdk } = useClickFix(options);",
        "  return (",
        '    <button onClick={() => sdk("open")}>',
        "      Destek {unread > 0 ? `(${unread})` : null}",
        "    </button>",
        "  );",
        "}",
      ].join("\n");
    case "vue":
      return [
        "<!-- App.vue -->",
        '<script setup lang="ts">',
        'import { onMounted, onBeforeUnmount } from "vue";',
        "",
        "onMounted(() => {",
        '  const s = document.createElement("script");',
        `  s.src = "${origin}/widget.js";`,
        `  s.dataset.widget = "${widgetId}";`,
        "  s.async = true;",
        "  document.body.appendChild(s);",
        "});",
        'onBeforeUnmount(() => window.ClickFix?.("shutdown"));',
        "</script>",
      ].join("\n");
    case "wordpress":
      return [
        "<?php",
        "// functions.php of your (child) theme",
        "add_action('wp_footer', function () {",
        `  echo '<script src="${origin}/widget.js" data-widget="${widgetId}" async></script>';`,
        "});",
      ].join("\n");
    case "shopify":
      return [
        "{% comment %} Online Store → Themes → Edit code → layout/theme.liquid, before </body> {% endcomment %}",
        tag,
        "{% if customer %}",
        "<script>",
        "  window.ClickFix = window.ClickFix || function () { (window.ClickFix.q = window.ClickFix.q || []).push(arguments); };",
        '  ClickFix("update", { name: {{ customer.name | json }}, email: {{ customer.email | json }} });',
        "</script>",
        "{% endif %}",
      ].join("\n");
    case "gtm":
      return [
        "<!-- Google Tag Manager → New tag → Custom HTML, trigger: All Pages -->",
        "<script>",
        "  (function () {",
        '    var s = document.createElement("script");',
        `    s.src = "${origin}/widget.js";`,
        `    s.setAttribute("data-widget", "${widgetId}");`,
        "    s.async = true;",
        "    document.body.appendChild(s);",
        "  })();",
        "</script>",
      ].join("\n");
    case "identify":
      return identifySnippet();
  }
}

/** The identify call example for verified users. */
export function identifySnippet() {
  return [
    "<script>",
    "  window.ClickFix = window.ClickFix || function () {",
    "    (window.ClickFix.q = window.ClickFix.q || []).push(arguments);",
    "  };",
    '  ClickFix("identify", {',
    '    user_id: "USER_ID",',
    "    // hex(HMAC-SHA256(identity_secret, user_id)), computed on your server",
    '    user_hash: "USER_HASH",',
    '    name: "Ada Yılmaz",',
    '    email: "ada@example.com",',
    '    company: { id: "acme", name: "Acme A.Ş.", domain: "acme.com" },',
    '    attributes: { plan: "pro" },',
    "  });",
    "</script>",
  ].join("\n");
}

/** Server-side user_hash examples. */
export function hashSnippet(lang: "node" | "go" | "php" | "python"): string {
  switch (lang) {
    case "node":
      return 'import { createHmac } from "node:crypto";\nconst userHash = createHmac("sha256", process.env.CLICKFIX_IDENTITY_SECRET!).update(user.id).digest("hex");';
    case "go":
      return 'mac := hmac.New(sha256.New, []byte(os.Getenv("CLICKFIX_IDENTITY_SECRET")))\nmac.Write([]byte(user.ID))\nuserHash := hex.EncodeToString(mac.Sum(nil))';
    case "php":
      return "$userHash = hash_hmac('sha256', $user->id, getenv('CLICKFIX_IDENTITY_SECRET'));";
    case "python":
      return 'user_hash = hmac.new(os.environ["CLICKFIX_IDENTITY_SECRET"].encode(), user.id.encode(), hashlib.sha256).hexdigest()';
  }
}

/** Seconds since the install was last seen → freshness class. */
export function installFreshness(
  lastSeen: string,
  now = Date.now(),
): "live" | "recent" | "stale" {
  const age = now - new Date(lastSeen).getTime();
  if (age < 15 * 60 * 1000) return "live";
  if (age < 7 * 24 * 3600 * 1000) return "recent";
  return "stale";
}

/**
 * Which identity warning the editor shows for the widget's last rejected
 * identify call (CF-176). A "no secret" rejection is stale once a secret
 * exists: nothing failed since, so there is nothing to warn about.
 */
export function identityWarning(
  rejection: { code: string } | null | undefined,
  hasSecret: boolean,
): "disabled" | "invalid" | null {
  if (!rejection) return null;
  if (rejection.code === "WIDGET_IDENTITY_INVALID") return "invalid";
  if (rejection.code === "WIDGET_IDENTITY_DISABLED")
    return hasSecret ? null : "disabled";
  return null;
}

// ---------------------------------------------------------------------------
// Basic set-up (CF-198): three steps a new widget needs before it goes live.
// Everything else is the advanced editor; every basic path is also an
// advanced path (tested), so the two views never disagree.

export type BasicStepId = "appearance" | "identity" | "channel";

export const BASIC_STEPS: { id: BasicStepId; paths: string[] }[] = [
  {
    id: "appearance",
    paths: ["color", "position", "appearance.theme", "appearance.launcher"],
  },
  {
    id: "identity",
    paths: ["locale", "texts", "appearance.logo_url", "show_team"],
  },
  {
    id: "channel",
    paths: [
      "ai.enabled",
      "prechat.enabled",
      "prechat.form_uuid",
      "allowed_domains",
    ],
  },
];

/** The advanced section that edits a basic path (undefined: none). */
export function advancedSectionFor(path: string): SectionId | undefined {
  return SECTIONS.find((s) =>
    s.paths.some((p) => path === p || path.startsWith(`${p}.`)),
  )?.id;
}

/** Issues of a basic step (its paths and their children). */
export function basicStepIssues(issues: Issue[], step: BasicStepId): number {
  const paths = BASIC_STEPS.find((s) => s.id === step)?.paths ?? [];
  return issues.filter(
    (i) =>
      (i.level ?? "error") === "error" &&
      paths.some((p) => i.path === p || i.path.startsWith(`${p}.`)),
  ).length;
}
