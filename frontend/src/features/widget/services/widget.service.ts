import type { components } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations";
import { apiClient, unwrap } from "@/lib/api";

/** A web widget is a channel instance of type `widget` (CF-55). */
export type WidgetInstance = components["schemas"]["ChannelInstance"];

type CreateBody = components["schemas"]["ChannelInstanceCreate"];
type UpdateBody = components["schemas"]["ChannelInstanceUpdate"];

export const WIDGET_TYPE = "widget";

/** Tenant routes are scoped by the active organization. */
function orgHeader() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

export const widgetService = {
  async list() {
    const data = await unwrap<{ items: WidgetInstance[] }>(
      await apiClient.GET("/v1/tenant/channels", {
        params: { header: orgHeader(), query: { type: WIDGET_TYPE } },
      }),
    );
    return data.items;
  },

  async create(body: Omit<CreateBody, "type">) {
    return unwrap<WidgetInstance>(
      await apiClient.POST("/v1/tenant/channels", {
        params: { header: orgHeader() },
        body: { ...body, type: WIDGET_TYPE },
      }),
    );
  },

  async update(uuid: string, body: UpdateBody) {
    return unwrap<WidgetInstance>(
      await apiClient.PATCH("/v1/tenant/channels/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
        body,
      }),
    );
  },

  async remove(uuid: string) {
    return unwrap<WidgetInstance>(
      await apiClient.DELETE("/v1/tenant/channels/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },
};

export type WidgetConfig = components["schemas"]["WidgetConfig"];
export type WidgetDraft = components["schemas"]["WidgetDraft"];
export type WidgetVersion = components["schemas"]["WidgetVersion"];
export type WidgetVersionDetail = components["schemas"]["WidgetVersionDetail"];
export type WidgetInstall = components["schemas"]["WidgetInstall"];
export type WidgetIdentityRejection =
  components["schemas"]["WidgetIdentityRejection"];
export type WidgetAppKey = components["schemas"]["WidgetAppKey"];
export type WidgetAppKeyCreated = components["schemas"]["WidgetAppKeyCreated"];
export type WidgetAppKeyCreate = components["schemas"]["WidgetAppKeyCreate"];

/** Editor API (CF-150): drafts, publishing, versions and installs. */
export const widgetConfigService = {
  async config(uuid: string) {
    return unwrap<WidgetConfig>(
      await apiClient.GET("/v1/tenant/widgets/{uuid}/config", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  async saveDraft(
    uuid: string,
    settings: Record<string, unknown>,
    baseVersion: number,
  ) {
    return unwrap<WidgetDraft>(
      await apiClient.PUT("/v1/tenant/widgets/{uuid}/draft", {
        params: { header: orgHeader(), path: { uuid } },
        body: { settings, base_version: baseVersion },
      }),
    );
  },

  async discardDraft(uuid: string) {
    return unwrap<{ discarded: boolean }>(
      await apiClient.DELETE("/v1/tenant/widgets/{uuid}/draft", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  async publish(
    uuid: string,
    body: {
      settings: Record<string, unknown>;
      base_version: number;
      note: string;
    },
  ) {
    return unwrap<WidgetVersion>(
      await apiClient.POST("/v1/tenant/widgets/{uuid}/publish", {
        params: { header: orgHeader(), path: { uuid } },
        body,
      }),
    );
  },

  async versions(uuid: string) {
    const data = await unwrap<{ items: WidgetVersion[] }>(
      await apiClient.GET("/v1/tenant/widgets/{uuid}/versions", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
    return data.items;
  },

  async version(uuid: string, version: number) {
    return unwrap<WidgetVersionDetail>(
      await apiClient.GET("/v1/tenant/widgets/{uuid}/versions/{version}", {
        params: { header: orgHeader(), path: { uuid, version } },
      }),
    );
  },

  async rollback(uuid: string, version: number, baseVersion: number) {
    return unwrap<WidgetVersion>(
      await apiClient.POST(
        "/v1/tenant/widgets/{uuid}/versions/{version}/rollback",
        {
          params: { header: orgHeader(), path: { uuid, version } },
          body: { base_version: baseVersion },
        },
      ),
    );
  },

  async duplicate(uuid: string, name: string) {
    return unwrap<WidgetInstance>(
      await apiClient.POST("/v1/tenant/widgets/{uuid}/duplicate", {
        params: { header: orgHeader(), path: { uuid } },
        body: { name },
      }),
    );
  },

  /** Install check: where the widget ran and its last rejected identify. */
  async installs(uuid: string) {
    return unwrap<{
      items: WidgetInstall[];
      identity_rejection: WidgetIdentityRejection | null;
    }>(
      await apiClient.GET("/v1/tenant/widgets/{uuid}/installs", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  /** Mobile app keys (CF-201): list, create (key shown once) and revoke. */
  async appKeys(uuid: string) {
    const data = await unwrap<{ items: WidgetAppKey[] }>(
      await apiClient.GET("/v1/tenant/widgets/{uuid}/app-keys", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
    return data.items;
  },

  async createAppKey(uuid: string, body: WidgetAppKeyCreate) {
    return unwrap<WidgetAppKeyCreated>(
      await apiClient.POST("/v1/tenant/widgets/{uuid}/app-keys", {
        params: { header: orgHeader(), path: { uuid } },
        body,
      }),
    );
  },

  async revokeAppKey(uuid: string, key: string) {
    return unwrap<WidgetAppKey>(
      await apiClient.DELETE("/v1/tenant/widgets/{uuid}/app-keys/{key}", {
        params: { header: orgHeader(), path: { uuid, key } },
      }),
    );
  },
};
