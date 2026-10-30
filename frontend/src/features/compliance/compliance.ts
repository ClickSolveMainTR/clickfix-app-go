"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { contactsService } from "@/features/contacts/services/contacts.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantDownloadFile, tenantRequest } from "@/lib/api/tenant-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Schemas = components["schemas"];

export type RetentionPolicy = Schemas["RetentionPolicy"];
export type DataSubjectRequest = Schemas["DataSubjectRequest"];
export type DataSubjectRequestItem = Schemas["DataSubjectRequestItem"];
export type DataSubjectKind = DataSubjectRequest["kind"];
export type DataSubjectStatus = DataSubjectRequest["status"];
export type ContactConsent = Schemas["ContactConsent"];
export type ContactConsentInput = Schemas["ContactConsentInput"];

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

/** Draft VERBİS inventory (`/processing-inventory`); free-form on the API. */
export type InventoryItem = {
  category: string;
  purpose: string;
  legal_basis: string;
  subjects?: string[];
  data?: string[];
};
export type ProcessingInventory = {
  organization_uuid: string;
  retention: RetentionPolicy;
  items: InventoryItem[];
  review_required: boolean;
  note: string;
};

const BASE = "/v1/tenant/compliance";

export const complianceService = {
  requests(slug: string, params: { limit: number; offset: number }) {
    return tenantRequest<Page<DataSubjectRequestItem>>(
      slug,
      "GET",
      `${BASE}/requests`,
      { query: params },
    );
  },
  createRequest(slug: string, contact: string, kind: DataSubjectKind) {
    return tenantRequest<DataSubjectRequest>(
      slug,
      "POST",
      `${BASE}/contacts/${contact}/requests`,
      { body: { kind } },
    );
  },
  retention(slug: string) {
    return tenantRequest<RetentionPolicy>(slug, "GET", `${BASE}/retention`);
  },
  saveRetention(slug: string, body: RetentionPolicy) {
    return tenantRequest<RetentionPolicy>(slug, "PUT", `${BASE}/retention`, {
      body,
    });
  },
  consents(
    slug: string,
    contact: string,
    params: { limit: number; offset: number },
  ) {
    return tenantRequest<Page<ContactConsent>>(
      slug,
      "GET",
      `${BASE}/contacts/${contact}/consents`,
      { query: params },
    );
  },
  recordConsent(slug: string, contact: string, body: ContactConsentInput) {
    return tenantRequest<ContactConsent>(
      slug,
      "POST",
      `${BASE}/contacts/${contact}/consents`,
      { body },
    );
  },
  inventory(slug: string) {
    return tenantRequest<ProcessingInventory>(
      slug,
      "GET",
      `${BASE}/processing-inventory`,
    );
  },
};

export const complianceKeys = {
  all: (slug: string) => ["tenant", slug, "compliance"] as const,
  requests: (slug: string, params: Record<string, unknown>) =>
    [...complianceKeys.all(slug), "requests", params] as const,
  retention: (slug: string) =>
    [...complianceKeys.all(slug), "retention"] as const,
  consents: (slug: string, contact: string, params: Record<string, unknown>) =>
    [...complianceKeys.all(slug), "consents", contact, params] as const,
  inventory: (slug: string) =>
    [...complianceKeys.all(slug), "inventory"] as const,
};

export function useDataSubjectRequests(params: {
  limit: number;
  offset: number;
}) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: complianceKeys.requests(slug, params),
    queryFn: () => complianceService.requests(slug, params),
    placeholderData: (previous) => previous,
    enabled: Boolean(slug),
    // Requests run in the background worker: poll while any is unfinished.
    refetchInterval: (query) =>
      query.state.data?.items.some((r) => r.status !== "completed")
        ? 5_000
        : false,
  });
}

export function useCreateDataSubjectRequest() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (v: { contact: string; kind: DataSubjectKind }) =>
      complianceService.createRequest(slug, v.contact, v.kind),
    onSuccess: (_, v) => {
      appToast.success(
        t(
          v.kind === "erase"
            ? "privacy.toast.erase_queued"
            : "privacy.toast.export_queued",
        ),
      );
      void queryClient.invalidateQueries({
        queryKey: complianceKeys.all(slug),
      });
    },
  });
}

export function useRetentionPolicy() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: complianceKeys.retention(slug),
    queryFn: () => complianceService.retention(slug),
    enabled: Boolean(slug),
  });
}

export function useSaveRetentionPolicy() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: RetentionPolicy) =>
      complianceService.saveRetention(slug, body),
    onSuccess: (data) => {
      appToast.success(t("privacy.toast.retention_saved"));
      queryClient.setQueryData(complianceKeys.retention(slug), data);
      void queryClient.invalidateQueries({
        queryKey: complianceKeys.inventory(slug),
      });
    },
  });
}

export function useContactConsents(
  contact: string | null,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: complianceKeys.consents(slug, contact ?? "", params),
    queryFn: () => complianceService.consents(slug, contact as string, params),
    placeholderData: (previous) => previous,
    enabled: Boolean(slug && contact),
  });
}

export function useRecordConsent(contact: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: ContactConsentInput) =>
      complianceService.recordConsent(slug, contact, body),
    onSuccess: () => {
      appToast.success(t("privacy.toast.consent_recorded"));
      void queryClient.invalidateQueries({
        queryKey: [...complianceKeys.all(slug), "consents", contact],
      });
    },
  });
}

export function useProcessingInventory() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: complianceKeys.inventory(slug),
    queryFn: () => complianceService.inventory(slug),
    enabled: Boolean(slug),
  });
}

/** Contact search for the subject picker (needs `tenant.contacts.read`). */
export async function searchContacts(q: string) {
  const page = await contactsService.list({ q, limit: 20, offset: 0 });
  return page.items.map((c) => ({
    value: c.uuid,
    label: c.name || c.email || c.phone || c.uuid,
    description: c.name ? (c.email ?? c.phone ?? undefined) : undefined,
  }));
}

/**
 * Saves the export ZIP through the API-owned `download_url`
 * (`/api/v1/tenant/...`); the browser never builds a storage key.
 */
export async function downloadExport(slug: string, downloadUrl: string) {
  const path = downloadUrl.replace(/^\/api(?=\/v1\/)/, "");
  const { blob, filename } = await tenantDownloadFile(slug, path);
  const href = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = href;
  a.download = filename ?? "data-subject.zip";
  a.click();
  URL.revokeObjectURL(href);
}

/** Static label keys (no template keys, see check-i18n). */
export const KIND_LABEL_KEYS: Record<DataSubjectKind, string> = {
  export: "privacy.kind.export",
  erase: "privacy.kind.erase",
};

export const STATUS_LABEL_KEYS: Record<DataSubjectStatus, string> = {
  pending: "privacy.status.pending",
  processing: "privacy.status.processing",
  completed: "privacy.status.completed",
};
