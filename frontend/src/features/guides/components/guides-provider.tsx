"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { usePathname, useRouter } from "next/navigation";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { useOptionalAssistant } from "@/features/assistant/components/assistant-provider";
import { useOptionalTenant } from "@/features/tenant";
import { useLocale } from "@/providers/locale-provider";

import {
  guidesService,
  type Guide,
  type GuideProgress,
} from "../services/guides.service";
import { ProductFeedbackDialog } from "./product-feedback-dialog";
import { TourOverlay } from "./tour-overlay";
import { canAutoStart } from "../lib/auto-start";

type Active = {
  /** Guides played back to back (the set-up tour is a queue). */
  queue: Guide[];
  guide: number;
  step: number;
  /** Page tours end when the user leaves the page; set-up tours do not. */
  setup: boolean;
  manual: boolean;
};

type GuidesContextValue = {
  /** Guides of the current page (page guide first). */
  pageGuides: Guide[];
  pageGuide: Guide | null;
  setup: { items: Guide[]; completed: number; total: number } | null;
  active: boolean;
  startTour: (guide: Guide) => void;
  startSetupTour: () => void;
  resetTours: () => Promise<void>;
  openFeedback: () => void;
  autoStart: boolean;
  setAutoStart: (on: boolean) => void;
};

const GuidesContext = createContext<GuidesContextValue | null>(null);

export const guidesKeys = {
  all: ["guides"] as const,
  route: (slug: string, route: string, locale: string) =>
    ["guides", slug, "route", route, locale] as const,
  setup: (slug: string, locale: string) =>
    ["guides", slug, "setup", locale] as const,
};

/**
 * Module guides of the workspace shell (CF-141): loads the guides of the
 * current route and the set-up tour, starts the page tour on the first
 * visit, the set-up tour on the home page of a fresh organization (after
 * the CF-140 onboarding), and stores progress per user.
 */
