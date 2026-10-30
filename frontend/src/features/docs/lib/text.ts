const TR_ASCII: Record<string, string> = {
  ç: "c",
  ğ: "g",
  ı: "i",
  İ: "i",
  ö: "o",
  ş: "s",
  ü: "u",
  â: "a",
  î: "i",
  û: "u",
};

/**
 * Lowercase, Turkish letters folded to ASCII. Used for heading anchors and
 * for search, so "kvkk" finds "KVKK" and "sifre" finds "şifre".
 */
export function fold(text: string) {
  return text
    .toLocaleLowerCase("tr")
    .replace(/[çğıİöşüâîû]/g, (c) => TR_ASCII[c] ?? c)
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "");
}

/** Anchor id of a heading (same on the server TOC and the rendered h2/h3). */
export function headingId(text: string) {
  return (
    fold(text)
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "")
      .slice(0, 80) || "bolum"
  );
}

/** Markdown reduced to readable text (search index, heading labels). */
export function plainText(markdown: string) {
  return markdown
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/!\[[^\]]*]\([^)]*\)/g, " ")
    .replace(/\[([^\]]+)]\([^)]*\)/g, "$1")
    .replace(/`([^`]+)`/g, "$1")
    .replace(/^#{1,6}\s+/gm, "")
    .replace(/^\s*(?:[-*+]|\d+\.)\s+/gm, "")
    .replace(/^\s*>\s?/gm, "")
    .replace(/[*_~|]/g, "")
    .replace(/\s+/g, " ")
    .trim();
}
