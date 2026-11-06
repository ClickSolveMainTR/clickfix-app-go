/**
 * Text normalised for the client-side search box: Turkish-aware lower case
 * (İ -> i, I -> ı), then dotless ı folded into i and diacritics dropped, so
 * "iade" finds "İade", "ısı" finds "Isı" and "sikayet" finds "Şikâyet".
 */
export function normalizeSearchText(value: unknown): string {
  if (value == null) return "";
  return String(value)
    .toLocaleLowerCase("tr")
    .replaceAll("ı", "i")
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "");
}

/** Does `value` contain `query`, ignoring case and diacritics? */
export function searchTextIncludes(value: unknown, query: unknown): boolean {
  const needle = normalizeSearchText(query).trim();
  if (!needle) return true;
  return normalizeSearchText(value).includes(needle);
}
