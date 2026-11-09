/** Turns a label into a key the API accepts ("Faturam gelmedi" → "faturam_gelmedi"). */
export function toKey(label: string, max = 50): string {
  const map: Record<string, string> = {
    ç: "c",
    ğ: "g",
    ı: "i",
    ö: "o",
    ş: "s",
    ü: "u",
  };
  const key = label
    .toLocaleLowerCase("tr")
    .replace(/[çğıöşü]/g, (c) => map[c] ?? c)
    .normalize("NFKD")
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "")
    .slice(0, max);
  return /^[a-z]/.test(key) ? key : key ? `k_${key}`.slice(0, max) : "";
}

export const KEY_PATTERN = /^[a-z][a-z0-9_]{1,49}$/;
export const INTENT_PATTERN = /^[a-z][a-z0-9_]{1,63}$/;
export const PARAM_PATTERN = /^[a-z][a-z0-9_]{0,39}$/;
