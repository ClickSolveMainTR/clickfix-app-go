import { useQuery } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { orgKey, useOrgSlug } from '@/core/api/org';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';

import { forecastWindow, type Granularity, MODEL, timesheetWindow } from './core';

/**
 * Workforce planning, read-only (CF-170), the panel's Workforce page
 * (`frontend/src/features/workforce`). The forecast needs
 * `tenant.schedules.read`, timesheets `tenant.schedules.manage`
 * (`backend/internal/modules/workforce/routes.go`); a hook never calls an
 * endpoint the member cannot read.
 */
export type Forecast = components['schemas']['WorkforceForecast'];

export function useWorkforceScope() {
  const can = useCan();
  return { canForecast: can(P.schedulesRead), canTimesheets: can(P.schedulesRead) && can(P.schedulesManage) };
}

const keys = {
  all: (org: string) => orgKey(org, 'workforce'),
  forecast: (org: string, since: string, until: string, granularity: Granularity) =>
    [...keys.all(org), 'forecast', since, until, granularity] as const,
  timesheets: (org: string, since: string, until: string) => [...keys.all(org), 'timesheets', since, until] as const,
};

/**
 * Demand and staffing for the next `days` days with the panel's default
 * model parameters. `POST /v1/tenant/workforce/forecast` only calculates;
 * it stores nothing.
 */
export function useForecast(days: number, granularity: Granularity) {
  const org = useOrgSlug();
  const { canForecast } = useWorkforceScope();
  const { since, until } = forecastWindow(days);
  return useQuery({
    queryKey: keys.forecast(org, since, until, granularity),
    enabled: !!org && canForecast,
    staleTime: 5 * 60_000,
    placeholderData: (prev) => prev,
    queryFn: () =>
      unwrap(
        api.POST('/v1/tenant/workforce/forecast', {
          params: { header: orgHeader() },
          body: { since, until, granularity, ...MODEL },
        }),
      ),
  });
}

/** Activity rows (agent × state × conversation) of the last `days` days; the API takes a window, not a page. */
export function useTimesheets(days: number, enabled = true) {
  const org = useOrgSlug();
  const { canTimesheets } = useWorkforceScope();
  const { since, until } = timesheetWindow(days);
  return useQuery({
    queryKey: keys.timesheets(org, since, until),
    enabled: !!org && enabled && canTimesheets,
    staleTime: 60_000,
    placeholderData: (prev) => prev,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/workforce/timesheets', { params: { header: orgHeader(), query: { since, until } } })),
  });
}
