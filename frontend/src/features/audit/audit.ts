"use client";

import { useMutation, useQuery } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantDownloadFile, tenantRequest } from "@/lib/api/tenant-request";
import { download } from "@/lib/utils/format";

type Schemas = components["schemas"];

export type AuditEvent = Schemas["AuditEvent"];
export type AuditEventPage = Schemas["AuditEventPage"];
export type AuditFacet = Schemas["AuditFacet"];

/** Filters shared by the list and the CSV export. */
export type AuditFilters = {
  q?: string;
  /** Comma-separated user uuids. */
  actor?: string;
  /** Comma-separated resource slugs. */
  resource?: string;
  /** Comma-separated action slugs. */
  action?: string;
  /** `YYYY-MM-DD` (start of day). */
  from?: string;
  /** `YYYY-MM-DD` (whole day included). */
  to?: string;
};

export type ListAuditParams = AuditFilters & {
  limit: number;
  offset: number;
  sort?: string;
};

const BASE = "/v1/tenant/audit";

export const auditService = {
  list(slug: string, params: ListAuditParams) {
    return tenantRequest<AuditEventPage>(slug, "GET", BASE, { query: params });
  },
  facets(slug: string) {
    return tenantRequest<{ items: AuditFacet[] }>(
      slug,
      "GET",
      `${BASE}/facets`,
    );
  },
  /** CSV through the same-origin BFF. */
  export(slug: string, filters: AuditFilters) {
    return tenantDownloadFile(slug, `${BASE}/export`, filters);
  },
};

export const auditKeys = {
  all: (slug: string) => ["tenant", slug, "audit"] as const,
  list: (slug: string, params: ListAuditParams) =>
    [...auditKeys.all(slug), "list", params] as const,
  facets: (slug: string) => [...auditKeys.all(slug), "facets"] as const,
};

export function useAuditList(params: ListAuditParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: auditKeys.list(slug, params),
    queryFn: () => auditService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled: Boolean(slug),
  });
}

export function useAuditFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: auditKeys.facets(slug),
    queryFn: () => auditService.facets(slug),
    enabled: Boolean(slug),
    staleTime: 60_000,
  });
}

export function useAuditExport() {
  const { slug } = useTenant();
  return useMutation({
    mutationFn: async (filters: AuditFilters) => {
      const { blob, filename } = await auditService.export(slug, filters);
      download(blob, filename ?? `audit-${slug}.csv`);
    },
  });
}

/** Translated label of an activity slug, the raw slug when unknown. */
export function activityLabel(
  t: (key: string) => string,
  kind: "actions" | "resources",
  slug: string,
): string {
  const key = `activity.${kind}.${slug}`;
  const label = t(key);
  return label === key ? slug : label;
}
