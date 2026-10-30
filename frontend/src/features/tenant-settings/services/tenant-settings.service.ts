import type {
  OrganizationProfile,
  OrganizationProfilePatchRequest,
} from "@/features/tenant-settings/types";
import { tenantFormRequest, tenantRequest } from "@/lib/api/tenant-request";

const PATH = "/v1/tenant/organization";

/** Organization profile of one organization (`X-Organization`). */
export const tenantSettingsService = {
  get(slug: string) {
    return tenantRequest<OrganizationProfile>(slug, "GET", PATH);
  },

  /** Any subset of the profile; objects are replaced whole. */
  update(slug: string, body: OrganizationProfilePatchRequest) {
    return tenantRequest<OrganizationProfile>(slug, "PATCH", PATH, { body });
  },

  uploadLogo(slug: string, file: File) {
    const form = new FormData();
    form.append("logo", file);
    return tenantFormRequest<OrganizationProfile>(
      slug,
      "PUT",
      `${PATH}/logo`,
      form,
    );
  },

  removeLogo(slug: string) {
    return tenantRequest<OrganizationProfile>(slug, "DELETE", `${PATH}/logo`);
  },
};
