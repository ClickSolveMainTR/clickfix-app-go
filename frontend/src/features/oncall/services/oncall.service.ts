import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

type Schemas = components["schemas"];

export type OncallRota = Schemas["OncallRota"];
export type OncallRotaInput = Schemas["OncallRotaInput"];
export type OncallLayer = Schemas["OncallLayer"];
export type OncallOverride = Schemas["OncallOverride"];
export type EscalationPolicy = Schemas["EscalationPolicy"];
export type EscalationPolicyInput = Schemas["EscalationPolicyInput"];
export type EscalationStep = Schemas["EscalationStep"];
export type EscalationChannel = EscalationStep["channels"][number];
export type UptimeMonitor = Schemas["UptimeMonitor"];
export type UptimeMonitorInput = Schemas["UptimeMonitorInput"];
export type UptimeResult = Schemas["UptimeResult"];
export type OncallAlert = Schemas["OncallAlert"];
export type StatusComponent = Schemas["StatusComponent"];
export type StatusComponentInput = Schemas["StatusComponentInput"];
export type StatusIncident = Schemas["StatusIncident"];

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

function list<T>(slug: string, path: string, limit = 100) {
  return tenantRequest<Page<T>>(slug, "GET", path, {
    query: { limit, offset: 0 },
  }).then((page) => page.items);
}

/**
 * On-call and uptime API (CF-104). Reads need tenant.schedules.read,
 * changes tenant.schedules.manage; both also need conversations read_all.
 */
export const oncallService = {
  rotas: (slug: string) => list<OncallRota>(slug, "/v1/tenant/oncall/rotas"),
  createRota: (slug: string, body: OncallRotaInput) =>
    tenantRequest<OncallRota>(slug, "POST", "/v1/tenant/oncall/rotas", {
      body,
    }),
  updateRota: (slug: string, uuid: string, body: OncallRotaInput) =>
    tenantRequest<OncallRota>(slug, "PUT", `/v1/tenant/oncall/rotas/${uuid}`, {
      body,
    }),
  removeRota: (slug: string, uuid: string) =>
    tenantRequest<{ deleted: boolean }>(
      slug,
      "DELETE",
      `/v1/tenant/oncall/rotas/${uuid}`,
    ),
  current: (slug: string, uuid: string) =>
    tenantRequest<{ agents: string[]; at: string }>(
      slug,
      "GET",
      `/v1/tenant/oncall/rotas/${uuid}/current`,
    ),

  policies: (slug: string) =>
    list<EscalationPolicy>(slug, "/v1/tenant/oncall/policies"),
  createPolicy: (slug: string, body: EscalationPolicyInput) =>
    tenantRequest<EscalationPolicy>(
      slug,
      "POST",
      "/v1/tenant/oncall/policies",
      {
        body,
      },
    ),
  updatePolicy: (slug: string, uuid: string, body: EscalationPolicyInput) =>
    tenantRequest<EscalationPolicy>(
      slug,
      "PUT",
      `/v1/tenant/oncall/policies/${uuid}`,
      { body },
    ),
  removePolicy: (slug: string, uuid: string) =>
    tenantRequest<{ deleted: boolean }>(
      slug,
      "DELETE",
      `/v1/tenant/oncall/policies/${uuid}`,
    ),

  alerts: (slug: string) =>
    list<OncallAlert>(slug, "/v1/tenant/oncall/alerts", 50),
  acknowledge: (slug: string, uuid: string) =>
    tenantRequest<{ acknowledged: boolean }>(
      slug,
      "POST",
      `/v1/tenant/oncall/alerts/${uuid}/acknowledge`,
    ),

  monitors: (slug: string) =>
    list<UptimeMonitor>(slug, "/v1/tenant/uptime-monitors"),
  createMonitor: (slug: string, body: UptimeMonitorInput) =>
    tenantRequest<UptimeMonitor>(slug, "POST", "/v1/tenant/uptime-monitors", {
      body,
    }),
  updateMonitor: (slug: string, uuid: string, body: UptimeMonitorInput) =>
    tenantRequest<UptimeMonitor>(
      slug,
      "PUT",
      `/v1/tenant/uptime-monitors/${uuid}`,
      { body },
    ),
  removeMonitor: (slug: string, uuid: string) =>
    tenantRequest<{ deleted: boolean }>(
      slug,
      "DELETE",
      `/v1/tenant/uptime-monitors/${uuid}`,
    ),
  /** Newest first; checks are kept for 30 days. */
  results: (slug: string, uuid: string, limit = 30) =>
    list<UptimeResult>(
      slug,
      `/v1/tenant/uptime-monitors/${uuid}/results`,
      limit,
    ),

  /** Status-page components a monitor drives (CF-103). */
  components: (slug: string) =>
    tenantRequest<{ items?: StatusComponent[] }>(
      slug,
      "GET",
      "/v1/tenant/status/components",
    ).then((page) => page.items ?? []),
  createComponent: (slug: string, body: StatusComponentInput) =>
    tenantRequest<StatusComponent>(
      slug,
      "POST",
      "/v1/tenant/status/components",
      {
        body,
      },
    ),
  /**
   * One page of incidents, newest first. Needs tenant.conversations.read
   * only, so agents can read it (CF-162).
   */
  incidentsPage: (slug: string, params: { limit: number; offset: number }) =>
    tenantRequest<Partial<Page<StatusIncident>>>(
      slug,
      "GET",
      "/v1/tenant/status/incidents",
      { query: params },
    ),
  incidents: (slug: string) =>
    oncallService
      .incidentsPage(slug, { limit: 50, offset: 0 })
      .then((page) => page.items ?? []),
};
