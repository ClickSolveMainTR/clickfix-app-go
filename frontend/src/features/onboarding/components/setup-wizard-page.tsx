"use client";

import {
  ArrowLeft,
  Check,
  Loader2,
  PartyPopper,
  RefreshCw,
} from "lucide-react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { motion } from "motion/react";

import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { BRAND_EASE } from "@/lib/motion";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useOnboarding, useOnboardingActions } from "../hooks/use-onboarding";
import { stepIcon } from "../lib/steps";
import type {
  OnboardingState,
  OnboardingStepKey,
} from "../services/onboarding.service";
import { ChannelStep } from "./channel-step";
import { CreateOrganizationForm } from "./create-organization-form";
import { ProgressRing } from "./progress-ring";
import { SmartSetupBanner } from "./smart-setup/smart-setup-banner";

function StepPanel({
  stepKey,
  state,
  slug,
  organizationName,
}: {
  stepKey: OnboardingStepKey;
  state: OnboardingState;
  slug: string;
  organizationName: string;
}) {
  const { t } = useLocale();
  const { completeStep, refresh } = useOnboardingActions(slug);
  const step = state.steps.find((s) => s.key === stepKey);
  const done = Boolean(step?.done);

  const confirm = (key: "team" | "widget", labelKey: string) =>
    done ? (
      <p className="bg-signal-soft text-signal-text flex w-fit items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium">
        <Check className="size-4" aria-hidden />
        {t("onboarding.wizard.step_done")}
      </p>
    ) : (
      <Button
        variant="outline"
        className="w-fit"
        disabled={completeStep.isPending}
        onClick={() => completeStep.mutate(key)}
      >
        {completeStep.isPending ? <Loader2 className="animate-spin" /> : null}
        {t(labelKey)}
      </Button>
    );

  switch (stepKey) {
    case "organization":
      return (
        <p className="bg-signal-soft text-signal-text flex w-fit items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium">
          <Check className="size-4" aria-hidden />
          {t("onboarding.wizard.organization_ready", {
            name: organizationName,
          })}
        </p>
      );
    case "channel":
      return <ChannelStep slug={slug} done={done} />;
    case "team":
      return (
        <div className="flex flex-col gap-4">
          <ul className="text-muted-foreground list-inside list-disc space-y-1 text-sm">
            <li>{t("onboarding.wizard.team_point_roles")}</li>
            <li>{t("onboarding.wizard.team_point_invite")}</li>
            <li>{t("onboarding.wizard.team_point_auto")}</li>
          </ul>
          <div className="flex flex-wrap gap-2">
            <Button asChild className="w-fit">
              <Link href={`${routes.cms.settings.members}?tab=invitations`}>
                {t("onboarding.wizard.team_invite")}
              </Link>
            </Button>
            {confirm("team", "onboarding.wizard.team_solo")}
          </div>
        </div>
      );
    case "widget":
      return (
        <div className="flex flex-col gap-4">
          <ol className="text-muted-foreground list-inside list-decimal space-y-1 text-sm">
            <li>{t("onboarding.wizard.widget_point_channel")}</li>
            <li>{t("onboarding.wizard.widget_point_snippet")}</li>
            <li>{t("onboarding.wizard.widget_point_test")}</li>
          </ol>
          <div className="flex flex-wrap gap-2">
            <Button asChild className="w-fit">
              <Link href="/widget">{t("onboarding.wizard.widget_open")}</Link>
            </Button>
            {confirm("widget", "onboarding.wizard.widget_confirm")}
          </div>
        </div>
      );
    case "first_conversation":
      return (
        <div className="flex flex-col gap-4">
          <p className="text-muted-foreground text-sm">
            {t("onboarding.wizard.first_conversation_hint")}
          </p>
          {done ? (
            <p className="bg-signal-soft text-signal-text flex w-fit items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium">
              <Check className="size-4" aria-hidden />
              {t("onboarding.wizard.first_conversation_done")}
            </p>
          ) : (
            <Button variant="outline" className="w-fit" onClick={refresh}>
              <RefreshCw />
              {t("onboarding.wizard.check_again")}
            </Button>
          )}
        </div>
      );
  }
}

/**
 * Guided set-up (`/setup`): open an organization when the account has
 * none, then the checklist steps with the selected step's panel. The step
 * comes from `?adim=` so the checklist card can deep-link.
 */
