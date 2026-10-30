"use client";

import { useAuth } from "@/providers/auth-provider";
import type { MeOrganization } from "@/lib/auth/types";

import {
  resolveActive,
  useActiveOrganizationStore,
} from "./active-organization-store";

/** The user's organizations, the active one and a setter. */
export function useActiveOrganization(): {
  organizations: MeOrganization[];
  active: MeOrganization | null;
  select: (slug: string) => void;
} {
  const { user } = useAuth();
  const slug = useActiveOrganizationStore((s) => s.slug);
  const setSlug = useActiveOrganizationStore((s) => s.setSlug);
  const organizations = user?.organizations ?? [];
  return {
    organizations,
    active: resolveActive(organizations, slug),
    select: setSlug,
  };
}
