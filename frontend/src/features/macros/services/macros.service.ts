import type { components } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations/lib/active-organization-store";
import { apiClient, unwrap } from "@/lib/api";

export type Canned = components["schemas"]["Canned"];
export type CannedInput = components["schemas"]["CannedInput"];
export type Macro = components["schemas"]["Macro"];
export type MacroInput = components["schemas"]["MacroInput"];
export type MacroAction = components["schemas"]["MacroAction"];

function header() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

export const macrosService = {
  async canned(q?: string) {
    const res = await unwrap<{ items: Canned[] }>(
      await apiClient.GET("/v1/tenant/canned-responses", {
        params: { header: header(), query: q ? { q } : {} },
      }),
    );
    return res.items;
  },
  async createCanned(body: CannedInput) {
    return unwrap<Canned>(
      await apiClient.POST("/v1/tenant/canned-responses", {
        params: { header: header() },
        body,
      }),
    );
  },
  async updateCanned(uuid: string, body: CannedInput) {
    return unwrap<Canned>(
      await apiClient.PATCH("/v1/tenant/canned-responses/{uuid}", {
        params: { header: header(), path: { uuid } },
        body,
      }),
    );
  },
  async deleteCanned(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/canned-responses/{uuid}", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },
  async render(uuid: string, conversation: string) {
    const res = await unwrap<{ content: string }>(
      await apiClient.POST("/v1/tenant/canned-responses/{uuid}/render", {
        params: { header: header(), path: { uuid } },
        body: { conversation },
      }),
    );
    return res.content;
  },
  async macros() {
    const res = await unwrap<{ items: Macro[] }>(
      await apiClient.GET("/v1/tenant/macros", {
        params: { header: header() },
      }),
    );
    return res.items;
  },
  async createMacro(body: MacroInput) {
    return unwrap<Macro>(
      await apiClient.POST("/v1/tenant/macros", {
        params: { header: header() },
        body,
      }),
    );
  },
  async updateMacro(uuid: string, body: MacroInput) {
    return unwrap<Macro>(
      await apiClient.PATCH("/v1/tenant/macros/{uuid}", {
        params: { header: header(), path: { uuid } },
        body,
      }),
    );
  },
  async deleteMacro(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/macros/{uuid}", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },
  async apply(conversation: string, macro: string) {
    return unwrap<{ applied: string[] }>(
      await apiClient.POST("/v1/tenant/conversations/{uuid}/macros/{macro}", {
        params: { header: header(), path: { uuid: conversation, macro } },
      }),
    );
  },
};