export function SetupWizardPage() {
  const { t } = useLocale();
  const router = useRouter();
  const params = useSearchParams();
  const wantsNew = params.get("yeni") === "1";
  const { slug, organization, data, isLoading, isError } = useOnboarding();
  const { finish } = useOnboardingActions(slug);

  const header = (
    <PageHeader
      eyebrow={organization?.name ?? t("onboarding.wizard.eyebrow")}
      title={t("onboarding.wizard.title")}
      description={t("onboarding.wizard.description")}
      actions={
        <Button variant="ghost" asChild>
          <Link href={routes.cms.home}>
            <ArrowLeft />
            {t("onboarding.wizard.back_home")}
          </Link>
        </Button>
      }
    />
  );

  if (!organization || wantsNew) {
    return (
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
        {header}
        <section className="bg-card rounded-xl border p-6 shadow-sm">
          <h2 className="font-display mb-1 text-lg font-semibold tracking-tight">
            {t("onboarding.create.title")}
          </h2>
          <p className="text-muted-foreground mb-5 text-sm">
            {t("onboarding.create.body")}
          </p>
          <CreateOrganizationForm
            onCreated={() => router.replace(routes.cms.smartSetup)}
          />
        </section>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
        {header}
        <Skeleton className="h-80 w-full rounded-xl" />
      </div>
    );
  }

  if (isError || !data || data.steps.length === 0 || !data.active) {
    return (
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
        {header}
        <EmptyState
          title={
            data && !data.active && data.finished_at
              ? t("onboarding.wizard.finished_title")
              : t("onboarding.wizard.none_title")
          }
          description={
            data && !data.active && data.finished_at
              ? t("onboarding.wizard.finished_body")
              : t("onboarding.wizard.none_body")
          }
          action={
            <Button asChild variant="outline">
              <Link href={routes.cms.home}>
                {t("onboarding.wizard.back_home")}
              </Link>
            </Button>
          }
        />
      </div>
    );
  }

  const requested = params.get("adim") as OnboardingStepKey | null;
  const current =
    data.steps.find((s) => s.key === requested) ??
    data.steps.find((s) => !s.done) ??
    data.steps[data.steps.length - 1];
  const allDone = data.completed === data.total;
  const select = (key: OnboardingStepKey) =>
    router.replace(`${routes.cms.onboarding}?adim=${key}`, { scroll: false });

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      {header}
      <SmartSetupBanner />
      <div className="grid gap-6 lg:grid-cols-[18rem_minmax(0,1fr)]">
        <nav
          aria-label={t("onboarding.wizard.steps_label")}
          className="bg-card flex flex-col gap-4 rounded-xl border p-4 shadow-sm"
        >
          <div className="flex items-center gap-3 px-1">
            <ProgressRing
              value={data.completed}
              total={data.total}
              className="size-12"
            />
            <p className="text-muted-foreground text-sm">
              {t("onboarding.wizard.progress", {
                done: data.completed,
                total: data.total,
              })}
            </p>
          </div>
          <ol className="flex flex-col gap-1">
            {data.steps.map((step, i) => {
              const Icon = stepIcon(step.key);
              const isCurrent = step.key === current.key;
              return (
                <li key={step.key}>
                  <button
                    type="button"
                    onClick={() => select(step.key)}
                    aria-current={isCurrent ? "step" : undefined}
                    className={cn(
                      "focus-visible:ring-ring/50 flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left text-sm transition-colors outline-none focus-visible:ring-[3px]",
                      isCurrent ? "bg-accent font-medium" : "hover:bg-muted/70",
                    )}
                  >
                    <span
                      className={cn(
                        "font-accent flex size-7 shrink-0 items-center justify-center rounded-full text-xs",
                        step.done
                          ? "bg-signal text-signal-foreground"
                          : isCurrent
                            ? "bg-primary text-primary-foreground"
                            : "bg-muted text-muted-foreground",
                      )}
                    >
                      {step.done ? (
                        <Check className="size-3.5" aria-hidden />
                      ) : (
                        i + 1
                      )}
                    </span>
                    <span className="min-w-0 flex-1 truncate">
                      {t(`onboarding.steps.${step.key}_title`)}
                    </span>
                    <Icon
                      className="text-muted-foreground size-4 shrink-0"
                      aria-hidden
                    />
                  </button>
                </li>
              );
            })}
          </ol>
          {allDone ? (
            <Button
              onClick={() =>
                finish.mutate(undefined, {
                  onSuccess: () => router.push(routes.cms.home),
                })
              }
              disabled={finish.isPending}
            >
              <PartyPopper />
              {t("onboarding.wizard.finish")}
            </Button>
          ) : null}
        </nav>

        <motion.section
          key={current.key}
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.35, ease: BRAND_EASE }}
          className="bg-card flex flex-col gap-5 rounded-xl border p-5 shadow-sm sm:p-7"
          aria-labelledby="setup-step-title"
        >
          <div className="space-y-1.5">
            <p className="font-accent text-muted-foreground text-[11px] font-semibold tracking-[0.16em] uppercase">
              {t("onboarding.wizard.step_of", {
                n: data.steps.findIndex((s) => s.key === current.key) + 1,
                total: data.total,
              })}
            </p>
            <h2
              id="setup-step-title"
              className="font-display text-xl font-semibold tracking-tight"
            >
              {t(`onboarding.steps.${current.key}_title`)}
            </h2>
            <p className="text-muted-foreground text-sm text-pretty">
              {t(`onboarding.steps.${current.key}_body`)}
            </p>
          </div>
          <StepPanel
            stepKey={current.key}
            state={data}
            slug={slug}
            organizationName={organization.name}
          />
        </motion.section>
      </div>
    </div>
  );
}
