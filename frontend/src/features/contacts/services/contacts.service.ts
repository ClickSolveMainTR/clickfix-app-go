import type { components, operations } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations/lib/active-organization-store";
import { apiClient, unwrap } from "@/lib/api";

export type Contact = components["schemas"]["Contact"];
export type ContactInput = components["schemas"]["ContactInput"];
export type ContactPage = components["schemas"]["ContactPage"];
export type Company = components["schemas"]["Company"];
export type CompanyInput = components["schemas"]["CompanyInput"];
export type CompanyPage = components["schemas"]["CompanyPage"];
export type ContactMergeResult = components["schemas"]["ContactMergeResult"];

/** The org header is added by the client middleware; typed here for openapi-fetch. */
function header() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

export type ContactIdentityState =
  components["schemas"]["ContactIdentityState"];

export type ContactListQuery = {
  q?: string;
  company?: string;
  /** CF-195: verified / anonymous / self_named. */
  identity?: ContactIdentityState;
  limit: number;
  offset: number;
  /** One field, `-` prefix for descending. Server default: `-created_at`. */
  sort?: ContactSort;
};

export type ContactSort = NonNullable<
  NonNullable<operations["listTenantContacts"]["parameters"]["query"]>["sort"]
>;

export const contactsService = {
  async list(p: ContactListQuery) {
    return unwrap<ContactPage>(
      await apiClient.GET("/v1/tenant/contacts", {
        params: {
          header: header(),
          query: {
            limit: p.limit,
            offset: p.offset,
            ...(p.q ? { q: p.q } : {}),
            ...(p.company ? { company: p.company } : {}),
            ...(p.identity ? { identity: p.identity } : {}),
            ...(p.sort ? { sort: p.sort } : {}),
          },
        },
      }),
    );
  },
  async get(uuid: string) {
    return unwrap<Contact>(
      await apiClient.GET("/v1/tenant/contacts/{uuid}", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },
  async create(body: ContactInput) {
    return unwrap<Contact>(
      await apiClient.POST("/v1/tenant/contacts", {
        params: { header: header() },
        body,
      }),
    );
  },
  async update(uuid: string, body: ContactInput) {
    return unwrap<Contact>(
      await apiClient.PATCH("/v1/tenant/contacts/{uuid}", {
        params: { header: header(), path: { uuid } },
        body,
      }),
    );
  },
  async remove(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/contacts/{uuid}", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },
  async merge(into: string, from: string) {
    return unwrap<ContactMergeResult>(
      await apiClient.POST("/v1/tenant/contacts/{uuid}/merge", {
        params: { header: header(), path: { uuid: into } },
        body: { from },
      }),
    );
  },
  async companies(p: { q?: string; limit: number; offset: number }) {
    return unwrap<CompanyPage>(
      await apiClient.GET("/v1/tenant/companies", {
        params: {
          header: header(),
          query: {
            limit: p.limit,
            offset: p.offset,
            ...(p.q ? { q: p.q } : {}),
          },
        },
      }),
    );
  },
  async createCompany(body: CompanyInput) {
    return unwrap<Company>(
      await apiClient.POST("/v1/tenant/companies", {
        params: { header: header() },
        body,
      }),
    );
  },
  async getCompany(uuid: string) {
    return unwrap<Company>(
      await apiClient.GET("/v1/tenant/companies/{uuid}", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },
  async updateCompany(uuid: string, body: CompanyInput) {
    return unwrap<Company>(
      await apiClient.PATCH("/v1/tenant/companies/{uuid}", {
        params: { header: header(), path: { uuid } },
        body,
      }),
    );
  },
  async removeCompany(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/companies/{uuid}", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },
};

export type CompanyOverview = components["schemas"]["CompanyOverview"];
export type CompanyTimelineItem = components["schemas"]["CompanyTimelineItem"];
export type CompanyTimelinePage = components["schemas"]["CompanyTimelinePage"];
export type CompanyTimelineKind = CompanyTimelineItem["kind"];
export type CompanyNote = components["schemas"]["CompanyNote"];

/** Account screen and unified timeline (CF-85). */
export const accountService = {
  async overview(uuid: string) {
    return unwrap<CompanyOverview>(
      await apiClient.GET("/v1/tenant/companies/{uuid}/overview", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },
  async timeline(
    uuid: string,
    p: { kinds?: CompanyTimelineKind[]; before?: string | null },
  ) {
    return unwrap<CompanyTimelinePage>(
      await apiClient.GET("/v1/tenant/companies/{uuid}/timeline", {
        params: {
          header: header(),
          path: { uuid },
          query: {
            limit: 30,
            ...(p.kinds?.length ? { kinds: p.kinds.join(",") } : {}),
            ...(p.before ? { before: p.before } : {}),
          },
        },
      }),
    );
  },
  async addNote(uuid: string, body: string) {
    return unwrap<CompanyNote>(
      await apiClient.POST("/v1/tenant/companies/{uuid}/notes", {
        params: { header: header(), path: { uuid } },
        body: { body },
      }),
    );
  },
  async deleteNote(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/company-notes/{uuid}", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },
};
