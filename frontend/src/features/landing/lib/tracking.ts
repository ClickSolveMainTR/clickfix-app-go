/**
 * Cookie consent, marketing tags and sign-up attribution of the public
 * marketing site (CF-144). Browser only; nothing here runs before the
 * visitor chose their cookie categories (KVKK).
 */

/** Tag ids and sanitized snippets from "Pazarlama & izleme". */
export type TrackingConfig = {
  gtm_id: string;
  ga4_id: string;
  meta_pixel_id: string;
  linkedin_partner_id: string;
  tiktok_pixel_id: string;
  google_ads_id: string;
  google_ads_signup_label: string;
  clarity_id: string;
  hotjar_id: string;
  custom_head: string;
  custom_body: string;
};

/** Cookie categories; necessary cookies (session, consent itself) are always on. */
export type Consent = { analytics: boolean; marketing: boolean };

export const CONSENT_COOKIE = "cf_consent";
/** Window event that reopens the preferences dialog (footer link). */
export const CONSENT_OPEN_EVENT = "cf:consent-open";
export const CONSENT_CHANGE_EVENT = "cf:consent-change";
/** First / last touch cookies, read by the BFF on sign-up. */
export const FIRST_TOUCH_COOKIE = "cf_ft";
export const LAST_TOUCH_COOKIE = "cf_lt";
/** Set by the BFF after a successful sign-up; the conversion fires once. */
export const CONVERSION_COOKIE = "cf_conv";

const CONSENT_VERSION = "1";
const DAY = 24 * 60 * 60;

function readCookie(name: string): string | null {
  if (typeof document === "undefined") return null;
  const prefix = `${name}=`;
  for (const part of document.cookie.split("; ")) {
    if (part.startsWith(prefix)) return part.slice(prefix.length);
  }
  return null;
}

function writeCookie(name: string, value: string, maxAge: number) {
  const secure = location.protocol === "https:" ? "; Secure" : "";
  document.cookie = `${name}=${value}; Path=/; Max-Age=${maxAge}; SameSite=Lax${secure}`;
}

export function deleteCookie(name: string) {
  writeCookie(name, "", 0);
}

/** Raw consent cookie (a stable snapshot for `useSyncExternalStore`). */
export function consentSnapshot(): string {
  return readCookie(CONSENT_COOKIE) ?? "";
}

/** Notifies when the choice changes in this tab. */
export function subscribeConsent(callback: () => void) {
  window.addEventListener(CONSENT_CHANGE_EVENT, callback);
  return () => window.removeEventListener(CONSENT_CHANGE_EVENT, callback);
}

/** Stored choice, or null while the visitor has not decided. */
export function readConsent(): Consent | null {
  return parseConsent(readCookie(CONSENT_COOKIE));
}

export function parseConsent(raw: string | null): Consent | null {
  const match = raw?.match(/^v(\d+)\.a([01])m([01])$/);
  if (!match || match[1] !== CONSENT_VERSION) return null;
  return { analytics: match[2] === "1", marketing: match[3] === "1" };
}

export function writeConsent(consent: Consent) {
  writeCookie(
    CONSENT_COOKIE,
    `v${CONSENT_VERSION}.a${consent.analytics ? 1 : 0}m${consent.marketing ? 1 : 0}`,
    180 * DAY,
  );
  window.dispatchEvent(
    new CustomEvent<Consent>(CONSENT_CHANGE_EVENT, { detail: consent }),
  );
}

export function hasAnyTag(config: TrackingConfig) {
  return Object.values(config).some(Boolean);
}

// ---------------------------------------------------------------------------
// Attribution

export type Touch = {
  source?: string;
  medium?: string;
  campaign?: string;
  term?: string;
  content?: string;
  gclid?: string;
  fbclid?: string;
  msclkid?: string;
  referrer?: string;
  landing?: string;
  at?: string;
};

const TOUCH_PARAMS = {
  utm_source: "source",
  utm_medium: "medium",
  utm_campaign: "campaign",
  utm_term: "term",
  utm_content: "content",
  gclid: "gclid",
  fbclid: "fbclid",
  msclkid: "msclkid",
} as const satisfies Record<string, keyof Touch>;

const PENDING_KEY = "cf_touch_pending";

/**
 * The touch of this page view, or null when it is an internal navigation
 * without campaign parameters (nothing new to attribute).
 */
