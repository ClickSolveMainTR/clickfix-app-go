import type { components } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations";
import { apiClient, unwrap } from "@/lib/api";

export type RequestForm = components["schemas"]["RequestForm"];
export type RequestFormInput = components["schemas"]["RequestFormInput"];
export type RequestFormField = components["schemas"]["RequestFormField"];
export type PublicRequestForm = components["schemas"]["PublicRequestForm"];
export type PublicRequestFormField =
  components["schemas"]["PublicRequestFormField"];
export type ConversationForm = components["schemas"]["ConversationForm"];

/** Tenant routes are scoped by the active organization. */
function orgHeader() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

export const requestFormsService = {
  async list() {
    const data = await unwrap<{ items: RequestForm[] }>(
      await apiClient.GET("/v1/tenant/request-forms", {
        params: { header: orgHeader() },
      }),
    );
    return data.items;
  },

  async create(body: RequestFormInput) {
    return unwrap<RequestForm>(
      await apiClient.POST("/v1/tenant/request-forms", {
        params: { header: orgHeader() },
        body,
      }),
    );
  },

  async update(uuid: string, body: RequestFormInput) {
    return unwrap<RequestForm>(
      await apiClient.PATCH("/v1/tenant/request-forms/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
        body,
      }),
    );
  },

  async remove(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/request-forms/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  async conversation(uuid: string) {
    return unwrap<ConversationForm>(
      await apiClient.GET("/v1/tenant/conversations/{uuid}/form", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  async updateConversation(uuid: string, values: Record<string, unknown>) {
    return unwrap<ConversationForm>(
      await apiClient.PATCH("/v1/tenant/conversations/{uuid}/form", {
        params: { header: orgHeader(), path: { uuid } },
        body: { values },
      }),
    );
  },
};
