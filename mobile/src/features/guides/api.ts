import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { create } from 'zustand';

import { api, orgHeader, unwrap, unwrapVoid } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { orgKey, useOrgSlug } from '@/core/api/org';

import type { components } from '@/core/api/schema';

import type { Guide, GuideStatus } from './core';

export type { Guide, GuideStep, GuideStatus } from './core';

/**
 * Module guides (CF-142 mobile, CF-141 API). Contract:
 * `GET /v1/tenant/guides?surface=mobile&route=&locale=`, `GET /v1/tenant/guides/setup`,
 * `PUT /v1/tenant/guides/{key}/progress`, `POST /v1/tenant/guides/reset`.
 * A server without these routes answers 404: the guides then stay hidden.
 */
export type SetupGuides = components['schemas']['GuideSetupTour'];

/**
 * Which help features the server has (guides, assistant, product feedback).
 * A 404 turns one off until the app restarts, so a server without CF-141
 * never shows an entry point that cannot work.
 */
type Availability = { guides: boolean; assistant: boolean; feedback: boolean };
export const useHelpAvailability = create<Availability>(() => ({ guides: true, assistant: true, feedback: true }));

/**
 * Marks a feature missing when a collection route answers 404 (only called
 * for routes that exist whenever the feature does, never for one record).
 */
export function noteMissing(feature: keyof Availability, e: unknown) {
  if (isApiError(e) && e.status === 404) useHelpAvailability.setState({ [feature]: false });
}

/** Never retry a 404 (the route is missing); other failures retry like any query. */
export const retryUnlessMissing = (count: number, e: Error) => !(isApiError(e) && e.status === 404) && count < 2;

const keys = {
  all: (org: string) => orgKey(org, 'guides'),
  screen: (org: string, route: string, locale: string) => [...keys.all(org), 'screen', route, locale] as const,
  setup: (org: string, locale: string) => [...keys.all(org), 'setup', locale] as const,
};

async function fetchGuides(route: string, locale: string): Promise<Guide[]> {
  try {
    const r = await unwrap(
      api.GET('/v1/tenant/guides', {
        params: { header: orgHeader(), query: { surface: 'mobile', route, locale: locale === 'en' ? 'en' : 'tr' } },
      }),
    );
    return r?.items ?? [];
  } catch (e) {
    noteMissing('guides', e);
    throw e;
  }
}

/** The mobile guides written for a route pattern (`/inbox`, `/conversation/[uuid]`), in the app language. */
export function useScreenGuides(route: string) {
  const org = useOrgSlug();
  const { i18n } = useTranslation();
  const available = useHelpAvailability((s) => s.guides);
  return useQuery({
    queryKey: keys.screen(org, route, i18n.language),
    enabled: !!org && !!route && available,
    staleTime: 5 * 60_000,
    retry: retryUnlessMissing,
    queryFn: () => fetchGuides(route, i18n.language),
  });
}

/** The organization's ordered setup tour (onboarding checklist). */
export function useSetupGuides(enabled = true) {
  const org = useOrgSlug();
  const { i18n } = useTranslation();
  const available = useHelpAvailability((s) => s.guides);
  return useQuery({
    queryKey: keys.setup(org, i18n.language),
    enabled: !!org && available && enabled,
    staleTime: 60_000,
    retry: retryUnlessMissing,
    queryFn: async () => {
      try {
        return await unwrap(
          api.GET('/v1/tenant/guides/setup', {
            params: { header: orgHeader(), query: { surface: 'mobile', locale: i18n.language === 'en' ? 'en' : 'tr' } },
          }),
        );
      } catch (e) {
        noteMissing('guides', e);
        throw e;
      }
    },
  });
}

/** Writes a guide's progress into every cached list that holds it, so it never starts again on its own. */
function patchCached(qc: QueryClient, org: string, key: string, progress: Guide['progress']) {
  const patch = (g: Guide) => (g.key === key ? { ...g, progress } : g);
  qc.setQueriesData<Guide[]>({ queryKey: [...keys.all(org), 'screen'] }, (old) => old?.map(patch));
  qc.setQueriesData<SetupGuides>({ queryKey: [...keys.all(org), 'setup'] }, (old) => {
    if (!old) return old;
    const items = old.items.map(patch);
    const completed = items.filter((g) => g.progress.status === 'completed').length;
    return { ...old, items, completed };
  });
}

// Keep Next/Skip writes ordered: a delayed in_progress must not overwrite dismissal.
let progressWrite: Promise<unknown> = Promise.resolve();
export function saveGuideProgress(
  qc: QueryClient,
  org: string,
  key: string,
  status: GuideStatus,
  stepIndex: number,
  dismissSetup = false,
  restart = false,
) {
  const progress = { status, step_index: stepIndex, dismiss_setup: dismissSetup, restart };
  patchCached(qc, org, key, progress);
  progressWrite = progressWrite
    .catch(() => undefined)
    .then(() =>
      unwrapVoid(
        api.PUT('/v1/tenant/guides/{key}/progress', {
          params: { header: { 'X-Organization': org }, path: { key } },
          body: progress,
        }),
      ),
    )
    .catch(() => undefined);
  return progressWrite;
}

/** "Show the guides again": every tour starts once more on its screen. */
export function useResetGuides() {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrapVoid(api.POST('/v1/tenant/guides/reset', { params: { header: orgHeader() } })),
    onSettled: () => qc.invalidateQueries({ queryKey: keys.all(org) }),
  });
}

/** The setup tour as last fetched (the overlay moves on to its next item without a round trip). */
export function cachedSetupGuides(qc: QueryClient, org: string): Guide[] {
  const found = qc.getQueriesData<SetupGuides>({ queryKey: [...keys.all(org), 'setup'] });
  return found.find(([, d]) => !!d)?.[1]?.items ?? [];
}

export function useGuidePreferences() {
  const org = useOrgSlug();
  return useQuery({
    queryKey: [...keys.all(org), 'preferences'],
    enabled: !!org,
    staleTime: 0,
    queryFn: () => unwrap(api.GET('/v1/tenant/guides/preferences', { params: { header: orgHeader() } })),
  });
}

export function useSetGuidePreferences() {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (auto_start: boolean) =>
      unwrap(api.PUT('/v1/tenant/guides/preferences', { params: { header: orgHeader() }, body: { auto_start } })),
    onSuccess: (data) => qc.setQueryData([...keys.all(org), 'preferences'], data),
  });
}

export async function claimGuide(key: string) {
  return (
    await unwrap(api.POST('/v1/tenant/guides/{key}/auto-start', { params: { header: orgHeader(), path: { key } } }))
  ).started;
}
