/**
 * Custom CSS sanitizer, the twin of backend .../widget/css.go (same order,
 * same rules; tested against the shared fixture). The CSS only ever lands
 * inside the widget's Shadow DOM.
 */
const MAX = 8000;

export function sanitizeCSS(input: string): {
  css: string;
  problems: string[];
} {
  const problems: string[] = [];
  const note = (p: string) => {
    if (!problems.includes(p)) problems.push(p);
  };
  let css = input;
  if (Array.from(css).length > MAX) {
    css = Array.from(css).slice(0, MAX).join("");
    note("custom CSS is too long");
  }
  css = css.replace(/\/\*[\s\S]*?\*\//g, "");
  if (css.includes("\\")) {
    css = css.split("\\").join("");
    note("escape sequences are not allowed");
  }
  if (css.includes("<")) {
    css = css.split("<").join("");
    note("HTML is not allowed");
  }
  const atRule = /@(import|charset|namespace)[^;{}]*;?/gi;
  if (atRule.test(css)) {
    css = css.replace(atRule, "");
    note("@import, @charset and @namespace are not allowed");
  }
  const expr = /expression\s*\(/gi;
  if (expr.test(css)) {
    css = css.replace(expr, "invalid(");
    note("expression() is not allowed");
  }
  const scheme = /(javascript|vbscript)\s*:/gi;
  if (scheme.test(css)) {
    css = css.replace(scheme, "");
    note("script URLs are not allowed");
  }
  const binding = /(-moz-binding|behavior)\s*:/gi;
  if (binding.test(css)) {
    css = css.replace(binding, "x-blocked:");
    note("behavior and -moz-binding are not allowed");
  }
  css = css.replace(
    /url\(\s*(['"]?)([^'")]*)(['"]?)\s*\)/gi,
    (m, _q: string, target: string) => {
      if (/^(https:\/\/|data:image\/(png|jpeg|gif|webp);)/i.test(target.trim()))
        return m;
      note("only https:// and data:image URLs are allowed");
      return "none";
    },
  );
  return { css: css.trim(), problems };
}
