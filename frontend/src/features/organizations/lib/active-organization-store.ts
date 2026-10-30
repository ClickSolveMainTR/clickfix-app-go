"use client";

import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

import { useAuthStore } from "@/lib/auth/session-store";
import type { MeOrganization } from "@/lib/auth/types";

/**
 * The organization the workspace shell works in, by slug. Persisted per
 * browser so a reload keeps the choice; the API client sends it as
 * `X-Organization` on `/v1/tenant/*` calls (lib/api/client.ts).
 */
type ActiveOrganizationState = {
  slug: string | null;
  setSlug: (slug: string | null) => void;
};

export const useActiveOrganizationStore = create<ActiveOrganizationState>()(
  persist(
    (set) => ({
      slug: null,
      setSlug: (slug) => set({ slug }),
    }),
    {
      name: "clickfix.active-organization",
      storage: createJSONStorage(() => {
        if (typeof window === "undefined") {
          return {
            getItem: () => null,
            setItem: () => {},
            removeItem: () => {},
          };
        }
        return localStorage;
      }),
    },
  ),
);

/** Slug for the `X-Organization` header outside React (API middleware). */
export function getActiveOrganizationSlug(): string | null {
  const organizations = useAuthStore.getState().user?.organizations ?? [];
  return (
    resolveActive(organizations, useActiveOrganizationStore.getState().slug)
      ?.slug ?? null
  );
}

/** Stored slug when still a member, else the first active membership. */
export function resolveActive(
  organizations: MeOrganization[],
  slug: string | null,
): MeOrganization | null {
  return (
    organizations.find((o) => o.slug === slug) ??
    organizations.find((o) => o.status === "active") ??
    organizations[0] ??
    null
  );
}

const WORKSPACE_ENTERED_KEY = "clickfix.workspace-entered";

/**
 * Platform admins land on the platform console; once they pick one of their
 * organizations in this tab, `/` opens that workspace instead of bouncing
 * back. Stored per user in the tab and cleared on sign-out, so the next
 * sign-in starts on the console again.
 */
export function markWorkspaceEntered(userUuid: string) {
  try {
    sessionStorage.setItem(WORKSPACE_ENTERED_KEY, userUuid);
  } catch {
    // Storage blocked: `/` keeps dispatching to the console.
  }
}

export function hasEnteredWorkspace(userUuid: string): boolean {
  try {
    return sessionStorage.getItem(WORKSPACE_ENTERED_KEY) === userUuid;
  } catch {
    return false;
  }
}

if (typeof window !== "undefined") {
  useAuthStore.subscribe((state, prev) => {
    if (prev.user && !state.user) {
      try {
        sessionStorage.removeItem(WORKSPACE_ENTERED_KEY);
      } catch {
        // Nothing stored.
      }
    }
  });
}
