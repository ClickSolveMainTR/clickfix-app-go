/** Health score colour: below the organization's threshold is a problem, under 70 a warning. */
export function healthTone(score: number | null | undefined, below?: boolean) {
  if (score == null) return { bg: 'bg-secondary', key: 'unknown' as const };
  if (below || score < 40) return { bg: 'bg-destructive/20', key: 'risk' as const };
  if (score < 70) return { bg: 'bg-warning/20', key: 'watch' as const };
  return { bg: 'bg-success/20', key: 'healthy' as const };
}
