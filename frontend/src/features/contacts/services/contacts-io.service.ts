import type { components } from "@/generated/api";
import {
  tenantDownloadFile,
  tenantFormRequest,
  tenantRequest,
} from "@/lib/api/tenant-request";

export type ContactImportJob = components["schemas"]["ContactImportJob"];
export type ContactIOResource = "contacts" | "companies";
export type ContactIOFormat = "csv" | "xlsx";

const IMPORTS = "/v1/tenant/contact-imports";

/** Contact / company import and export (CF-84), scoped to `slug`. */
export const contactsIOService = {
  export(
    slug: string,
    resource: ContactIOResource,
    format: ContactIOFormat,
    query?: { q?: string; company?: string },
  ) {
    return tenantDownloadFile(slug, `/v1/tenant/${resource}/export`, {
      format,
      q: query?.q,
      company: query?.company,
    });
  },
  sample(slug: string, resource: ContactIOResource, format: ContactIOFormat) {
    return tenantDownloadFile(slug, `/v1/tenant/${resource}/import/sample`, {
      format,
    });
  },
  upload(
    slug: string,
    resource: ContactIOResource,
    file: File,
    locale: string,
  ) {
    const form = new FormData();
    form.append("file", file);
    form.append("locale", locale);
    return tenantFormRequest<ContactImportJob>(
      slug,
      "POST",
      `/v1/tenant/${resource}/import`,
      form,
    );
  },
  list(slug: string, resource: ContactIOResource) {
    return tenantRequest<{ items: ContactImportJob[]; total: number }>(
      slug,
      "GET",
      IMPORTS,
      { query: { resource, limit: 5 } },
    );
  },
  get(slug: string, uuid: string) {
    return tenantRequest<ContactImportJob>(slug, "GET", `${IMPORTS}/${uuid}`);
  },
  updateMapping(
    slug: string,
    uuid: string,
    mapping: Record<string, string>,
    defaults: Record<string, string>,
  ) {
    return tenantRequest<ContactImportJob>(
      slug,
      "PATCH",
      `${IMPORTS}/${uuid}/mapping`,
      { body: { mapping, defaults } },
    );
  },
  preview(slug: string, uuid: string) {
    return tenantRequest<ContactImportJob>(
      slug,
      "POST",
      `${IMPORTS}/${uuid}/preview`,
    );
  },
  confirm(slug: string, uuid: string) {
    return tenantRequest<ContactImportJob>(
      slug,
      "POST",
      `${IMPORTS}/${uuid}/confirm`,
    );
  },
  rollback(slug: string, uuid: string) {
    return tenantRequest<ContactImportJob>(
      slug,
      "POST",
      `${IMPORTS}/${uuid}/rollback`,
    );
  },
};