export function currentTouch(): Touch | null {
  const params = new URLSearchParams(location.search);
  const touch: Touch = {};
  for (const [param, field] of Object.entries(TOUCH_PARAMS)) {
    const value = params.get(param)?.trim();
    if (value) touch[field] = value.slice(0, 200);
  }
  let external = false;
  if (document.referrer) {
    try {
      const ref = new URL(document.referrer);
      external = ref.host !== location.host;
      if (external) touch.referrer = `${ref.origin}${ref.pathname}`;
    } catch {
      // Unparseable referrer: ignore it.
    }
  }
  if (Object.keys(touch).length === 0 && !isSessionStart()) return null;
  touch.landing = location.pathname.slice(0, 300);
  touch.at = new Date().toISOString().replace(/\.\d+Z$/, "Z");
  return touch;
}

function isSessionStart() {
  try {
    if (sessionStorage.getItem("cf_session")) return false;
    sessionStorage.setItem("cf_session", "1");
  } catch {
    // Storage blocked: treat every page as a new visit.
  }
  return true;
}

function encodeTouch(touch: Touch) {
  return encodeURIComponent(JSON.stringify(touch));
}

/**
 * Stores the touch: cookies once analytics is allowed (first touch is never
 * overwritten), otherwise kept in this tab until the visitor decides.
 */
export function recordTouch(touch: Touch | null, consent: Consent | null) {
  if (!consent?.analytics) {
    if (touch) {
      try {
        sessionStorage.setItem(PENDING_KEY, JSON.stringify(touch));
      } catch {
        // Nothing to keep it in.
      }
    }
    return;
  }
  let pending: Touch | null = touch;
  if (!pending) {
    try {
      const raw = sessionStorage.getItem(PENDING_KEY);
      pending = raw ? (JSON.parse(raw) as Touch) : null;
    } catch {
      pending = null;
    }
  }
  try {
    sessionStorage.removeItem(PENDING_KEY);
  } catch {
    // Ignore.
  }
  if (!pending) return;
  if (!readCookie(FIRST_TOUCH_COOKIE)) {
    writeCookie(FIRST_TOUCH_COOKIE, encodeTouch(pending), 180 * DAY);
  }
  writeCookie(LAST_TOUCH_COOKIE, encodeTouch(pending), 30 * DAY);
}

export function clearAttribution() {
  deleteCookie(FIRST_TOUCH_COOKIE);
  deleteCookie(LAST_TOUCH_COOKIE);
  try {
    sessionStorage.removeItem(PENDING_KEY);
  } catch {
    // Ignore.
  }
}

// ---------------------------------------------------------------------------
// Tags

type Fn = ((...args: unknown[]) => void) & Record<string, unknown>;
type TagWindow = Window & {
  dataLayer?: unknown[];
  gtag?: (...args: unknown[]) => void;
  fbq?: Fn;
  _fbq?: Fn;
  ttq?: Fn & unknown[];
  TiktokAnalyticsObject?: string;
  clarity?: Fn;
  hj?: Fn;
  _hjSettings?: { hjid: number; hjsv: number };
  _linkedin_partner_id?: string;
  _linkedin_data_partner_ids?: string[];
  [key: `ga-disable-${string}`]: boolean;
};

function w() {
  return window as unknown as TagWindow;
}

function loadScript(src: string, id: string) {
  if (document.getElementById(id)) return;
  const script = document.createElement("script");
  script.id = id;
  script.async = true;
  script.src = src;
  document.head.appendChild(script);
}

/** The standard gtag stub: Google's loaders expect `arguments` objects. */
function gtag() {
  const win = w();
  win.dataLayer = win.dataLayer || [];
  // eslint-disable-next-line prefer-rest-params
  win.dataLayer.push(arguments);
}

/** Google Consent Mode v2 state for the chosen categories. */
function googleConsent(consent: Consent) {
  const ads = consent.marketing ? "granted" : "denied";
  return {
    analytics_storage: consent.analytics ? "granted" : "denied",
    ad_storage: ads,
    ad_user_data: ads,
    ad_personalization: ads,
  };
}

const loaded = new Set<string>();

/**
 * Loads every configured tag the consent allows; safe to call again after
 * the choice changes (already loaded tags are skipped, consent is updated).
 * Withdrawing a category reloads the page so nothing loaded keeps running.
 */
