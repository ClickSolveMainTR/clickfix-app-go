import type { components } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";
import { getActiveOrganizationSlug } from "@/features/organizations/lib/active-organization-store";

export type WidgetSession = components["schemas"]["WidgetSession"];
export type WidgetLiveTokens = components["schemas"]["WidgetLiveTokens"];
export type WidgetReplayManifest =
  components["schemas"]["WidgetReplayManifest"];
export type PointerType = "pointer" | "highlight" | "control_click";

/** rrweb `eventWithTime`; kept loose so rrweb stays a lazy import. */
export type ReplayEvent = { type: number; timestamp: number; data?: unknown };

function header() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

/** gzip magic bytes; the API serves chunks as `application/gzip`. */
function isGzip(bytes: Uint8Array) {
  return bytes.length > 2 && bytes[0] === 0x1f && bytes[1] === 0x8b;
}

async function readChunk(url: string): Promise<ReplayEvent[]> {
  // Manifest URLs are same-origin API paths; never follow anything else.
  if (!url.startsWith("/api/v1/public/widget-replays/")) {
    throw new Error("unexpected replay chunk url");
  }
  const res = await fetch(url, { credentials: "omit", cache: "no-store" });
  if (!res.ok) throw new Error(`replay chunk ${res.status}`);
  const bytes = new Uint8Array(await res.arrayBuffer());
  const body = isGzip(bytes)
    ? new Blob([bytes]).stream().pipeThrough(new DecompressionStream("gzip"))
    : new Blob([bytes]).stream();
  const events = (await new Response(body).json()) as unknown;
  return Array.isArray(events) ? (events as ReplayEvent[]) : [];
}

/**
 * Session replay and co-browse of a widget conversation (CF-126). Listing and
 * playback need tenant.replays.read; co-browse needs tenant.cobrowse.use
 * (guided clicks additionally tenant.cobrowse.control). Every call is audited
 * by the API.
 */
export const replayService = {
  async sessions(conversation: string) {
    const data = await unwrap<{ items?: WidgetSession[] }>(
      await apiClient.GET("/v1/tenant/conversations/{uuid}/sessions", {
        params: { header: header(), path: { uuid: conversation } },
      }),
      { silent: true },
    );
    return data.items ?? [];
  },

  async requestCobrowse(conversation: string, control: boolean) {
    return unwrap<WidgetSession>(
      await apiClient.POST("/v1/tenant/conversations/{uuid}/cobrowse", {
        params: { header: header(), path: { uuid: conversation } },
        body: { control_enabled: control },
      }),
      { silent: true },
    );
  },

  /**
   * Downloads every chunk of a recording (signed URLs live two minutes, so
   * they are fetched right away) and returns the events in time order.
   */
  async events(conversation: string, session: string) {
    const manifest = await unwrap<WidgetReplayManifest>(
      await apiClient.GET(
        "/v1/tenant/conversations/{uuid}/sessions/{session}/manifest",
        { params: { header: header(), path: { uuid: conversation, session } } },
      ),
      { silent: true },
    );
    const chunks = [...manifest.chunks].sort((a, b) => a.sequence - b.sequence);
    const parts = await Promise.all(chunks.map((c) => readChunk(c.url)));
    return {
      session: manifest.session,
      events: parts.flat().sort((a, b) => a.timestamp - b.timestamp),
    };
  },

  /** Stream tokens; the API answers 400 until the visitor accepted. */
  async liveToken(conversation: string, session: string) {
    return unwrap<WidgetLiveTokens>(
      await apiClient.POST(
        "/v1/tenant/conversations/{uuid}/sessions/{session}/live-token",
        { params: { header: header(), path: { uuid: conversation, session } } },
      ),
      { silent: true },
    );
  },

  async pointer(
    conversation: string,
    session: string,
    type: PointerType,
    x: number,
    y: number,
  ) {
    return unwrap<Record<string, unknown>>(
      await apiClient.POST(
        "/v1/tenant/conversations/{uuid}/sessions/{session}/pointer",
        {
          params: { header: header(), path: { uuid: conversation, session } },
          body: { type, x: Math.round(x), y: Math.round(y) },
        },
      ),
      { silent: true },
    );
  },

  async stop(conversation: string, session: string) {
    return unwrap<WidgetSession>(
      await apiClient.POST(
        "/v1/tenant/conversations/{uuid}/sessions/{session}/stop",
        { params: { header: header(), path: { uuid: conversation, session } } },
      ),
      { silent: true },
    );
  },
};
