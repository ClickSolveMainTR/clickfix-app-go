/**
 * Page targeting and proactive trigger rules, the browser twin of
 * backend/internal/platform/channels/widget/targeting.go. Both are tested
 * against backend/.../widget/testdata/targeting_cases.json.
 */
import type { Targeting, Trigger, URLRule } from "./schema";

export type PageContext = {
  url?: string;
  device?: string;
  country?: string;
  language?: string;
  returning?: boolean;
  identified?: boolean;
};

const MOBILE_UA =
  /Mobi|Android|iPhone|iPod|Windows Phone|BlackBerry|Opera Mini/i;

/** "mobile" or "desktop" (tablets count as desktop). */
export function deviceOf(userAgent: string): "mobile" | "desktop" {
  return MOBILE_UA.test(userAgent) && !/ipad/i.test(userAgent)
    ? "mobile"
    : "desktop";
}

function pathAndQuery(target: string): string {
  try {
    const u = new URL(target);
    return (u.pathname || "/") + u.search;
  } catch {
    return target;
  }
}

function globMatch(pattern: string, s: string): boolean {
  const parts = pattern.split("*");
  if (parts.length === 1) return pattern === s;
  if (!s.startsWith(parts[0]!)) return false;
  s = s.slice(parts[0]!.length);
  for (const p of parts.slice(1, -1)) {
    const i = s.indexOf(p);
    if (i < 0) return false;
    s = s.slice(i + p.length);
  }
  return s.endsWith(parts[parts.length - 1]!);
}

const trimSlash = (s: string) => (s.endsWith("/") ? s.slice(0, -1) : s);

/** Whether a page URL matches one rule (case-insensitive, no fragment;
 * a value starting with "/" compares path + query only). */
export function matchURL(rule: URLRule, page: string): boolean {
  let target = page.toLowerCase();
  const hash = target.indexOf("#");
  if (hash >= 0) target = target.slice(0, hash);
  const value = rule.value.trim().toLowerCase();
  if (value.startsWith("/")) target = pathAndQuery(target);
  switch (rule.op) {
    case "equals":
      return trimSlash(target) === trimSlash(value);
    case "starts_with":
      return target.startsWith(value);
    case "ends_with":
      return target.endsWith(value);
    case "glob":
      return globMatch(value, target);
    case "not_contains":
      return !target.includes(value);
    default:
      return target.includes(value);
  }
}

/** Positive rules: at least one must match; not_contains: all must hold. */
export function matchRules(rules: URLRule[], page: string): boolean {
  let positive = false;
  let hit = false;
  for (const r of rules) {
    if (r.op === "not_contains") {
      if (!matchURL(r, page)) return false;
      continue;
    }
    positive = true;
    if (matchURL(r, page)) hit = true;
  }
  return !positive || hit;
}

export function visible(
  t: Targeting,
  ctx: PageContext,
): { visible: boolean; reason: string } {
  if (
    t.devices &&
    t.devices !== "all" &&
    ctx.device &&
    t.devices !== ctx.device
  )
    return { visible: false, reason: "device" };
  const cc = (ctx.country ?? "").toUpperCase();
  if (cc) {
    const inList = t.countries.list.includes(cc);
    if (
      (t.countries.mode === "allow" && !inList) ||
      (t.countries.mode === "block" && inList)
    )
      return { visible: false, reason: "country" };
  }
  if (ctx.url) {
    if (t.hide_on.some((r) => matchURL(r, ctx.url!)))
      return { visible: false, reason: "hidden_page" };
    if (t.show_on.length && !matchRules(t.show_on, ctx.url))
      return { visible: false, reason: "not_listed_page" };
  }
  return { visible: true, reason: "" };
}

/** Page-level trigger conditions (time, scroll and exit intent are
 * checked by the runtime's trigger engine). */
export function eligible(t: Trigger, ctx: PageContext): boolean {
  const c = t.conditions;
  if (!t.enabled) return false;
  if (ctx.url && !matchRules(c.url ?? [], ctx.url)) return false;
  if (c.visitor === "new" && ctx.returning) return false;
  if (c.visitor === "returning" && !ctx.returning) return false;
  if (c.device && c.device !== "any" && ctx.device && c.device !== ctx.device)
    return false;
  if (c.identified === "yes" && !ctx.identified) return false;
  if (c.identified === "no" && ctx.identified) return false;
  const lang = (ctx.language ?? "").toLowerCase();
  if (c.languages?.length && lang && !c.languages.includes(lang)) return false;
  const cc = (ctx.country ?? "").toUpperCase();
  if (c.countries?.length && cc && !c.countries.includes(cc)) return false;
  if (c.utm_source || c.utm_medium || c.utm_campaign) {
    let q = new URLSearchParams();
    try {
      q = new URL(ctx.url ?? "").searchParams;
    } catch {
      /* no URL: no UTM */
    }
    const want: [string, string | undefined][] = [
      ["utm_source", c.utm_source],
      ["utm_medium", c.utm_medium],
      ["utm_campaign", c.utm_campaign],
    ];
    for (const [k, v] of want)
      if (v && (q.get(k) ?? "").toLowerCase() !== v.toLowerCase()) return false;
  }
  return true;
}
