"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { permissions } from "@/config/permissions";
import { engineeringKeys } from "@/features/engineering/hooks/use-engineering";
import { isProviderAuthorizeUrl } from "@/features/engineering/lib/integration-setup";
import {
  engineeringService,
  type EngineeringIntegrationInput,
  type EngineeringProvider,
} from "@/features/engineering/services/engineering.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { usePermission } from "@/providers/permission-provider";

export const integrationKeys = {
  list: (slug: string) =>
    [...engineeringKeys.all(slug), "integrations"] as const,
};

/**
 * Settings readers see the masked status; configuring, OAuth and switching
 * off need `tenant.settings.write`.
 */
export function useIntegrationScope() {
  const { slug } = useTenant();
  const { can } = usePermission();
  return {
    slug,
    canRead: can(permissions.tenant.settings.read),
    canWrite: can(permissions.tenant.settings.write),
  };
}

export function useIntegrations() {
  const { slug, canRead } = useIntegrationScope();
  return useQuery({
    queryKey: integrationKeys.list(slug),
    queryFn: () => engineeringService.integrations(slug),
    enabled: Boolean(slug) && canRead,
    staleTime: 30_000,
  });
}

export function useSaveIntegration(provider: EngineeringProvider) {
  const { slug } = useIntegrationScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: EngineeringIntegrationInput) =>
      engineeringService.saveIntegration(slug, provider, body),
    onSettled: () =>
      void queryClient.invalidateQueries({
        queryKey: integrationKeys.list(slug),
      }),
  });
}

/**
 * Asks the API for the consent URL and leaves for the provider. The URL is
 * followed only when it points at that provider's own consent host.
 */
export function useStartOAuth(provider: EngineeringProvider) {
  const { slug } = useIntegrationScope();
  return useMutation({
    mutationFn: async () => {
      const { authorize_url: url } = await engineeringService.startOAuth(
        slug,
        provider,
      );
      if (!isProviderAuthorizeUrl(provider, url)) {
        throw new Error("unexpected authorize URL");
      }
      window.location.assign(url);
    },
  });
}
