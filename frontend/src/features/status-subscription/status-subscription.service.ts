import type { paths } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";

type SubscriptionPath = "/v1/public/status/{slug}/subscriptions/{token}";

export type StatusSubscriptionPreview = NonNullable<
  paths[SubscriptionPath]["get"]["responses"][200]["content"]["application/json"]["data"]
>;

/**
 * Status page e-mail links (CF-157). Opening a link only previews it; the
 * subscription changes on the page's button (POST), so link scanners and
 * previews cannot confirm or cancel it.
 */
export const statusSubscriptionService = {
  async preview(slug: string, token: string) {
    return unwrap<StatusSubscriptionPreview>(
      await apiClient.GET("/v1/public/status/{slug}/subscriptions/{token}", {
        params: { path: { slug, token } },
      }),
    );
  },

  async apply(slug: string, token: string) {
    return unwrap<{ ok: boolean }>(
      await apiClient.POST("/v1/public/status/{slug}/subscriptions/{token}", {
        params: { path: { slug, token } },
      }),
    );
  },
};
