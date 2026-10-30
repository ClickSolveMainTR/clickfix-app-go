/** Whether an organization's access window has ended at `now`. */
export function isOrganizationAccessExpired(
  accessEndsAt: string | null | undefined,
  now: number = Date.now(),
): boolean {
  if (!accessEndsAt) return false;
  const ends = Date.parse(accessEndsAt);
  return Number.isFinite(ends) && ends <= now;
}
