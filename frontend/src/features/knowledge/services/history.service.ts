import type { components } from "@/generated/api";
import {
  tenantDownloadFile,
  tenantFormRequest,
  tenantRequest,
} from "@/lib/api/tenant-request";

export type HistoryImport = components["schemas"]["HistoryImport"];
export type HistorySuggestion = components["schemas"]["HistorySuggestion"];
export type HistoryCluster = components["schemas"]["HistoryCluster"];
export type HistorySettings = components["schemas"]["HistorySettings"];
export type HistorySettingsInput =
  components["schemas"]["HistorySettingsInput"];
export type HistoryDecisionInput =
  components["schemas"]["HistoryDecisionInput"];

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

const BASE = "/v1/tenant/history";

/** A run that is still moving: the panel polls while one exists. */
export function isActive(run: HistoryImport) {
  return run.status === "pending" || run.status === "running";
}

/** History pipeline (CF-217), scoped to the organization `slug`. */
export const historyService = {
  imports(slug: string) {
    return tenantRequest<Page<HistoryImport>>(slug, "GET", `${BASE}/imports`, {
      query: { limit: 20 },
    });
  },
  upload(
    slug: string,
    file: File,
    consent: { analysis: boolean; training: boolean },
  ) {
    const form = new FormData();
    form.append("file", file);
    form.append("consent_analysis", String(consent.analysis));
    form.append("consent_training", String(consent.training));
    return tenantFormRequest<HistoryImport>(
      slug,
      "POST",
      `${BASE}/imports`,
      form,
    );
  },
  cancel(slug: string, uuid: string) {
    return tenantRequest<HistoryImport>(
      slug,
      "POST",
      `${BASE}/imports/${uuid}/cancel`,
    );
  },
  rollback(slug: string, uuid: string) {
    return tenantRequest<HistoryImport>(
      slug,
      "POST",
      `${BASE}/imports/${uuid}/rollback`,
    );
  },
  suggestions(slug: string) {
    return tenantRequest<Page<HistorySuggestion>>(
      slug,
      "GET",
      `${BASE}/suggestions`,
      { query: { status: "pending", limit: 50 } },
    );
  },
  clusters(slug: string) {
    return tenantRequest<Page<HistoryCluster>>(
      slug,
      "GET",
      `${BASE}/clusters`,
      { query: { limit: 20 } },
    );
  },
  decide(slug: string, uuid: string, body: HistoryDecisionInput) {
    return tenantRequest(slug, "POST", `${BASE}/suggestions/${uuid}/decision`, {
      body,
    });
  },
  settings(slug: string) {
    return tenantRequest<HistorySettings>(slug, "GET", `${BASE}/settings`);
  },
  saveSettings(slug: string, body: HistorySettingsInput) {
    return tenantRequest<HistorySettings>(slug, "PUT", `${BASE}/settings`, {
      body,
    });
  },
  sample(slug: string) {
    return tenantDownloadFile(slug, `${BASE}/sample`);
  },
};

export const historyKeys = {
  all: (org: string | undefined) => ["tenant", org, "history"] as const,
  imports: (org: string | undefined) =>
    ["tenant", org, "history", "imports"] as const,
  suggestions: (org: string | undefined) =>
    ["tenant", org, "history", "suggestions"] as const,
  clusters: (org: string | undefined) =>
    ["tenant", org, "history", "clusters"] as const,
  settings: (org: string | undefined) =>
    ["tenant", org, "history", "settings"] as const,
};
