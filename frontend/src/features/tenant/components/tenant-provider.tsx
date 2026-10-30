"use client";

import { createContext, useContext, useMemo, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";

import { useActiveOrganization } from "@/features/organizations";
import { isOrganizationAccessExpired } from "@/features/tenant/lib/access";
import {
  tenantKeys,
  tenantService,
} from "@/features/tenant/services/tenant.service";
import { isApiError, type ApiError } from "@/lib/api";
import type { MeOrganization } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { PermissionProvider } from "@/providers/permission-provider";

export type TenantOrganization = {
  uuid: string;
  slug: string;
  name: string;
  /** API-owned logo stream (`/v1/organizations/{uuid}/logo?v=…`). */
  logoUrl: string | null;
  status: "active" | "suspended";
  accessEndsAt: string | null;
};

export type TenantScope = {
  /** Slug of the active organization (`""` when the user has none). */
  slug: string;
  organization: TenantOrganization | null;
  /** Primary tenant role in this organization (`owner`, `admin`, …). */
  role: { key: string; name?: string } | null;
  /** Every tenant role key the member holds here (permissions are their union). */
  roles: string[];
  /** `tenant.*` permissions in this organization only. */
  permissions: string[];
  /** The API confirmed the membership for this slug. */
  verified: boolean;
  /**
   * The cached `/me` membership is usable while the API answers (active and
   * inside its access window), so pages can render optimistically.
   */
  optimistic: boolean;
  isLoading: boolean;
  /** API error of the context request (membership, suspension, …). */
  error: ApiError | null;
  /** The context request failed (any error, including network). */
  failed: boolean;
  refetch: () => void;
};

const TenantContext = createContext<TenantScope | null>(null);

const EMPTY: string[] = [];

function fromMembership(org: MeOrganization): TenantOrganization {
  return {
    uuid: org.uuid,
    slug: org.slug,
    name: org.name,
    logoUrl: org.logo_url || null,
    status: org.status,
    accessEndsAt: org.access_ends_at ?? null,
  };
}

function shouldRetry(failureCount: number, error: unknown) {
  if (isApiError(error) && error.status < 500) return false;
  return failureCount < 2;
}

/**
 * Scope of the ACTIVE organization of the workspace shell (sidebar switcher,
 * `features/organizations`): resolves it through `GET /v1/tenant/context`
 * and adds its `tenant.*` permissions to `usePermission()` below. Every
 * tenant query key carries the slug, so switching organizations refetches.
 */
export function TenantProvider({ children }: { children: ReactNode }) {
  const { user, isAuthenticated } = useAuth();
  const { active: membership } = useActiveOrganization();
  const slug = membership?.slug ?? "";

  const query = useQuery({
    queryKey: tenantKeys.context(slug),
    queryFn: ({ signal }) => tenantService.getContext(slug, signal),
    enabled: isAuthenticated && Boolean(slug),
    retry: shouldRetry,
    staleTime: 30_000,
  });

  const { data, error, isLoading, refetch } = query;

  const value = useMemo<TenantScope>(() => {
    const apiError = isApiError(error) ? error : null;
    const optimistic =
      membership !== null &&
      membership.status === "active" &&
      !isOrganizationAccessExpired(membership.access_ends_at);

    if (data && data.organization.slug === slug) {
      const roleName =
        membership && membership.role.key === data.role
          ? membership.role.name
          : undefined;
      return {
        slug,
        organization: {
          uuid: data.organization.uuid,
          slug: data.organization.slug,
          name: membership?.name ?? data.organization.name,
          logoUrl: membership?.logo_url || null,
          status: data.organization.status,
          accessEndsAt: data.organization.access_ends_at ?? null,
        },
        role: { key: data.role, name: roleName },
        roles: data.roles?.length ? data.roles : [data.role],
        permissions: data.permissions ?? EMPTY,
        verified: true,
        optimistic,
        isLoading: false,
        error: apiError,
        failed: Boolean(apiError),
        refetch: () => void refetch(),
      };
    }

    return {
      slug,
      organization: membership ? fromMembership(membership) : null,
      role: membership ? membership.role : null,
      roles: membership ? [membership.role.key] : EMPTY,
      permissions: membership?.permissions ?? EMPTY,
      verified: false,
      optimistic,
      isLoading: Boolean(slug) && isLoading,
      error: apiError,
      failed: Boolean(error),
      refetch: () => void refetch(),
    };
  }, [data, error, isLoading, membership, refetch, slug]);

  // The user's own permissions plus the active organization's `tenant.*`
  // ones, so the workspace nav and pages can gate on both.
  const globalPermissions = user?.permissions;
  const globalRoles = user?.roles;
  const scopedPermissions = useMemo(
    () => [...new Set([...(globalPermissions ?? EMPTY), ...value.permissions])],
    [globalPermissions, value.permissions],
  );
  const scopedRoles = useMemo(
    () => [...new Set([...(globalRoles ?? EMPTY), ...value.roles])],
    [globalRoles, value.roles],
  );

  return (
    <TenantContext.Provider value={value}>
      <PermissionProvider permissions={scopedPermissions} roles={scopedRoles}>
        {children}
      </PermissionProvider>
    </TenantContext.Provider>
  );
}

/** Active organization scope; only inside the workspace shell. */
export function useTenant(): TenantScope {
  const ctx = useContext(TenantContext);
  if (!ctx) throw new Error("useTenant must be used within TenantProvider");
  return ctx;
}

/** Active organization scope, or `null` outside the workspace shell. */
export function useOptionalTenant(): TenantScope | null {
  return useContext(TenantContext);
}