export function applyTags(config: TrackingConfig, consent: Consent) {
  const win = w();
  const google = Boolean(
    config.gtm_id || config.ga4_id || config.google_ads_id,
  );
  if (google && (consent.analytics || consent.marketing)) {
    win.gtag = win.gtag || (gtag as (...args: unknown[]) => void);
    if (!loaded.has("google")) {
      win.gtag("consent", "default", {
        ...googleConsent({ analytics: false, marketing: false }),
        wait_for_update: 500,
      });
      loaded.add("google");
    }
    win.gtag("consent", "update", googleConsent(consent));
    if (config.ga4_id) win[`ga-disable-${config.ga4_id}`] = false;
    if (config.gtm_id && !loaded.has("gtm")) {
      win.dataLayer!.push({ "gtm.start": Date.now(), event: "gtm.js" });
      loadScript(
        `https://www.googletagmanager.com/gtm.js?id=${encodeURIComponent(config.gtm_id)}`,
        "cf-gtm",
      );
      loaded.add("gtm");
    }
    const gaAllowed = config.ga4_id && consent.analytics;
    const adsAllowed = config.google_ads_id && consent.marketing;
    if ((gaAllowed || adsAllowed) && !loaded.has("gtag")) {
      win.gtag("js", new Date());
      loadScript(
        `https://www.googletagmanager.com/gtag/js?id=${encodeURIComponent(config.ga4_id || config.google_ads_id)}`,
        "cf-gtag",
      );
      loaded.add("gtag");
    }
    if (gaAllowed && !loaded.has("ga4")) {
      win.gtag("config", config.ga4_id);
      loaded.add("ga4");
    }
    if (adsAllowed && !loaded.has("ads")) {
      win.gtag("config", config.google_ads_id);
      loaded.add("ads");
    }
  }

  if (consent.analytics) {
    if (config.clarity_id && !loaded.has("clarity")) {
      const fn = function (...args: unknown[]) {
        (fn.q = (fn.q as unknown[]) || []).push(args);
      } as Fn;
      win.clarity = win.clarity || fn;
      loadScript(
        `https://www.clarity.ms/tag/${encodeURIComponent(config.clarity_id)}`,
        "cf-clarity",
      );
      win.clarity("consent");
      loaded.add("clarity");
    }
    if (config.hotjar_id && !loaded.has("hotjar")) {
      const fn = function (...args: unknown[]) {
        (fn.q = (fn.q as unknown[]) || []).push(args);
      } as Fn;
      win.hj = win.hj || fn;
      win._hjSettings = { hjid: Number(config.hotjar_id), hjsv: 6 };
      loadScript(
        `https://static.hotjar.com/c/hotjar-${encodeURIComponent(config.hotjar_id)}.js?sv=6`,
        "cf-hotjar",
      );
      loaded.add("hotjar");
    }
  }

  if (consent.marketing) {
    if (config.meta_pixel_id && !loaded.has("meta")) {
      if (!win.fbq) {
        const fbq = function (...args: unknown[]) {
          if (fbq.callMethod) {
            (fbq.callMethod as (...a: unknown[]) => void)(...args);
          } else {
            (fbq.queue as unknown[]).push(args);
          }
        } as Fn;
        fbq.push = fbq;
        fbq.loaded = true;
        fbq.version = "2.0";
        fbq.queue = [];
        win.fbq = fbq;
        win._fbq = win._fbq || fbq;
      }
      loadScript("https://connect.facebook.net/en_US/fbevents.js", "cf-meta");
      win.fbq("consent", "grant");
      win.fbq("init", config.meta_pixel_id);
      win.fbq("track", "PageView");
      loaded.add("meta");
    }
    if (config.linkedin_partner_id && !loaded.has("linkedin")) {
      win._linkedin_partner_id = config.linkedin_partner_id;
      win._linkedin_data_partner_ids = win._linkedin_data_partner_ids || [];
      win._linkedin_data_partner_ids.push(config.linkedin_partner_id);
      loadScript(
        "https://snap.licdn.com/li.lms-analytics/insight.min.js",
        "cf-linkedin",
      );
      loaded.add("linkedin");
    }
    if (config.tiktok_pixel_id && !loaded.has("tiktok")) {
      loadTikTok(config.tiktok_pixel_id);
      loaded.add("tiktok");
    }
    if (!loaded.has("custom")) {
      injectSnippet(config.custom_head, document.head);
      injectSnippet(config.custom_body, document.body);
      loaded.add("custom");
    }
  }
}

