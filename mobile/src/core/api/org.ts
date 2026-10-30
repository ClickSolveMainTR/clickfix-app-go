import { useSession } from '@/core/auth/session';

/** Slug of the active organization; '' before one is picked (queries stay disabled). */
export function useOrgSlug() {
  return useSession((s) => s.organization?.slug ?? '');
}

/** Query key under the active organization, so an organization switch never shows another's data. */
export const orgKey = (org: string, ...parts: unknown[]) => ['org', org, ...parts] as const;
