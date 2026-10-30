import type { components } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations";
import { apiClient, unwrap } from "@/lib/api";

export type Brand = components["schemas"]["Brand"];
export type BrandInput = components["schemas"]["BrandInput"];
export type BrandBusinessHours = components["schemas"]["WidgetBusinessHours"];

/** Tenant routes are scoped by the active organization. */
function orgHeader() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

/**
 * Organization brands (CF-124): identity, sender, help center host/path,
 * widget defaults, signature, hours and SLA override. Read with
 * tenant.channels.read, write with tenant.channels.manage.
 */
export const brandsService = {
  async list() {
    const data = await unwrap<{ items?: Brand[] }>(
      await apiClient.GET("/v1/tenant/brands", {
        params: { header: orgHeader() },
      }),
    );
    return data.items ?? [];
  },

  async create(body: BrandInput) {
    return unwrap<Brand>(
      await apiClient.POST("/v1/tenant/brands", {
        params: { header: orgHeader() },
        body,
      }),
    );
  },

  async replace(uuid: string, body: BrandInput) {
    return unwrap<Brand>(
      await apiClient.PUT("/v1/tenant/brands/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
        body,
      }),
    );
  },

  async remove(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/brands/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  /** Puts a knowledge article on a brand's help center (tenant.knowledge.write). */
  async assignArticle(uuid: string, article: string) {
    return unwrap(
      await apiClient.PUT("/v1/tenant/brands/{uuid}/articles/{article}", {
        params: { header: orgHeader(), path: { uuid, article } },
      }),
    );
  },

  /**
   * Uploads a logo through the tenant attachments flow; the brand references
   * the returned (unclaimed) attachment id and the API owns the URL.
   */
  async uploadLogo(file: File) {
    const form = new FormData();
    form.append("file", file);
    return unwrap<{ uuid: string }>(
      await apiClient.POST("/v1/tenant/attachments", {
        params: { header: orgHeader() },
        body: form as never,
        bodySerializer: (b: unknown) => b as FormData,
      }),
    );
  },
};
