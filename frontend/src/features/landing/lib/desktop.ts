import { upstreamConfig } from "@/config/api";
import type { DesktopDownload } from "@/features/desktop/lib/downloads";

/**
 * Download page entries from the API (GET /v1/public/desktop/downloads,
 * CF-240), cached a minute. An unreachable API yields an empty list: the
 * page then says no release is available instead of failing.
 */
export async function fetchDesktopDownloads(): Promise<DesktopDownload[]> {
  try {
    const response = await fetch(
      `${upstreamConfig.baseUrl}/public/desktop/downloads`,
      {
        headers: { Accept: "application/json" },
        next: { revalidate: 60 },
        signal: AbortSignal.timeout(3000),
      },
    );
    if (!response.ok) return [];
    const json = (await response.json()) as {
      data?: { items?: DesktopDownload[] };
    };
    return json.data?.items ?? [];
  } catch {
    return [];
  }
}
