import { apiConfig } from "@/config/api";

/**
 * Starts SP-initiated organization SSO (CF-128) through the BFF and returns
 * the IdP URL. The BFF keeps the API's state cookie and `next` (where to land
 * after the callback) server-side.
 */
export async function startOrganizationSSO(
  organization: string,
  next: string,
): Promise<string> {
  const base = apiConfig.baseUrl.replace(/\/$/, "");
  const query = new URLSearchParams({ next });
  const res = await fetch(
    `${base}/v1/public/sso/${organization}/start?${query.toString()}`,
    { credentials: "include", headers: { Accept: "application/json" } },
  );
  const payload = (await res.json().catch(() => null)) as {
    data?: { authorization_url?: string };
  } | null;
  const url = payload?.data?.authorization_url;
  if (!res.ok || !url) throw new Error(`SSO start failed (${res.status})`);
  return url;
}
