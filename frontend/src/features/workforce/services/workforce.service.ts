import type { components } from "@/generated/api";
import { tenantDownloadFile, tenantRequest } from "@/lib/api/tenant-request";

type Schemas = components["schemas"];

export type ForecastInput = Schemas["WorkforceForecastInput"];
export type Forecast = Schemas["WorkforceForecast"];
export type ForecastHour = Schemas["WorkforceHour"];
export type ForecastDay = Schemas["WorkforceDay"];
export type Timesheet = Schemas["WorkforceTimesheet"];
export type ActivityInput = Schemas["WorkforceActivityInput"];
export type ActivityState = ActivityInput["state"];
export type TimesheetFormat = "csv" | "xlsx" | "json";

/** Half-open `[since, until)` RFC 3339 window. */
export type TimesheetWindow = { since: string; until: string };

/**
 * Workforce API (CF-107). Forecast and activity need tenant.schedules.read;
 * timesheets and their export need tenant.schedules.manage.
 */
export const workforceService = {
  forecast(slug: string, body: ForecastInput) {
    return tenantRequest<Forecast>(
      slug,
      "POST",
      "/v1/tenant/workforce/forecast",
      { body, silent: true },
    );
  },

  /** Records the signed-in member's own state (heartbeat / transition). */
  activity(slug: string, body: ActivityInput) {
    return tenantRequest<Schemas["WorkforceActivity"]>(
      slug,
      "POST",
      "/v1/tenant/workforce/activity",
      { body, silent: true },
    );
  },

  timesheets(slug: string, params: TimesheetWindow & { agent?: string }) {
    return tenantRequest<Timesheet[]>(
      slug,
      "GET",
      "/v1/tenant/workforce/timesheets",
      {
        query: {
          since: params.since,
          until: params.until,
          agent_uuid: params.agent,
        },
        silent: true,
      },
    );
  },

  exportTimesheets(
    slug: string,
    params: TimesheetWindow & { agent?: string; format: TimesheetFormat },
  ) {
    return tenantDownloadFile(slug, "/v1/tenant/workforce/timesheets/export", {
      since: params.since,
      until: params.until,
      agent_uuid: params.agent,
      format: params.format,
    });
  },
};
