import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

type Schemas = components["schemas"];

export type CalibrationSummary = Schemas["CalibrationSummary"];
export type CalibrationSession = Schemas["CalibrationSession"];
export type CalibrationInput = Schemas["CalibrationInput"];
export type CalibrationScoreValue = Schemas["CalibrationScoreValue"];
export type CalibrationScore = Schemas["CalibrationScore"];
export type CalibrationVariance = Schemas["CalibrationVariance"];
export type RubricCriterion = CalibrationSession["rubric"][number];
export type Scorecard = Schemas["QAScorecard"];
export type CoachingItem = Schemas["CoachingItem"];
export type CoachingInput = Schemas["CoachingInput"];
export type CoachingKind = CoachingInput["kind"];
export type GameRule = Schemas["GamificationRule"];
export type GameRuleInput = Schemas["GamificationRuleInput"];
export type GameMetric = GameRuleInput["metric"];
export type Standing = Schemas["GamificationStanding"];

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/**
 * Calibration, coaching and gamification API (CF-109). Calibration and
 * coaching need tenant.quality.manage; rules (read), preferences and the
 * leaderboard need tenant.schedules.read; saving rules needs quality.manage.
 */
export const coachingService = {
  calibrations(slug: string, params: { limit: number; offset: number }) {
    return tenantRequest<Page<CalibrationSummary>>(
      slug,
      "GET",
      "/v1/tenant/quality/calibrations",
      { query: params, silent: true },
    );
  },

  calibration(slug: string, uuid: string) {
    return tenantRequest<CalibrationSession>(
      slug,
      "GET",
      `/v1/tenant/quality/calibrations/${uuid}`,
      { silent: true },
    );
  },

  createCalibration(slug: string, body: CalibrationInput) {
    return tenantRequest<CalibrationSession>(
      slug,
      "POST",
      "/v1/tenant/quality/calibrations",
      { body },
    );
  },

  score(slug: string, uuid: string, scores: CalibrationScoreValue[]) {
    return tenantRequest<CalibrationScore>(
      slug,
      "POST",
      `/v1/tenant/quality/calibrations/${uuid}/scores`,
      { body: { scores } },
    );
  },

  closeCalibration(slug: string, uuid: string) {
    return tenantRequest<{ closed: boolean }>(
      slug,
      "POST",
      `/v1/tenant/quality/calibrations/${uuid}/close`,
    );
  },

  /** CF-108 scorecards (tenant.quality.read). */
  scorecards(slug: string) {
    return tenantRequest<Scorecard[]>(
      slug,
      "GET",
      "/v1/tenant/quality/scorecards",
      { silent: true },
    );
  },

  items(
    slug: string,
    params: { agent: string; limit: number; offset: number },
  ) {
    return tenantRequest<Page<CoachingItem>>(
      slug,
      "GET",
      "/v1/tenant/coaching",
      {
        query: {
          agent_uuid: params.agent,
          limit: params.limit,
          offset: params.offset,
        },
        silent: true,
      },
    );
  },

  createItem(slug: string, body: CoachingInput) {
    return tenantRequest<CoachingItem>(slug, "POST", "/v1/tenant/coaching", {
      body,
    });
  },

  completeItem(slug: string, uuid: string, completed: boolean) {
    return tenantRequest<{ completed: boolean }>(
      slug,
      "PATCH",
      `/v1/tenant/coaching/${uuid}`,
      { body: { completed } },
    );
  },

  rules(slug: string) {
    return tenantRequest<GameRule[]>(
      slug,
      "GET",
      "/v1/tenant/gamification/rules",
      { silent: true },
    );
  },

  /** Upsert by name: an existing name updates that rule. */
  saveRule(slug: string, body: GameRuleInput) {
    return tenantRequest<GameRule>(
      slug,
      "PUT",
      "/v1/tenant/gamification/rules",
      { body },
    );
  },

  preference(slug: string) {
    return tenantRequest<{ opted_out: boolean }>(
      slug,
      "GET",
      "/v1/tenant/gamification/preferences",
      { silent: true },
    );
  },

  setPreference(slug: string, optedOut: boolean) {
    return tenantRequest<{ opted_out: boolean }>(
      slug,
      "PUT",
      "/v1/tenant/gamification/preferences",
      { body: { opted_out: optedOut } },
    );
  },

  leaderboard(
    slug: string,
    params: {
      since: string;
      until: string;
      team?: string;
      limit: number;
      offset: number;
    },
  ) {
    return tenantRequest<Page<Standing>>(
      slug,
      "GET",
      "/v1/tenant/gamification/leaderboard",
      {
        query: {
          since: params.since,
          until: params.until,
          team_uuid: params.team,
          limit: params.limit,
          offset: params.offset,
        },
        silent: true,
      },
    );
  },
};
