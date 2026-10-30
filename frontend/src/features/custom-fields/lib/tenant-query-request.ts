import { apiConfig } from "@/config/api";
import { unwrap } from "@/lib/api/client";
import { getActiveOrganizationSlug } from "@/features/organizations/lib/active-organization-store";
import { withStepUpRetry } from "@/features/step-up-engine/lib/step-up-interceptor";

type QueryValue = string | number | boolean | undefined | null;

export type TenantRequestOptions = {
  query?: Record<string, QueryValue | QueryValue[]>;
  body?: unknown;
  /** Do not emit the global API error toast (the caller shows the error). */
  silent?: boolean;
};

const ORGANIZATION_HEADER = "X-Organization";

/**
 * BFF fetch for `/v1/tenant/*` endpoints with dynamic query parameters
 * (e.g. custom field filters `cf.<key>.<op>`) that the typed client cannot
 * express. Sends the active organization as `X-Organization`, like the
 * typed client's middleware.
 */
export async function tenantQueryRequest<T>(
  method: string,
  path: string,
  options?: TenantRequestOptions,
): Promise<T> {
  return withStepUpRetry(async () => {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const pathname = path.startsWith("/") ? path : `/${path}`;
    let url = `${base}${pathname}`;

    if (options?.query) {
      const params = new URLSearchParams();
      for (const [key, raw] of Object.entries(options.query)) {
        const values = Array.isArray(raw) ? raw : [raw];
        for (const value of values) {
          if (value !== undefined && value !== null && value !== "") {
            params.append(key, String(value));
          }
        }
      }
      const qs = params.toString();
      if (qs) url += `?${qs}`;
    }

    const headers: Record<string, string> = { Accept: "application/json" };
    if (options?.body !== undefined) {
      headers["Content-Type"] = "application/json";
    }
    const slug = getActiveOrganizationSlug();
    if (slug) headers[ORGANIZATION_HEADER] = slug;

    const response = await fetch(url, {
      method,
      credentials: "include",
      headers,
      body:
        options?.body !== undefined ? JSON.stringify(options.body) : undefined,
    });

    const payload = await response.json().catch(() => undefined);
    return unwrap<T>({ data: payload, response }, { silent: options?.silent });
  });
}
