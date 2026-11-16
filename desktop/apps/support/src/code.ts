/** Uzaktan destek kodu 9 hanelidir (docs/design/masaustu.md, 5. Akış). */
export const CODE_LENGTH = 9;

/** Girdiden yalnız rakamları alır ve 9 haneyle sınırlar. */
export function normalizeCode(input: string): string {
  return input.replace(/\D/g, "").slice(0, CODE_LENGTH);
}

/** "123456789" -> "123 456 789" (okunurluk için 3'lü gruplar). */
export function formatCode(digits: string): string {
  return (digits.match(/.{1,3}/g) ?? []).join(" ");
}

export function isCompleteCode(digits: string): boolean {
  return digits.length === CODE_LENGTH;
}
