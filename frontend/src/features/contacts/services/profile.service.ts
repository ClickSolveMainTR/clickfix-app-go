import type { components } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations/lib/active-organization-store";
import { apiClient, unwrap } from "@/lib/api";

export type ContactOverview = components["schemas"]["ContactOverview"];
export type ContactProfileStats = components["schemas"]["ContactProfileStats"];
export type ContactProfileConversation =
  components["schemas"]["ContactProfileConversation"];
export type ContactTimelineItem = components["schemas"]["ContactTimelineItem"];
export type ContactTimelinePage = components["schemas"]["ContactTimelinePage"];
export type ContactTimelineKind = components["schemas"]["ContactTimelineKind"];
export type ContactNote = components["schemas"]["ContactNote"];
export type ContactAISummary = components["schemas"]["ContactAISummary"];

export type ContactTimelineQuery = {
  kinds?: ContactTimelineKind[];
  q?: string;
  /** `yyyy-MM-dd`, inclusive, local day. */
  from?: string;
  /** `yyyy-MM-dd`, inclusive, local day. */
  to?: string;
};

function header() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

/** Local `yyyy-MM-dd` → RFC 3339 instant at that local midnight. */
function dayStart(day: string, plusDays = 0) {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(y, (m ?? 1) - 1, (d ?? 1) + plusDays).toISOString();
}

/** Contact page, unified timeline, notes and AI summary (CF-221). */
export const contactProfileService = {
  async overview(uuid: string) {
    return unwrap<ContactOverview>(
      await apiClient.GET("/v1/tenant/contacts/{uuid}/overview", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },
  async timeline(
    uuid: string,
    p: ContactTimelineQuery & { before?: string | null },
  ) {
    return unwrap<ContactTimelinePage>(
      await apiClient.GET("/v1/tenant/contacts/{uuid}/timeline", {
        params: {
          header: header(),
          path: { uuid },
          query: {
            limit: 30,
            ...(p.kinds?.length ? { kinds: p.kinds.join(",") } : {}),
            ...(p.q ? { q: p.q } : {}),
            ...(p.from ? { from: dayStart(p.from) } : {}),
            // `to` is inclusive in the UI: up to the next local midnight.
            ...(p.to ? { to: dayStart(p.to, 1) } : {}),
            ...(p.before ? { before: p.before } : {}),
          },
        },
      }),
    );
  },
  async addNote(uuid: string, body: string) {
    return unwrap<ContactNote>(
      await apiClient.POST("/v1/tenant/contacts/{uuid}/notes", {
        params: { header: header(), path: { uuid } },
        body: { body },
      }),
    );
  },
  async deleteNote(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/contact-notes/{uuid}", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },
  async generateSummary(uuid: string, locale: "tr" | "en") {
    return unwrap<ContactAISummary>(
      await apiClient.POST("/v1/tenant/contacts/{uuid}/ai-summary", {
        params: { header: header(), path: { uuid }, query: { locale } },
      }),
    );
  },
};
