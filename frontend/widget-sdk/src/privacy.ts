// Strip attributes and URLs as well as text: DOM masking alone does not cover
// href/title/value/data-* attributes or metadata URLs. No plugins/network capture.
export function scrub(value: unknown, key = ""): unknown {
  if (key === "attributes") {
    const attrs = value as Record<string, unknown>;
    const safe: Record<string, unknown> = {};
    for (const name of [
      "width",
      "height",
      "rr_width",
      "rr_height",
      "rr_scrollLeft",
      "rr_scrollTop",
    ]) {
      const v = attrs?.[name];
      if (
        (typeof v === "number" && Number.isFinite(v)) ||
        (typeof v === "string" && /^\d{1,6}(px)?$/.test(v))
      )
        safe[name] = v;
    }
    return safe;
  }
  if (key === "href") return "about:blank";
  if (typeof value === "string") {
    if (key === "textContent" || key === "text")
      return value.replace(/\S/g, "*");
    if (key === "tagName" && /^[a-z][a-z0-9-]{0,40}$/i.test(value))
      return value;
    if (key === "type" && ["text/css", "text/html"].includes(value))
      return value;
    return "";
  }
  if (Array.isArray(value)) return value.map((v) => scrub(v));
  if (value && typeof value === "object") {
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(value)) out[k] = scrub(v, k);
    return out;
  }
  return value;
}
