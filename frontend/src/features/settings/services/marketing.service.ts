import type { components } from "@/generated/api";
import { ApiError } from "@/lib/api/errors";
import { platformFormRequest } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

export type MarketingConfig = Required<
  components["schemas"]["MarketingConfig"]
>;
export type MarketingSettings = components["schemas"]["MarketingSettings"];
export type MarketingSources = components["schemas"]["MarketingSources"];
export type OrganizationAttribution =
  components["schemas"]["OrganizationAttribution"];

/** Marketing site tracking settings and sign-up sources (CF-144). */
export const marketingService = {
  get() {
    return platformRequest<MarketingSettings>("GET", "/v1/platform/marketing");
  },

  save(body: MarketingConfig) {
    return platformRequest<MarketingSettings>("PUT", "/v1/platform/marketing", {
      body,
    });
  },

  uploadOGImage(file: File) {
    const form = new FormData();
    form.append("image", file);
    return platformFormRequest<MarketingSettings>(
      "PUT",
      "/v1/platform/marketing/og-image",
      form,
    );
  },

  deleteOGImage() {
    return platformRequest<MarketingSettings>(
      "DELETE",
      "/v1/platform/marketing/og-image",
    );
  },

  sources(from: string, to: string) {
    return platformRequest<MarketingSources>(
      "GET",
      "/v1/platform/marketing/sources",
      { query: { from, to } },
    );
  },

  /** Null when the organization has no recorded attribution (404). */
  async organizationAttribution(
    uuid: string,
  ): Promise<OrganizationAttribution | null> {
    try {
      return await platformRequest<OrganizationAttribution>(
        "GET",
        `/v1/platform/organizations/${encodeURIComponent(uuid)}/attribution`,
        { silent: true },
      );
    } catch (error) {
      if (error instanceof ApiError && error.isNotFound) return null;
      throw error;
    }
  },
};
