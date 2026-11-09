import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

type Schemas = components["schemas"];

export type SystemOverview = Schemas["SystemOverview"];
export type SystemConnection = Schemas["SystemConnection"];
export type SystemConnectionInput = Schemas["SystemConnectionInput"];
export type SystemConcept = Schemas["SystemConcept"];
export type SystemSource = Schemas["SystemSource"];
export type SystemSourceInput = Schemas["SystemSourceInput"];
export type SystemField = Schemas["SystemField"];
export type SystemTestInput = Schemas["SystemTestInput"];
export type SystemTestResult = Schemas["SystemTestResult"];
export type SystemPathOption = Schemas["SystemPathOption"];
export type SystemAction = Schemas["SystemAction"];
export type SystemActionInput = Schemas["SystemActionInput"];
export type SystemParam = Schemas["SystemParam"];
export type SystemChannel = Schemas["SystemChannel"];
export type SystemPlaybook = Schemas["SystemPlaybook"];
export type SystemPlaybookInput = Schemas["SystemPlaybookInput"];
export type SystemPreviewInput = Schemas["SystemPreviewInput"];
export type SystemPreview = Schemas["SystemPreview"];
export type SystemLookup = Schemas["SystemLookup"];
export type SystemCall = Schemas["SystemCall"];

const BASE = "/v1/tenant/integrations/system";

/** Connected system, "Sistemini bağla" (CF-197). */
export const connectService = {
  overview: (slug: string) => tenantRequest<SystemOverview>(slug, "GET", BASE),
  saveConnection: (slug: string, body: SystemConnectionInput) =>
    tenantRequest<SystemConnection>(slug, "PUT", `${BASE}/connection`, {
      body,
    }),
  saveSource: (slug: string, typeUuid: string, body: SystemSourceInput) =>
    tenantRequest<SystemSource>(
      slug,
      "PUT",
      `${BASE}/sources/${encodeURIComponent(typeUuid)}`,
      { body },
    ),
  deleteSource: (slug: string, typeUuid: string) =>
    tenantRequest<{ deleted: boolean }>(
      slug,
      "DELETE",
      `${BASE}/sources/${encodeURIComponent(typeUuid)}`,
    ),
  testSource: (slug: string, typeUuid: string, body: SystemTestInput) =>
    tenantRequest<SystemTestResult>(
      slug,
      "POST",
      `${BASE}/sources/${encodeURIComponent(typeUuid)}/test`,
      { body },
    ),
  saveAction: (slug: string, key: string, body: SystemActionInput) =>
    tenantRequest<SystemAction>(
      slug,
      "PUT",
      `${BASE}/actions/${encodeURIComponent(key)}`,
      { body },
    ),
  deleteAction: (slug: string, key: string) =>
    tenantRequest<{ deleted: boolean }>(
      slug,
      "DELETE",
      `${BASE}/actions/${encodeURIComponent(key)}`,
    ),
  savePlaybook: (slug: string, intent: string, body: SystemPlaybookInput) =>
    tenantRequest<SystemPlaybook>(
      slug,
      "PUT",
      `${BASE}/playbooks/${encodeURIComponent(intent)}`,
      { body },
    ),
  deletePlaybook: (slug: string, intent: string) =>
    tenantRequest<{ deleted: boolean }>(
      slug,
      "DELETE",
      `${BASE}/playbooks/${encodeURIComponent(intent)}`,
    ),
  preview: (slug: string, body: SystemPreviewInput) =>
    tenantRequest<SystemPreview>(slug, "POST", `${BASE}/preview`, { body }),
};

export const connectKeys = {
  overview: (slug: string) => ["tenant", slug, "connected-system"] as const,
};
