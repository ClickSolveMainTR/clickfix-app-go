import type { components } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations/lib/active-organization-store";
import { apiClient, unwrap } from "@/lib/api";

export type ClosingText = components["schemas"]["ClosingText"];
export type ClosingTextInput = components["schemas"]["ClosingTextInput"];
export type ClosingTextList = components["schemas"]["ClosingTextList"];
export type ClosingTextSettings = components["schemas"]["ClosingTextSettings"];
export type ClosingTextOptions = components["schemas"]["ClosingTextOptions"];
export type ClosingTextOption = components["schemas"]["ClosingTextOption"];

function header() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

/** Closing texts appended to replies (CF-250). */
export const closingTextsService = {
  async list() {
    return unwrap<ClosingTextList>(
      await apiClient.GET("/v1/tenant/closing-texts", {
        params: { header: header() },
      }),
    );
  },
  async create(body: ClosingTextInput) {
    return unwrap<ClosingText>(
      await apiClient.POST("/v1/tenant/closing-texts", {
        params: { header: header() },
        body,
      }),
    );
  },
  async update(uuid: string, body: ClosingTextInput) {
    return unwrap<ClosingText>(
      await apiClient.PUT("/v1/tenant/closing-texts/{uuid}", {
        params: { header: header(), path: { uuid } },
        body,
      }),
    );
  },
  async remove(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/closing-texts/{uuid}", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },
  async saveSettings(body: ClosingTextSettings) {
    return unwrap<ClosingTextSettings>(
      await apiClient.PUT("/v1/tenant/closing-text-settings", {
        params: { header: header() },
        body,
      }),
    );
  },
  /** Texts usable on one conversation, rendered, with the starting pick. */
  async forConversation(uuid: string) {
    return unwrap<ClosingTextOptions>(
      await apiClient.GET("/v1/tenant/conversations/{uuid}/closing-texts", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },
};
