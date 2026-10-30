const TURKISH: Record<string, string> = {
  ç: "c",
  Ç: "c",
  ğ: "g",
  Ğ: "g",
  ı: "i",
  I: "i",
  İ: "i",
  ö: "o",
  Ö: "o",
  ş: "s",
  Ş: "s",
  ü: "u",
  Ü: "u",
};

/** Same rule as the API (`^[a-z0-9]+(-[a-z0-9]+)*$`, at most 200 characters). */
export const SLUG_PATTERN = /^[a-z0-9]+(-[a-z0-9]+)*$/;
export const SLUG_MAX = 200;

/**
 * URL slug from a title, Turkish-safe: "İade nasıl yapılır?" →
 * "iade-nasil-yapilir" (plain NFD stripping would drop the dotless ı).
 */
export function slugify(value: string): string {
  return value
    .replace(/[çÇğĞıIİöÖşŞüÜ]/g, (c) => TURKISH[c] ?? c)
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, SLUG_MAX)
    .replace(/-+$/g, "");
}

export function isValidSlug(value: string): boolean {
  return value.length <= SLUG_MAX && SLUG_PATTERN.test(value);
}
