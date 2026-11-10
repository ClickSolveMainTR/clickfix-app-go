import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type AutoReplySettings = components["schemas"]["AutoReplySettings"];
export type AutoReply = components["schemas"]["AutoReply"];

const PATH = "/v1/tenant/auto-replies";

/**
 * Automatic customer replies (CF-198): the acknowledgement, the out-of-hours
 * message and the AI-to-human handoff notice, one source per text.
 */
export const autoRepliesService = {
  get(slug: string) {
    return tenantRequest<AutoReplySettings>(slug, "GET", PATH);
  },
  update(slug: string, body: AutoReplySettings) {
    return tenantRequest<AutoReplySettings>(slug, "PUT", PATH, { body });
  },
};
