import { router, type Href } from 'expo-router';
import { Keyboard } from 'react-native';
import { create } from 'zustand';

import { type Guide, resumeIndex } from './core';

/** Why a guide is on screen: it started by itself, the member asked, or it is part of the setup tour. */
export type GuideSource = 'auto' | 'manual' | 'setup';

type ActiveGuide = { guide: Guide; index: number; source: GuideSource; manual: boolean };

type TourState = {
  active: ActiveGuide | null;
  expectedRoute: string | null;
};

export const useGuideTour = create<TourState>(() => ({ active: null, expectedRoute: null }));

/** Plays a guide from its saved step (`manual`/`setup` from the start when it is finished). */
export function startGuide(guide: Guide, source: GuideSource, automatic = false) {
  if (!guide.steps.length) return;
  Keyboard.dismiss();
  if (source === 'setup' && guide.route && !guide.route.includes('[')) {
    expectGuideRoute(guide.route);
    router.navigate(guide.route as Href);
  }
  useGuideTour.setState({
    active: { guide, index: resumeIndex(guide), source, manual: source !== 'auto' && !automatic },
  });
}

export function setGuideStep(index: number) {
  useGuideTour.setState((s) => (s.active ? { active: { ...s.active, index } } : s));
}

export function closeGuide() {
  useGuideTour.setState({ active: null, expectedRoute: null });
}

/** A tour-owned navigation is allowed; ordinary navigation dismisses it. */
export function expectGuideRoute(route: string) {
  useGuideTour.setState({ expectedRoute: route });
}
