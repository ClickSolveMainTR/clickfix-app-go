import type {
  CustomStatus,
  CustomStatusInput,
} from "@/features/conversation-statuses/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/conversation-statuses";

/** Custom conversation statuses of one organization (`X-Organization`, CF-249). */
export const conversationStatusesService = {
  list(slug: string) {
    return tenantRequest<{ items: CustomStatus[] }>(slug, "GET", BASE);
  },

  create(slug: string, body: CustomStatusInput) {
    return tenantRequest<CustomStatus>(slug, "POST", BASE, { body });
  },

  update(slug: string, uuid: string, body: CustomStatusInput) {
    return tenantRequest<CustomStatus>(slug, "PATCH", `${BASE}/${uuid}`, {
      body,
    });
  },

  remove(slug: string, uuid: string) {
    return tenantRequest<{ deleted: boolean }>(
      slug,
      "DELETE",
      `${BASE}/${uuid}`,
    );
  },
};
