"use client";

import { MotionConfig } from "motion/react";
import { createContext, useContext, useMemo, type ReactNode } from "react";

import type { AppLocale } from "@/config/i18n";
import { translate } from "@/lib/i18n/messages";

type LandingContextValue = {
  locale: AppLocale;
  /** Whether self sign-up is on (else the CTA asks for a demo). */
  signupOpen: boolean;
  t: (key: string, params?: Record<string, string | number>) => string;
};

const LandingContext = createContext<LandingContextValue | null>(null);

/**
 * The landing page renders in the language of its link (`?lang=en`), not in
 * the visitor's saved panel language, so its sections read the locale from
 * here instead of the app's LocaleProvider. Motion follows the OS setting.
 */
export function LandingProvider({
  locale,
  signupOpen,
  children,
}: {
  locale: AppLocale;
  signupOpen: boolean;
  children: ReactNode;
}) {
  const value = useMemo<LandingContextValue>(
    () => ({
      locale,
      signupOpen,
      t: (key, params) => translate(locale, key, params),
    }),
    [locale, signupOpen],
  );
  return (
    <LandingContext.Provider value={value}>
      <MotionConfig reducedMotion="user">{children}</MotionConfig>
    </LandingContext.Provider>
  );
}

export function useLanding() {
  const value = useContext(LandingContext);
  if (!value) throw new Error("useLanding outside LandingProvider");
  return value;
}
