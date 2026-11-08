/** Pure helpers of the confirmation card v2 (CF-184). */

export type FieldType =
  "string" | "url" | "number" | "boolean" | "enum" | "json" | "secret";

const TARGET = /^[a-z0-9][a-z0-9_.-]{0,59}$/;

/** A data-guide key the page may highlight. */
export function validTarget(target?: string | null): target is string {
  return Boolean(target && TARGET.test(target));
}

/**
 * Link of "go to the page (and highlight)": internal paths only, never a
 * URL or a protocol-relative path; `?focus=` carries the target.
 */
export function focusHref(route: string, target?: string | null) {
  if (
    !route.startsWith("/") ||
    route.startsWith("//") ||
    route.includes("\\") ||
    /[?#]/.test(route)
  )
    return null;
  if (!validTarget(target)) return route;
  return `${route}?focus=${encodeURIComponent(target)}`;
}

export function toText(v: unknown): string {
  if (v === undefined || v === null) return "";
  if (typeof v === "object") return JSON.stringify(v, null, 2);
  return String(v);
}

/**
 * A form value read back into a param: undefined when empty, null when a
 * JSON field does not parse. The server validates everything again.
 */
export function readValue(type: FieldType | string, raw: string | boolean) {
  if (type === "boolean") return Boolean(raw);
  const text = String(raw).trim();
  if (text === "") return undefined;
  if (type === "number") {
    const n = Number(text);
    return Number.isFinite(n) ? n : text;
  }
  if (type === "json") {
    try {
      return JSON.parse(text) as unknown;
    } catch {
      return null;
    }
  }
  return text;
}
