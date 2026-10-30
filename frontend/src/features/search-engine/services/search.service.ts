import { apiClient, unwrap } from "@/lib/api";
import { platformRequest } from "@/lib/api/platform-request";

import type {
  RemoteSearchSpec,
  SearchHit,
} from "@/features/search-engine/types";

type SearchSpecsResponse = {
  items: RemoteSearchSpec[];
  enabled: boolean;
};

type SearchHitsResponse = {
  items: SearchHit[];
};

export async function fetchSearchSpecs(): Promise<SearchSpecsResponse> {
  // Silent: organization staff have no platform role and get 403 here on
  // every page; the palette then just searches the organization index.
  return platformRequest<SearchSpecsResponse>("GET", "/v1/search/specs", {
    silent: true,
  });
}

export async function fetchSearchHits(
  q: string,
  spec?: string,
  limit = 20,
): Promise<SearchHit[]> {
  const data = await platformRequest<SearchHitsResponse>("GET", "/v1/search", {
    query: { q, spec, limit },
    silent: true,
  });
  return data.items ?? [];
}

/**
 * Organization search (CF-45): conversations, messages and notes, contacts,
 * companies of `org` only. The API filters by organization and by the
 * member's tenant permissions.
 */
export async function fetchTenantSearchSpecs(
  org: string,
): Promise<SearchSpecsResponse> {
  return unwrap<SearchSpecsResponse>(
    await apiClient.GET("/v1/tenant/search/specs", {
      params: { header: { "X-Organization": org } },
    }),
    { silent: true },
  );
}

export async function fetchTenantSearchHits(
  org: string,
  q: string,
  spec?: string,
  limit = 20,
): Promise<SearchHit[]> {
  const data = await unwrap<SearchHitsResponse>(
    await apiClient.GET("/v1/tenant/search", {
      params: {
        header: { "X-Organization": org },
        query: {
          q,
          limit,
          ...(spec
            ? {
                spec: spec as
                  "conversations" | "messages" | "contacts" | "companies",
              }
            : {}),
        },
      },
    }),
    { silent: true },
  );
  return data.items ?? [];
}
