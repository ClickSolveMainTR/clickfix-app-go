import { useQuery } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { orgKey, useOrgSlug } from '@/core/api/org';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';

import { leaderboardWindow } from './core';

/**
 * Coaching and gamification, read-only (CF-170), the panel's Coaching page
 * (`frontend/src/features/coaching`). Leaderboard, badge rules and the own
 * opt-out need `tenant.schedules.read`; calibration and coaching items need
 * `tenant.quality.manage` (`backend/internal/modules/coaching/routes.go`).
 * A hook never calls an endpoint the member cannot read.
 */
export type Standing = components['schemas']['GamificationStanding'];
export type GameRule = components['schemas']['GamificationRule'];
export type CalibrationSummary = components['schemas']['CalibrationSummary'];
export type CalibrationSession = components['schemas']['CalibrationSession'];
export type CoachingItem = components['schemas']['CoachingItem'];

export function useCoachingScope() {
  const can = useCan();
  return { canGame: can(P.schedulesRead), canQuality: can(P.qualityManage) };
}

/** Page sizes the panel asks for. */
export const LEADERBOARD_PAGE = 50;
export const CALIBRATION_PAGE = 50;
export const ITEMS_PAGE = 100;

const keys = {
  all: (org: string) => orgKey(org, 'coaching'),
  leaderboard: (org: string, since: string, until: string) => [...keys.all(org), 'leaderboard', since, until] as const,
  preference: (org: string) => [...keys.all(org), 'preference'] as const,
  rules: (org: string) => [...keys.all(org), 'rules'] as const,
  calibrations: (org: string) => [...keys.all(org), 'calibrations'] as const,
  calibration: (org: string, uuid: string) => [...keys.all(org), 'calibration', uuid] as const,
  items: (org: string, agent: string) => [...keys.all(org), 'items', agent] as const,
};

export function useLeaderboard(days: number, enabled = true) {
  const org = useOrgSlug();
  const { canGame } = useCoachingScope();
  const { since, until } = leaderboardWindow(days);
  return useQuery({
    queryKey: keys.leaderboard(org, since, until),
    enabled: !!org && enabled && canGame,
    staleTime: 60_000,
    placeholderData: (prev) => prev,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/gamification/leaderboard', {
          params: { header: orgHeader(), query: { since, until, limit: LEADERBOARD_PAGE } },
        }),
      ),
  });
}

/** Whether the member has hidden themselves from the leaderboard (changed on the panel). */
export function useLeaderboardPreference(enabled = true) {
  const org = useOrgSlug();
  const { canGame } = useCoachingScope();
  return useQuery({
    queryKey: keys.preference(org),
    enabled: !!org && enabled && canGame,
    staleTime: 5 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/gamification/preferences', { params: { header: orgHeader() } })),
  });
}

/** Badge rules; the API returns all of them (at most 100) without paging. */
export function useGameRules(enabled = true) {
  const org = useOrgSlug();
  const { canGame } = useCoachingScope();
  return useQuery({
    queryKey: keys.rules(org),
    enabled: !!org && enabled && canGame,
    staleTime: 5 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/gamification/rules', { params: { header: orgHeader() } })),
  });
}

export function useCalibrations(enabled = true) {
  const org = useOrgSlug();
  const { canQuality } = useCoachingScope();
  return useQuery({
    queryKey: keys.calibrations(org),
    enabled: !!org && enabled && canQuality,
    staleTime: 60_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/quality/calibrations', {
          params: { header: orgHeader(), query: { limit: CALIBRATION_PAGE } },
        }),
      ),
  });
}

export function useCalibration(uuid: string) {
  const org = useOrgSlug();
  const { canQuality } = useCoachingScope();
  return useQuery({
    queryKey: keys.calibration(org, uuid),
    enabled: !!org && !!uuid && canQuality,
    staleTime: 60_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/quality/calibrations/{uuid}', { params: { header: orgHeader(), path: { uuid } } })),
  });
}

/** Notes, goals and follow-up tasks of one agent, newest first. */
export function useCoachingItems(agent: string) {
  const org = useOrgSlug();
  const { canQuality } = useCoachingScope();
  return useQuery({
    queryKey: keys.items(org, agent),
    enabled: !!org && !!agent && canQuality,
    staleTime: 60_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/coaching', {
          params: { header: orgHeader(), query: { agent_uuid: agent, limit: ITEMS_PAGE } },
        }),
      ),
  });
}
