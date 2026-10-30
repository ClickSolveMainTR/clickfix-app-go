"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { apiConfig } from "@/config/api";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { ApiError } from "@/lib/api/errors";
import { tenantRequest } from "@/lib/api/tenant-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export type SSOConfig = components["schemas"]["OrganizationSSOConfig"];
export type SSOProtocol = SSOConfig["protocol"];

const BASE = "/v1/tenant/security";

export const securityService = {
  sso(slug: string) {
    return tenantRequest<SSOConfig>(slug, "GET", `${BASE}/sso`);
  },
  saveSSO(slug: string, body: SSOConfig) {
    return tenantRequest<SSOConfig>(slug, "PUT", `${BASE}/sso`, {
      body,
      silent: true,
    });
  },
  clientIP(slug: string) {
    return tenantRequest<{ ip: string }>(slug, "GET", `${BASE}/client-ip`);
  },
  rotateSCIM(slug: string) {
    return tenantRequest<{ token: string; base_url: string }>(
      slug,
      "POST",
      `${BASE}/scim-token`,
      { silent: true },
    );
  },
};

export const securityKeys = {
  all: (slug: string) => ["tenant", slug, "security"] as const,
  sso: (slug: string) => [...securityKeys.all(slug), "sso"] as const,
  ip: (slug: string) => [...securityKeys.all(slug), "client-ip"] as const,
};

export function useSSOConfig() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: securityKeys.sso(slug),
    queryFn: () => securityService.sso(slug),
    enabled: Boolean(slug),
  });
}

export function useClientIP() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: securityKeys.ip(slug),
    queryFn: () => securityService.clientIP(slug),
    enabled: Boolean(slug),
    staleTime: 60_000,
  });
}

/** Maps the API's SSO save errors to a message (the request is silent). */
function saveErrorKey(error: unknown, forbiddenKey: string): string {
  if (error instanceof ApiError) {
    if (isPlanError(error)) return "security.errors.entitlement";
    if (error.status === 409) return "security.errors.ip_lockout";
    if (error.status === 403) return forbiddenKey;
    if (error.status === 400 || error.status === 422) {
      return "security.errors.invalid";
    }
  }
  return "security.errors.save_failed";
}

/** Plan gate (`security.sso` feature) answers 403 with an entitlement code. */
function isPlanError(error: ApiError) {
  return (
    error.status === 403 &&
    [
      "ENTITLEMENT_REQUIRED",
      "PLAN_LIMIT_EXCEEDED",
      "SUBSCRIPTION_READ_ONLY",
    ].includes(error.code)
  );
}

/**
 * PUT replaces the whole configuration: callers merge their section into the
 * loaded config. An empty `client_secret` keeps the stored secret.
 */
export function useSaveSSOConfig(
  successKey: string,
  forbiddenKey = "security.errors.forbidden",
) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: SSOConfig) => securityService.saveSSO(slug, body),
    onSuccess: (data) => {
      appToast.success(t(successKey));
      queryClient.setQueryData(securityKeys.sso(slug), data);
    },
    onError: (error) => appToast.error(t(saveErrorKey(error, forbiddenKey))),
  });
}

export function useRotateSCIM() {
  const { slug } = useTenant();
  const { t } = useLocale();
  return useMutation({
    mutationFn: () => securityService.rotateSCIM(slug),
    onError: (error) =>
      appToast.error(
        t(
          !(error instanceof ApiError)
            ? "security.errors.save_failed"
            : isPlanError(error)
              ? "security.errors.entitlement"
              : error.status === 400 || error.status === 422
                ? "security.errors.scim_needs_sso"
                : "security.errors.save_failed",
        ),
      ),
  });
}

/**
 * Service-provider URLs the IdP needs. The API builds them from the public
 * frontend origin + `/api` (BFF), the same origin this page runs on.
 */
export function spUrls(organization: string) {
  const origin = typeof window === "undefined" ? "" : window.location.origin;
  const root = `${origin}${apiConfig.baseUrl.replace(/\/$/, "")}/v1`;
  const sso = `${root}/public/sso/${organization}`;
  return {
    oidcRedirect: `${sso}/oidc/callback`,
    samlAcs: `${sso}/saml/callback`,
    samlMetadata: `${sso}/saml/metadata`,
    scimBase: `${root}/scim/${organization}/v2`,
    /** Panel SSO entry members can bookmark (`/login/sso`). */
    signIn: `${origin}${routes.guest.loginSSO}?organization=${organization}`,
  };
}
