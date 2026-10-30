"use client";

import {
  ArrowRight,
  Compass,
  LayoutGrid,
  ListChecks,
  Lock,
  Rocket,
  Sparkles,
  UsersRound,
  WandSparkles,
} from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { isApiError } from "@/lib/api";
import { BRAND_EASE, usePrefersReducedMotion } from "@/lib/motion";
import { useLocale } from "@/providers/locale-provider";

import { useOnboarding } from "../../hooks/use-onboarding";
import {
  useLatestSetupRun,
  useSetupActions,
  useSetupProgress,
  useSetupRun,
  useSetupTemplates,
} from "../../hooks/use-setup";
import type { SetupRun, SetupTemplate } from "../../services/setup.service";
import { BuildScreen } from "./build-screen";
import { DiscoveryChat, type DiscoveryResult } from "./discovery-chat";
import { PlanReview } from "./plan-review";
import { SetupShell, type ShellStep } from "./setup-shell";
import { TeamStep } from "./team-step";
import { TemplateGallery } from "./template-gallery";

type Stage = "source" | "discover" | "gallery" | "review" | "build" | "team";

const STEPS: ShellStep[] = [
  { key: "source", icon: Compass },
  { key: "discover", icon: WandSparkles },
  { key: "review", icon: ListChecks },
  { key: "build", icon: Rocket },
  { key: "team", icon: UsersRound },
];

const STEP_INDEX: Record<Stage, number> = {
  source: 0,
  discover: 1,
  gallery: 1,
  review: 2,
  build: 3,
  team: 4,
};

const ERRORS: Record<string, string> = {
  SETUP_DISCOVER_LIMIT: "onboarding.setup.errors.discover_limit",
  SETUP_RUN_ACTIVE: "onboarding.setup.errors.active",
  SETUP_RUN_NOT_DRAFT: "onboarding.setup.errors.not_draft",
  SETUP_WEBSITE_INVALID: "onboarding.setup.errors.website",
  VALIDATION_ERROR: "onboarding.setup.errors.website",
};

function stageFor(run: SetupRun): Stage {
  return run.status === "draft" ? "review" : "build";
}

function SourceStep({
  onPick,
}: {
  onPick: (s: "discover" | "gallery") => void;
}) {
  const { t } = useLocale();
  const reduced = usePrefersReducedMotion();
  const options = [
    { key: "discover" as const, icon: Sparkles, accent: true },
    { key: "gallery" as const, icon: LayoutGrid, accent: false },
  ];
  return (
    <div className="flex flex-col gap-6">
      <header className="space-y-2">
        <h1 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
          {t("onboarding.setup.source.title")}
        </h1>
        <p className="text-muted-foreground text-sm leading-relaxed">
          {t("onboarding.setup.source.body")}
        </p>
      </header>
      <div className="grid gap-3 sm:grid-cols-2">
        {options.map((o, i) => {
          const Icon = o.icon;
          return (
            <motion.button
              key={o.key}
              type="button"
              initial={reduced ? false : { opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.4, ease: BRAND_EASE, delay: i * 0.08 }}
              onClick={() => onPick(o.key)}
              className="group bg-card focus-visible:ring-ring/50 relative flex flex-col gap-3 overflow-hidden rounded-2xl border p-6 text-left shadow-sm transition-shadow outline-none hover:shadow-md focus-visible:ring-[3px]"
            >
              {o.accent ? (
                <span
                  aria-hidden
                  className="absolute -top-16 -right-16 size-40 rounded-full opacity-20 blur-2xl"
                  style={{ background: "var(--setup-accent)" }}
                />
              ) : null}
              <span
                className="grid size-11 place-items-center rounded-xl text-white"
                style={{
                  background: o.accent ? "var(--setup-accent)" : "#3f3f46",
                }}
              >
                <Icon className="size-5" aria-hidden />
              </span>
              <span className="font-display text-lg font-semibold">
                {t(`onboarding.setup.source.${o.key}_title`)}
              </span>
              <span className="text-muted-foreground text-sm">
                {t(`onboarding.setup.source.${o.key}_body`)}
              </span>
              <span className="mt-auto flex items-center gap-1 text-sm font-medium">
                {t(`onboarding.setup.source.${o.key}_cta`)}
                <ArrowRight
                  className="size-4 transition-transform group-hover:translate-x-0.5"
                  aria-hidden
                />
              </span>
            </motion.button>
          );
        })}
      </div>
    </div>
  );
}