export function GuidesProvider({ children }: { children: ReactNode }) {
  const tenant = useOptionalTenant();
  const slug = tenant?.slug ?? "";
  const pathname = usePathname() || "/";
  const router = useRouter();
  const { locale, t } = useLocale();
  const assistant = useOptionalAssistant();
  const qc = useQueryClient();
  const [active, setActive] = useState<Active | null>(null);
  const [feedbackOpen, setFeedbackOpen] = useState(false);
  const preferences = useQuery({
    queryKey: ["guides", slug, "preferences"],
    queryFn: () => guidesService.preferences(slug),
    enabled: Boolean(slug),
    staleTime: 0,
  });
  const autoStart = preferences.data?.auto_start === true;
  const autoTried = useRef<string>("");
  const navigating = useRef<string | null>(null);
  const writes = useRef<Promise<unknown>>(Promise.resolve());

  const routeQuery = useQuery({
    queryKey: guidesKeys.route(slug, pathname, locale),
    queryFn: () => guidesService.forRoute(slug, pathname, locale),
    enabled: Boolean(slug),
    staleTime: 5 * 60_000,
  });
  const setupQuery = useQuery({
    queryKey: guidesKeys.setup(slug, locale),
    queryFn: () => guidesService.setup(slug, locale),
    enabled: Boolean(slug),
    staleTime: 5 * 60_000,
  });

  const pageGuides = useMemo(() => {
    const items = routeQuery.data?.items ?? [];
    return [...items].sort(
      (a, b) =>
        Number(a.kind !== "page") - Number(b.kind !== "page") ||
        a.order - b.order,
    );
  }, [routeQuery.data]);
  const pageGuide = pageGuides.find((g) => g.kind === "page") ?? null;

  const save = useCallback(
    (key: string, progress: GuideProgress) => {
      if (!slug) return;
      writes.current = writes.current
        .catch(() => undefined)
        .then(() => guidesService.progress(slug, key, progress))
        .catch(() => undefined)
        .finally(() => qc.invalidateQueries({ queryKey: guidesKeys.all }));
    },
    [qc, slug],
  );

  const startTour = useCallback(
    (guide: Guide, automatic = false) => {
      if (!guide.steps.length) return;
      setActive({
        queue: [guide],
        guide: 0,
        step: 0,
        setup: false,
        manual: !automatic,
      });
      save(guide.key, {
        status: "in_progress",
        step_index: 0,
        restart: !automatic,
      });
    },
    [save],
  );

  const startSetupTour = useCallback(
    (automatic = false) => {
      const items = (setupQuery.data?.items ?? []).filter(
        (g) => g.steps.length,
      );
      const pending = items.filter(
        (g) =>
          g.progress.status !== "completed" && g.progress.status !== "skipped",
      );
      const queue = pending.length ? pending : items;
      if (!queue.length) return;
      setActive({
        queue,
        guide: 0,
        step: 0,
        setup: true,
        manual: automatic !== true,
      });
      save(queue[0].key, {
        status: "in_progress",
        step_index: 0,
        restart: automatic !== true,
      });
    },
    [save, setupQuery.data],
  );

  // Auto-start: the set-up tour on the home page of an organization whose
  // tour nobody started yet; otherwise the page tour on the first visit.
  useEffect(() => {
    if (!slug || active || !autoStart || autoTried.current === pathname) {
      return;
    }
    if (!routeQuery.data || !setupQuery.data) return;

    const setup = setupQuery.data;
    const freshSetup =
      setup.auto_start !== false &&
      setup.total > 0 &&
      setup.items.every((g) => g.progress.status === "new");
    let cancelled = false;
    const timer = window.setTimeout(() => {
      autoTried.current = pathname;
      const guide =
        pathname === "/" && freshSetup
          ? setup.items.find((g) => g.steps.length)
          : pageGuide;
      if (
        !guide ||
        !canAutoStart(guide.progress.status, autoStart, guide.steps.length)
      )
        return;
      void guidesService
        .claim(slug, guide.key)
        .then(({ started }) => {
          if (!started || cancelled) return;
          if (pathname === "/" && freshSetup) startSetupTour(true);
          else startTour(guide, true);
        })
        .catch(() => undefined);
    }, 700);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [
    active,
    autoStart,
    pageGuide,
    pathname,
    routeQuery.data,
    setupQuery.data,
    slug,
    startSetupTour,
    startTour,
  ]);

  // Leaving the page ends a page tour (the set-up tour walks across pages).
  const lastPath = useRef(pathname);
  useEffect(() => {
    if (lastPath.current === pathname) return;
    lastPath.current = pathname;
    if (navigating.current === pathname) {
      navigating.current = null;
      return;
    }
    navigating.current = null;
    if (active) {
      const guide = active.queue[active.guide];
      save(guide.key, {
        status: "skipped",
        step_index: active.step,
        dismiss_setup: active.setup,
      });
      queueMicrotask(() => setActive(null));
    }
  }, [pathname, active, save]);

  const current = active ? active.queue[active.guide] : null;
  const step = current?.steps[active?.step ?? 0];

  const next = useCallback(() => {
    if (!active || !current) return;
    if (active.step + 1 < current.steps.length) {
      const s = active.step + 1;
      setActive({ ...active, step: s });
      save(current.key, { status: "in_progress", step_index: s });
      return;
    }
    save(current.key, {
      status: "completed",
      step_index: current.steps.length - 1,
    });
    if (active.guide + 1 < active.queue.length) {
      const g = active.guide + 1;
      setActive({ ...active, guide: g, step: 0 });
      save(active.queue[g].key, {
        status: "in_progress",
        step_index: 0,
        restart: active.manual,
      });
    } else {
      setActive(null);
    }
  }, [active, current, save]);

  const back = useCallback(() => {
    if (!active || active.step === 0) return;
    setActive({ ...active, step: active.step - 1 });
  }, [active]);

  const skip = useCallback(() => {
    if (current && active) {
      save(current.key, {
        status: "skipped",
        step_index: active.step,
        dismiss_setup: active.setup,
      });
    }
    setActive(null);
  }, [active, current, save]);

  const ask = useCallback(() => {
    const guide = current ?? pageGuide;
    skip();
    assistant?.ask(
      step ? t("guides.ask_about", { title: step.title }) : undefined,
      guide?.key,
    );
  }, [assistant, current, pageGuide, step, t, skip]);

  const resetTours = useCallback(async () => {
    if (!slug) return;
    await guidesService.reset(slug);
    autoTried.current = "";
    await qc.invalidateQueries({ queryKey: guidesKeys.all });
  }, [qc, slug]);

  const setAutoStart = useCallback(
    (on: boolean) => {
      void guidesService
        .setPreferences(slug, on)
        .then((data) => {
          qc.setQueryData(["guides", slug, "preferences"], data);
        })
        .catch(() => undefined);
    },
    [qc, slug],
  );

  // Persist a dismissal when the shell unmounts or the tab closes.
  const activeRef = useRef(active);
  useEffect(() => {
    activeRef.current = active;
  }, [active]);
  useEffect(() => {
    const dismiss = () => {
      const cur = activeRef.current;
      if (!cur) return;
      const guide = cur.queue[cur.guide];
      void fetch(
        `/api/v1/tenant/guides/${encodeURIComponent(guide.key)}/progress`,
        {
          method: "PUT",
          keepalive: true,
          headers: {
            "Content-Type": "application/json",
            "X-Organization": slug,
          },
          body: JSON.stringify({
            status: "skipped",
            step_index: cur.step,
            dismiss_setup: cur.setup,
          }),
        },
      ).catch(() => undefined);
    };
    window.addEventListener("pagehide", dismiss);
    return () => {
      window.removeEventListener("pagehide", dismiss);
      dismiss();
    };
  }, [slug]);

  const value = useMemo<GuidesContextValue>(
    () => ({
      pageGuides,
      pageGuide,
      setup: setupQuery.data ?? null,
      active: Boolean(active),
      startTour,
      startSetupTour,
      resetTours,
      openFeedback: () => setFeedbackOpen(true),
      autoStart,
      setAutoStart,
    }),
    [
      active,
      autoStart,
      pageGuide,
      pageGuides,
      resetTours,
      setAutoStart,
      setupQuery.data,
      startSetupTour,
      startTour,
    ],
  );

  return (
    <GuidesContext.Provider value={value}>
      {children}
      {current && step && active ? (
        <TourOverlay
          key={`${current.key}:${active.step}`}
          guideTitle={current.title}
          step={step}
          index={active.step}
          total={current.steps.length}
          onNext={next}
          onBack={back}
          onSkip={skip}
          onAsk={ask}
          onNavigate={(route) => {
            navigating.current = route;
            router.push(route);
            next();
          }}
        />
      ) : null}
      {slug ? (
        <ProductFeedbackDialog
          open={feedbackOpen}
          onOpenChange={setFeedbackOpen}
          slug={slug}
        />
      ) : null}
    </GuidesContext.Provider>
  );
}

export function useGuides(): GuidesContextValue {
  const ctx = useContext(GuidesContext);
  if (!ctx) throw new Error("useGuides must be used within GuidesProvider");
  return ctx;
}

/** Guides context, or null outside the workspace shell. */
export function useOptionalGuides(): GuidesContextValue | null {
  return useContext(GuidesContext);
}
