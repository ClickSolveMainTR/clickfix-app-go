import { useSegments } from 'expo-router';
import { useEffect, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';

import { useOrgSlug } from '@/core/api/org';
import { useAppLock } from '@/features/security/app-lock';

import { claimGuide, saveGuideProgress, useGuidePreferences, useScreenGuides, useSetupGuides } from './api';
import { autoStartGuide, autoSetupGuide, routePattern } from './core';
import { GuideOverlay } from './overlay';
import { closeGuide, startGuide, useGuideTour } from './store';

/** The first screen after sign-in; the setup tour is offered there. */
const HOME_ROUTE = '/inbox';
/** Lets the screen lay out (and its targets register) before a tour points at them. */
const START_DELAY_MS = 900;

/** The route pattern of the screen on top (`/conversation/[uuid]`), as guides name it. */
export function useRoutePattern() {
  return routePattern(useSegments());
}

/**
 * Starts guides by themselves (CF-142): right after sign-in the setup tour,
 * once per user and topic across devices; on each screen its first unseen
 * tour, on the first visit. Never while the app lock or another guide is
 * up. Mounted once in the organization shell, with the overlay.
 */
export function GuideHost() {
  const org = useOrgSlug();
  const route = useRoutePattern();
  const locked = useAppLock((s) => s.locked);
  const active = useGuideTour((s) => !!s.active);
  const setup = useSetupGuides();
  const preferences = useGuidePreferences();
  const qc = useQueryClient();
  const previousRoute = useRef(route);
  useEffect(() => {
    if (previousRoute.current === route) return;
    previousRoute.current = route;
    const state = useGuideTour.getState();
    if (state.expectedRoute === route) {
      useGuideTour.setState({ expectedRoute: null });
      return;
    }
    const cur = state.active;
    if (cur) {
      void saveGuideProgress(qc, org, cur.guide.key, 'skipped', cur.index, cur.source === 'setup');
      closeGuide();
    }
  }, [route, org, qc]);
  useEffect(
    () => () => {
      const cur = useGuideTour.getState().active;
      if (cur) {
        void saveGuideProgress(qc, org, cur.guide.key, 'skipped', cur.index, cur.source === 'setup');
        closeGuide();
      }
    },
    [org, qc],
  );
  const screen = useScreenGuides(route);

  useEffect(() => {
    if (active || locked || !org || preferences.data?.auto_start !== true || !setup.data || !screen.data) return;
    const setupGuide = route === HOME_ROUTE ? autoSetupGuide(setup.data.items, setup.data.auto_start !== false) : null;
    const screenGuide = autoStartGuide(screen.data);
    const guide = setupGuide ?? screenGuide;
    if (!guide) return;
    let cancelled = false;
    const id = setTimeout(() => {
      void claimGuide(guide.key)
        .then((started) => {
          if (started && !cancelled && !useGuideTour.getState().active)
            startGuide(guide, setupGuide ? 'setup' : 'auto', true);
        })
        .catch(() => undefined);
    }, START_DELAY_MS);
    return () => {
      cancelled = true;
      clearTimeout(id);
    };
  }, [active, locked, org, route, setup.data, screen.data, preferences.data]);

  return <GuideOverlay />;
}
