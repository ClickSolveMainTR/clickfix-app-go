"use client";

import { ArrowRight, Sparkles, Undo2 } from "lucide-react";
import { motion } from "motion/react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { BRAND_EASE, usePrefersReducedMotion } from "@/lib/motion";
import { useLocale } from "@/providers/locale-provider";

import { useOnboarding } from "../../hooks/use-onboarding";
import { isApplying, useLatestSetupRun } from "../../hooks/use-setup";

/**
 * Entry to the smart set-up from the CF-13 checklist: an invitation while
 * nothing was applied yet, the live run while it applies, and the review /
 * undo link for 24 hours after.
 */
export function SmartSetupBanner() {
  const { t } = useLocale();
  const reduced = usePrefersReducedMotion();
  const { slug, organization } = useOnboarding();
  const canManage = Boolean(
    organization?.permissions.includes("tenant.onboarding.manage") ||
    organization?.role.key === "owner",
  );
  const { data: run, isLoading } = useLatestSetupRun(slug, canManage);
  if (!canManage || isLoading) return null;

  const applied =
    run &&
    run.status !== "draft" &&
    run.status !== "undone" &&
    !isApplying(run);
  if (applied && !run.can_undo) return null;

  const href = run
    ? `${routes.cms.smartSetup}?calisma=${run.uuid}`
    : routes.cms.smartSetup;
  const state =
    !run || run.status === "undone"
      ? "start"
      : isApplying(run)
        ? "applying"
        : run.status === "draft"
          ? "draft"
          : "applied";

  return (
    <motion.section
      initial={reduced ? false : { opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4, ease: BRAND_EASE }}
      className="relative flex flex-col gap-4 overflow-hidden rounded-xl border bg-[#0a0d0b] p-5 text-white shadow-sm sm:flex-row sm:items-center sm:p-6"
    >
      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(90%_120%_at_100%_0%,rgba(8,115,19,0.55),transparent_60%)]"
      />
      <span className="relative grid size-11 shrink-0 place-items-center rounded-xl bg-white/10">
        {state === "applied" ? (
          <Undo2 className="size-5" aria-hidden />
        ) : (
          <Sparkles className="size-5" aria-hidden />
        )}
      </span>
      <div className="relative min-w-0 flex-1 space-y-1">
        <h2 className="font-display text-lg font-semibold tracking-tight">
          {t(`onboarding.setup.banner.${state}_title`)}
        </h2>
        <p className="text-sm text-white/70">
          {t(`onboarding.setup.banner.${state}_body`)}
        </p>
      </div>
      <Button asChild variant="secondary" className="relative w-fit">
        <Link href={href}>
          {t(`onboarding.setup.banner.${state}_cta`)}
          <ArrowRight />
        </Link>
      </Button>
    </motion.section>
  );
}
