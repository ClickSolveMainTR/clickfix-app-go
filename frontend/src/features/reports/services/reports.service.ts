import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

type Schemas = components["schemas"];

export type CustomReport = Schemas["CustomReport"];
export type CustomReportInput = Schemas["CustomReportInput"];
export type ReportDefinition = Schemas["ReportDefinition"];
export type ReportDimension = ReportDefinition["dimensions"][number];
export type ReportMetric = ReportDefinition["metrics"][number];
export type ReportChartType = ReportDefinition["chart_type"];
export type ReportDataset = Schemas["ReportDataset"];
export type ReportRow = ReportDataset["rows"][number];
export type ReportSchedule = Schemas["ReportSchedule"];
export type ReportScheduleInput = Schemas["ReportScheduleInput"];

export type ReportMeta = {
  dimensions: ReportDimension[];
  max_dimensions: number;
  metrics: ReportMetric[];
  chart_types: ReportChartType[];
};

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

/** Evaluation window (RFC 3339, `to` exclusive). */
export type ReportWindow = { from: string; to: string };

/** Reports API (CF-112). Every route needs conversations read + read_all. */
export const reportsService = {
  meta(slug: string) {
    return tenantRequest<ReportMeta>(slug, "GET", "/v1/tenant/reports/meta");
  },

  scorecards(
    slug: string,
    params: ReportWindow & { team?: string; channel?: string },
  ) {
    return tenantRequest<ReportDataset>(
      slug,
      "GET",
      "/v1/tenant/agent-scorecards",
      { query: params },
    );
  },

  async list(slug: string) {
    const page = await tenantRequest<Page<CustomReport>>(
      slug,
      "GET",
      "/v1/tenant/reports",
      { query: { limit: 100, offset: 0 } },
    );
    return page.items;
  },

  create(slug: string, body: CustomReportInput) {
    return tenantRequest<CustomReport>(slug, "POST", "/v1/tenant/reports", {
      body,
    });
  },

  update(slug: string, uuid: string, body: CustomReportInput) {
    return tenantRequest<CustomReport>(
      slug,
      "PUT",
      `/v1/tenant/reports/${uuid}`,
      { body },
    );
  },

  remove(slug: string, uuid: string) {
    return tenantRequest<{ deleted: boolean }>(
      slug,
      "DELETE",
      `/v1/tenant/reports/${uuid}`,
    );
  },

  evaluate(slug: string, uuid: string, body: ReportWindow) {
    return tenantRequest<ReportDataset>(
      slug,
      "POST",
      `/v1/tenant/reports/${uuid}/evaluate`,
      { body },
    );
  },

  async schedules(slug: string) {
    const page = await tenantRequest<Page<ReportSchedule>>(
      slug,
      "GET",
      "/v1/tenant/report-schedules",
      { query: { limit: 100, offset: 0 } },
    );
    return page.items;
  },

  createSchedule(slug: string, body: ReportScheduleInput) {
    return tenantRequest<ReportSchedule>(
      slug,
      "POST",
      "/v1/tenant/report-schedules",
      { body },
    );
  },

  removeSchedule(slug: string, uuid: string) {
    return tenantRequest<{ deleted: boolean }>(
      slug,
      "DELETE",
      `/v1/tenant/report-schedules/${uuid}`,
    );
  },
};
