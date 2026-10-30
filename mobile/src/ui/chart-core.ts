/** Finite non-negative proportions; no data/zero counts never become full-height bars. */
export function barFractions(values: (number | null)[]): number[] {
  const valid = values.map((v) => (v != null && Number.isFinite(v) ? Math.max(0, v) : 0));
  const max = Math.max(1, ...valid);
  return valid.map((v) => v / max);
}