/**
 * `/getting-started` — the AI-assisted set-up (CF-140). Pick AI discovery or a
 * template, review the plan, watch the workspace being built live, then
 * invite the team. Extends the CF-13 checklist: the run lives next to it
 * and `/setup` links here. Ported flow: otopoly-go onboarding wizard
 * (step transitions with direction, task progress panel).
 */
export function SmartSetupPage() {
  const { t, locale } = useLocale();
  const router = useRouter();
  const params = useSearchParams();
  const reduced = usePrefersReducedMotion();
  const { slug, organization } = useOnboarding();
  const canManage = Boolean(
    organization?.permissions.includes("tenant.onboarding.manage") ||
    organization?.role.key === "owner",
  );
  // null: not chosen yet — resume the newest run (a draft to review, an
  // apply in progress or one that can still be undone) or start fresh.
  const [chosen, setChosen] = useState<Stage | null>(null);
  const [direction, setDirection] = useState(1);
  const [runUuid, setRunUuid] = useState<string | null>(params.get("calisma"));
  const [error, setError] = useState<string | null>(null);
  const [pendingTemplate, setPendingTemplate] = useState<string | null>(null);

  const latest = useLatestSetupRun(slug, canManage && !runUuid);
  const candidate = runUuid ? null : (latest.data ?? null);
  const resumable =
    candidate &&
    (candidate.status === "draft" ||
      candidate.status === "queued" ||
      candidate.status === "running" ||
      candidate.can_undo)
      ? candidate
      : null;
  const activeUuid = runUuid ?? (chosen ? null : (resumable?.uuid ?? null));
  const runQuery = useSetupRun(slug, activeUuid);
  const run = runQuery.data ?? null;
  const stage: Stage =
    chosen ??
    (runUuid && run
      ? stageFor(run)
      : resumable
        ? stageFor(resumable)
        : "source");
  const resolving =
    !chosen && (runUuid ? runQuery.isLoading : latest.isLoading);
  const templates = useSetupTemplates(slug, canManage && stage === "gallery");
  const { fromTemplate, discover, apply, undo } = useSetupActions(slug);
  useSetupProgress(slug, activeUuid);

  const go = (next: Stage) => {
    setDirection(STEP_INDEX[next] >= STEP_INDEX[stage] ? 1 : -1);
    setError(null);
    if (activeUuid && !runUuid) setRunUuid(activeUuid);
    setChosen(next);
  };

  const open = (r: SetupRun) => {
    setRunUuid(r.uuid);
    setDirection(1);
    setError(null);
    setChosen(stageFor(r));
    router.replace(`${routes.cms.smartSetup}?calisma=${r.uuid}`, {
      scroll: false,
    });
  };

  const fail = (err: unknown) =>
    setError(
      t(
        (isApiError(err) && ERRORS[err.code ?? ""]) ||
          "onboarding.setup.errors.generic",
      ),
    );

  const onDiscover = ({ website, answers }: DiscoveryResult) => {
    discover.mutate(
      { website, answers, locale: locale === "en" ? "en" : "tr" },
      { onSuccess: open, onError: fail },
    );
  };

  const onTemplate = (tpl: SetupTemplate) => {
    setPendingTemplate(tpl.key);
    fromTemplate.mutate(tpl.key, {
      onSuccess: open,
      onError: fail,
      onSettled: () => setPendingTemplate(null),
    });
  };

  const onApply = (selection: string[]) => {
    if (!run) return;
    apply.mutate(
      { uuid: run.uuid, selection },
      { onSuccess: () => go("build"), onError: fail },
    );
  };

  const brand = run?.plan.branding.primary_color ?? null;

  if (!organization || (canManage && resolving)) {
    return (
      <SetupShell steps={STEPS} current={0}>
        <Skeleton className="h-80 w-full rounded-2xl" />
      </SetupShell>
    );
  }

  if (!canManage) {
    return (
      <SetupShell steps={STEPS} current={0}>
        <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center">
          <Lock className="text-muted-foreground size-8" aria-hidden />
          <h1 className="font-display text-xl font-semibold">
            {t("onboarding.setup.forbidden_title")}
          </h1>
          <p className="text-muted-foreground max-w-sm text-sm">
            {t("onboarding.setup.forbidden_body")}
          </p>
          <Button asChild variant="outline">
            <Link href={routes.cms.home}>
              {t("onboarding.wizard.back_home")}
            </Link>
          </Button>
        </div>
      </SetupShell>
    );
  }

  return (
    <SetupShell steps={STEPS} current={STEP_INDEX[stage]} brand={brand}>
      {error ? (
        <p
          role="alert"
          className="bg-destructive/10 text-destructive mb-4 rounded-lg px-3 py-2 text-sm"
        >
          {error}
        </p>
      ) : null}
      <AnimatePresence mode="wait" custom={direction} initial={false}>
        <motion.div
          key={stage}
          custom={direction}
          initial={reduced ? false : { opacity: 0, x: direction * 28 }}
          animate={{ opacity: 1, x: 0 }}
          exit={reduced ? undefined : { opacity: 0, x: direction * -28 }}
          transition={{ duration: 0.28, ease: BRAND_EASE }}
          className="flex flex-1 flex-col"
        >
          {stage === "source" ? <SourceStep onPick={(s) => go(s)} /> : null}

          {stage === "discover" ? (
            <div className="flex flex-col gap-6">
              <header className="space-y-2">
                <h1 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
                  {t("onboarding.setup.discover.title")}
                </h1>
                <p className="text-muted-foreground text-sm">
                  {t("onboarding.setup.discover.body")}
                </p>
              </header>
              <DiscoveryChat
                companyName={organization.name}
                busy={discover.isPending}
                onDone={onDiscover}
              />
              {!discover.isPending ? (
                <button
                  type="button"
                  onClick={() => go("gallery")}
                  className="text-muted-foreground w-fit text-sm hover:underline"
                >
                  {t("onboarding.setup.discover.to_gallery")}
                </button>
              ) : null}
            </div>
          ) : null}

          {stage === "gallery" ? (
            <div className="flex flex-col gap-6">
              <header className="space-y-2">
                <h1 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
                  {t("onboarding.setup.gallery.title")}
                </h1>
                <p className="text-muted-foreground text-sm">
                  {t("onboarding.setup.gallery.body")}
                </p>
              </header>
              <TemplateGallery
                templates={templates.data}
                loading={templates.isLoading}
                pendingKey={pendingTemplate}
                onPick={onTemplate}
              />
              <button
                type="button"
                onClick={() => go("source")}
                className="text-muted-foreground w-fit text-sm hover:underline"
              >
                {t("onboarding.setup.back")}
              </button>
            </div>
          ) : null}

          {stage === "review" ? (
            run && run.status === "draft" ? (
              <PlanReview
                key={run.uuid}
                run={run}
                applying={apply.isPending}
                onBack={() =>
                  go(run.source === "template" ? "gallery" : "discover")
                }
                onApply={onApply}
              />
            ) : (
              <Skeleton className="h-96 w-full rounded-2xl" />
            )
          ) : null}

          {stage === "build" ? (
            run ? (
              <BuildScreen
                run={run}
                undoing={undo.isPending}
                onUndo={() => {
                  // Pin the run so its "undone" summary stays on screen.
                  setRunUuid(run.uuid);
                  setChosen("build");
                  undo.mutate(run.uuid, { onError: fail });
                }}
                onContinue={() => {
                  if (run.status === "undone") {
                    setRunUuid(null);
                    setDirection(-1);
                    setChosen("source");
                    router.replace(routes.cms.smartSetup, { scroll: false });
                    return;
                  }
                  go("team");
                }}
              />
            ) : (
              <Skeleton className="h-96 w-full rounded-2xl" />
            )
          ) : null}

          {stage === "team" ? (
            <TeamStep
              slug={slug}
              run={run}
              onFinish={() => router.push(routes.cms.inbox)}
            />
          ) : null}
        </motion.div>
      </AnimatePresence>
    </SetupShell>
  );
}
