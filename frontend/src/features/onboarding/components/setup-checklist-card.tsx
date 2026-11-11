"use client";

import { AlertTriangle, ArrowRight, Check, X } from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useOnboarding, useOnboardingActions } from "../hooks/use-onboarding";
import { stepIcon } from "../lib/steps";
import { ProgressRing } from "./progress-ring";

/**
 * Persistent set-up checklist on the workspace home. Shows while the
 * organization's checklist is active; ticks itself as channels, members and
 * conversations appear; "Hide" closes it for good.
 */
export function SetupChecklistCard() {
  const { t } = useLocale();
  const { slug, data, isLoading } = useOnboarding();
  const { finish } = useOnboardingActions(slug);

  if (isLoading) {
    return <Skeleton className="h-52 w-full rounded-xl" />;
  }
  if (!data?.active) return null;

  // The next step is the first that is neither done nor skipped; skipped
  // steps stay as warnings and count towards finishing (CF-198).
  const next = data.steps.find((s) => !s.done && !s.skipped);
  const allDone = !next;

  return (
    <section
      aria-labelledby="setup-checklist-title"
      className="bg-card relative overflow-hidden rounded-xl border p-5 shadow-sm sm:p-6"
    >
      <div
        aria-hidden
        className="bg-signal/10 pointer-events-none absolute -top-24 -right-24 size-64 rounded-full blur-3xl"
      />
      <div className="relative flex flex-wrap items-start gap-4">
        <ProgressRing value={data.completed} total={data.total} />
        <div className="min-w-0 flex-1 space-y-1">
          <h2
            id="setup-checklist-title"
            className="font-display text-lg font-semibold tracking-tight"
          >
            {allDone
              ? t("onboarding.card.title_done")
              : t("onboarding.card.title")}
          </h2>
          <p className="text-muted-foreground text-sm">
            {allDone
              ? t("onboarding.card.body_done")
              : t("onboarding.card.body")}
          </p>
        </div>
        <Button
          variant="ghost"
          size="icon"
          className="text-muted-foreground -mt-1 -mr-2 rounded-lg"
          aria-label={t("onboarding.card.hide")}
          disabled={finish.isPending}
          onClick={() => finish.mutate()}
        >
          <X className="size-4" />
        </Button>
      </div>

      <ol className="relative mt-5 grid gap-2 sm:grid-cols-2 lg:grid-cols-5">
        {data.steps.map((step) => {
          const Icon = stepIcon(step.key);
          return (
            <li
              key={step.key}
              className={cn(
                "flex items-center gap-2.5 rounded-lg border px-3 py-2.5 text-sm",
                step.done
                  ? "bg-muted/60 text-muted-foreground border-transparent"
                  : step.skipped
                    ? "border-warning/40 bg-warning/10"
                    : step.key === next?.key
                      ? "border-signal-text/40 bg-signal-soft/50"
                      : "bg-background/60",
              )}
            >
              <span
                className={cn(
                  "flex size-6 shrink-0 items-center justify-center rounded-full",
                  step.done
                    ? "bg-signal text-signal-foreground"
                    : "bg-muted text-foreground/70",
                )}
              >
                {step.done ? (
                  <Check className="size-3.5" aria-hidden />
                ) : step.skipped ? (
                  <AlertTriangle className="size-3.5" aria-hidden />
                ) : (
                  <Icon className="size-3.5" aria-hidden />
                )}
              </span>
              <span
                className={cn("min-w-0 truncate", step.done && "line-through")}
              >
                {t(`onboarding.steps.${step.key}_title`)}
              </span>
              <span className="sr-only">
                {step.done
                  ? t("onboarding.card.step_done")
                  : step.skipped
                    ? t("onboarding.card.step_skipped")
                    : t("onboarding.card.step_todo")}
              </span>
            </li>
          );
        })}
      </ol>

      <div className="relative mt-5 flex flex-wrap items-center gap-2">
        {allDone ? (
          <Button onClick={() => finish.mutate()} disabled={finish.isPending}>
            {t("onboarding.card.finish")}
            <Check />
          </Button>
        ) : (
          <Button asChild>
            <Link href={`${routes.cms.onboarding}?adim=${next.key}`}>
              {t("onboarding.card.continue")}
              <ArrowRight />
            </Link>
          </Button>
        )}
      </div>
    </section>
  );
}
