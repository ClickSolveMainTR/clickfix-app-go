/** "TR330006100519786457841326" → "TR33 0006 1005 1978 6457 8413 26". */
export function formatIban(iban: string) {
  return iban
    .replace(/\s/g, "")
    .toUpperCase()
    .replace(/(.{4})/g, "$1 ")
    .trim();
}

/**
 * Turkish IBAN check (26 chars, ISO 13616 mod-97). The server validates
 * again (`platform/iban`); this only gives instant feedback in forms.
 */
export function isValidTrIban(raw: string) {
  const iban = raw.replace(/\s/g, "").toUpperCase();
  if (!/^TR\d{24}$/.test(iban)) return false;
  const rearranged = iban.slice(4) + iban.slice(0, 4);
  let remainder = 0;
  for (const ch of rearranged) {
    const code = ch >= "A" && ch <= "Z" ? String(ch.charCodeAt(0) - 55) : ch;
    for (const digit of code) {
      remainder = (remainder * 10 + Number(digit)) % 97;
    }
  }
  return remainder === 1;
}