/** Whether a stricter choice needs a reload to stop already loaded tags. */
export function needsReload(previous: Consent | null, next: Consent) {
  return (
    loaded.size > 0 &&
    !!previous &&
    ((previous.analytics && !next.analytics) ||
      (previous.marketing && !next.marketing))
  );
}

function loadTikTok(id: string) {
  const win = w();
  win.TiktokAnalyticsObject = "ttq";
  const ttq = (win.ttq || []) as unknown as Fn & unknown[];
  win.ttq = ttq;
  const methods = [
    "page",
    "track",
    "identify",
    "instances",
    "debug",
    "on",
    "off",
    "once",
    "ready",
    "alias",
    "group",
    "enableCookie",
    "disableCookie",
    "holdConsent",
    "revokeConsent",
    "grantConsent",
  ];
  for (const method of methods) {
    ttq[method] = (...args: unknown[]) => {
      ttq.push([method, ...args]);
    };
  }
  const src = "https://analytics.tiktok.com/i18n/pixel/events.js";
  ttq._i = {
    ...((ttq._i as object) || {}),
    [id]: Object.assign([], { _u: src }),
  };
  ttq._t = { ...((ttq._t as object) || {}), [id]: Date.now() };
  ttq._o = { ...((ttq._o as object) || {}), [id]: {} };
  loadScript(`${src}?sdkid=${encodeURIComponent(id)}&lib=ttq`, "cf-tiktok");
  (ttq.page as () => void)();
}

/**
 * Inserts an admin snippet (already sanitized by the API). Scripts parsed
 * from HTML do not run, so each one is recreated.
 */
function injectSnippet(html: string, target: HTMLElement) {
  if (!html) return;
  const template = document.createElement("template");
  template.innerHTML = html;
  for (const node of Array.from(template.content.childNodes)) {
    // `<meta>` tags are already in the server-rendered head (`headMetaTags`).
    if (node instanceof HTMLMetaElement) continue;
    if (node instanceof HTMLScriptElement) {
      const script = document.createElement("script");
      for (const attr of Array.from(node.attributes)) {
        script.setAttribute(attr.name, attr.value);
      }
      script.text = node.text;
      script.dataset.cfSnippet = "1";
      target.appendChild(script);
    } else {
      target.appendChild(node);
    }
  }
}

/** Page view after a client-side navigation between marketing pages. */
export function trackPageView() {
  const win = w();
  if (loaded.has("meta")) win.fbq?.("track", "PageView");
  if (loaded.has("tiktok")) (win.ttq?.page as (() => void) | undefined)?.();
}

/**
 * Sign-up conversion (once per browser): dataLayer event for GTM, GA4
 * `sign_up`, the Google Ads conversion and the pixels' registration events.
 */
export function trackSignup(config: TrackingConfig, consent: Consent) {
  const win = w();
  applyTags(config, consent);
  win.dataLayer = win.dataLayer || [];
  const details = { method: "clickfix", page_path: window.location.pathname };
  win.dataLayer.push({ event: "sign_up", ...details });
  if (loaded.has("ga4")) win.gtag?.("event", "sign_up", details);
  if (loaded.has("ads") && config.google_ads_signup_label) {
    win.gtag?.("event", "conversion", {
      send_to: `${config.google_ads_id}/${config.google_ads_signup_label}`,
    });
  }
  if (loaded.has("meta")) win.fbq?.("track", "CompleteRegistration");
  if (loaded.has("tiktok"))
    (win.ttq?.track as ((e: string) => void) | undefined)?.(
      "CompleteRegistration",
    );
}

/** The conversion marker the BFF set, cleared once read. */
export function takeConversion(): boolean {
  const value = readCookie(CONVERSION_COOKIE);
  if (!value) return false;
  deleteCookie(CONVERSION_COOKIE);
  try {
    if (localStorage.getItem("cf_conv_done")) return false;
    localStorage.setItem("cf_conv_done", new Date().toISOString());
  } catch {
    // Without storage the cookie alone keeps it to one event per sign-up.
  }
  return true;
}
