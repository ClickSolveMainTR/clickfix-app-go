"use client";

import { usePathname } from "next/navigation";
import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";

import type { AppLocale } from "@/config/i18n";
import { localizedHref } from "@/features/landing/lib/locale";
import {
  applyTags,
  clearAttribution,
  CONSENT_OPEN_EVENT,
  consentSnapshot,
  currentTouch,
  needsReload,
  parseConsent,
  readConsent,
  recordTouch,
  subscribeConsent,
  trackPageView,
  writeConsent,
  type Consent,
  type TrackingConfig,
} from "@/features/landing/lib/tracking";
import { translate } from "@/lib/i18n/messages";
import { cn } from "@/lib/utils";

/**
 * Cookie consent and marketing tags of the public marketing pages (CF-144).
 * Nothing but the consent cookie is written and no third-party script loads
 * until the visitor picks categories; Google tags get Consent Mode v2
 * signals. Campaign parameters of the visit are kept in this tab until then.
 * Mounted by `MarketingTrackingSlot` on every marketing page; the banner is
 * opened after the browser checks consent (fixed position: no CLS).
 */
export function MarketingTracking({
  locale,
  config,
}: {
  locale: AppLocale;
  config: TrackingConfig;
}) {
  const pathname = usePathname();
  const t = useCallback(
    (key: string) => translate(locale, `landing.consent.${key}`),
    [locale],
  );

  // Consent is unknown during SSR and hydration. Read it in the browser
  // before showing the fixed banner or loading any tracking tags.
  const raw = useSyncExternalStore(
    subscribeConsent,
    consentSnapshot,
    () => null,
  );
  const consent = useMemo(
    () => (raw === null ? undefined : parseConsent(raw)),
    [raw],
  );
  const [reopened, setReopened] = useState(false);
  const [customize, setCustomize] = useState(false);
  const [draft, setDraft] = useState<Consent>({
    analytics: false,
    marketing: false,
  });
  const firstPath = useRef(true);
  const titleId = useId();
  const open = reopened || consent === null;
  // Docs readers get a one-line bar at the bottom instead of the card (CF-208).
  const compact = /^\/(?:en\/)?docs(?:\/|$)/.test(pathname) && !customize;

  // Capture this visit's campaign parameters; reopen from the footer link.
  useEffect(() => {
    recordTouch(currentTouch(), readConsent());
    const reopen = () => {
      setDraft(readConsent() ?? { analytics: false, marketing: false });
      setCustomize(true);
      setReopened(true);
    };
    window.addEventListener(CONSENT_OPEN_EVENT, reopen);
    return () => window.removeEventListener(CONSENT_OPEN_EVENT, reopen);
  }, []);

  useEffect(() => {
    if (consent) applyTags(config, consent);
  }, [config, consent]);

  useEffect(() => {
    if (firstPath.current) {
      firstPath.current = false;
      return;
    }
    trackPageView();
  }, [pathname]);

  const decide = (next: Consent) => {
    const previous = consent ?? null;
    writeConsent(next);
    setReopened(false);
    setCustomize(false);
    if (!next.analytics) clearAttribution();
    else recordTouch(null, next);
    if (needsReload(previous, next)) window.location.reload();
  };

  if (!open) return null;

  const categories = [
    { key: "necessary", locked: true },
    { key: "analytics", locked: false },
    { key: "marketing", locked: false },
  ] as const;

  return (
    <section
      role="dialog"
      aria-modal="false"
      aria-labelledby={titleId}
      lang={locale}
      className={
        compact
          ? "fixed inset-x-0 bottom-0 z-[80] flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-white/15 bg-[#0F1310] px-4 py-2.5 text-white sm:px-6"
          : "fixed inset-x-3 bottom-3 z-[80] mx-auto max-w-xl rounded-2xl border border-white/15 bg-[#0F1310] p-5 text-white shadow-[0_20px_60px_-20px_rgb(0_0_0/0.8)] sm:inset-x-auto sm:right-5 sm:bottom-5"
      }
    >
      <h2
        id={titleId}
        className={compact ? "sr-only" : "font-display text-base font-semibold"}
      >
        {t("title")}
      </h2>
      <p
        className={
          compact
            ? "min-w-0 flex-1 text-xs leading-relaxed text-white/70"
            : "mt-2 text-sm leading-relaxed text-white/70"
        }
      >
        {t("body")}{" "}
        <a
          href={localizedHref("/legal/cookies", locale)}
          className="text-[#7DFF86] underline underline-offset-2 hover:text-white"
        >
          {t("policy")}
        </a>
      </p>

      {customize ? (
        <fieldset className="mt-4 space-y-3">
          <legend className="sr-only">{t("categories")}</legend>
          {categories.map(({ key, locked }) => {
            const id = `cf-consent-${key}`;
            const checked = locked ? true : draft[key];
            return (
              <div key={key} className="flex items-start gap-3">
                <input
                  id={id}
                  type="checkbox"
                  className="mt-1 size-4 accent-[#03FF13]"
                  checked={checked}
                  disabled={locked}
                  aria-describedby={`${id}-hint`}
                  onChange={(event) => {
                    if (locked) return;
                    setDraft((d) => ({ ...d, [key]: event.target.checked }));
                  }}
                />
                <label htmlFor={id} className="text-sm">
                  <span className="font-medium">{t(`${key}_title`)}</span>
                  <span
                    id={`${id}-hint`}
                    className="block text-xs leading-relaxed text-white/60"
                  >
                    {t(`${key}_body`)}
                  </span>
                </label>
              </div>
            );
          })}
        </fieldset>
      ) : null}

      <div
        className={
          compact ? "flex flex-wrap gap-2" : "mt-4 flex flex-wrap gap-2"
        }
      >
        {customize ? (
          <button
            type="button"
            onClick={() => decide(draft)}
            className="h-10 rounded-full bg-[#03FF13] px-4 text-sm font-semibold text-black hover:bg-[#3DFF4B]"
          >
            {t("save")}
          </button>
        ) : (
          <button
            type="button"
            onClick={() => setCustomize(true)}
            className={cn(
              "rounded-full border border-white/20 px-4 text-sm font-medium hover:border-white/40",
              compact ? "h-8 text-xs" : "h-10",
            )}
          >
            {t("customize")}
          </button>
        )}
        <button
          type="button"
          onClick={() => decide({ analytics: false, marketing: false })}
          className={cn(
            "rounded-full border border-white/20 px-4 text-sm font-medium hover:border-white/40",
            compact ? "h-8 text-xs" : "h-10",
          )}
        >
          {t("reject")}
        </button>
        <button
          type="button"
          onClick={() => decide({ analytics: true, marketing: true })}
          className={cn(
            customize
              ? "rounded-full border border-white/20 px-4 text-sm font-medium hover:border-white/40"
              : "rounded-full bg-[#03FF13] px-4 text-sm font-semibold text-black hover:bg-[#3DFF4B]",
            compact ? "h-8 text-xs" : "h-10",
          )}
        >
          {t("accept_all")}
        </button>
      </div>
    </section>
  );
}
