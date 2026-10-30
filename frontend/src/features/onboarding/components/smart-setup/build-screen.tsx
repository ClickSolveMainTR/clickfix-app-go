"use client";

import {
  AlertTriangle,
  ArrowRight,
  Check,
  Loader2,
  MinusCircle,
  PartyPopper,
  Undo2,
} from "lucide-react";
import { AnimatePresence, motion, useSpring, useTransform } from "motion/react";
import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { BRAND_EASE, usePrefersReducedMotion } from "@/lib/motion";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { FINISHED_ITEM, SETUP_KINDS, kindIcon } from "../../lib/setup-kinds";
import type { SetupItem, SetupRun } from "../../services/setup.service";
import { Confetti } from "./confetti";

const REVEAL_MS = 90;

/** Count that catches up with `target` one step per tick. */
function useReveal(target: number, instant: boolean) {
  const [shown, setShown] = useState(0);
  useEffect(() => {
    if (instant) return;
    const id = window.setInterval(() => {
      setShown((n) => (n < target ? n + 1 : n));
    }, REVEAL_MS);
    return () => window.clearInterval(id);
  }, [target, instant]);
  return instant ? target : Math.min(shown, target);
}

/** A number that counts up smoothly (instant with reduced motion). */
function Counter({ value }: { value: number }) {
  const reduced = usePrefersReducedMotion();
  const spring = useSpring(value, { stiffness: 120, damping: 20 });
  const text = useTransform(spring, (v) => Math.round(v).toString());
  useEffect(() => {
    if (reduced) spring.jump(value);
    else spring.set(value);
  }, [spring, value, reduced]);
  return <motion.span className="tabular-nums">{text}</motion.span>;
}

function ItemState({ status }: { status: SetupItem["status"] }) {
  switch (status) {
    case "created":
    case "updated":
      return (
        <motion.span
          initial={{ scale: 0 }}
          animate={{ scale: 1 }}
          transition={{ type: "spring", stiffness: 500, damping: 22 }}
          className="grid size-5 place-items-center rounded-full text-white"
          style={{ background: "var(--setup-accent)" }}
        >
          <Check className="size-3" aria-hidden />
        </motion.span>
      );
    case "failed":
      return <AlertTriangle className="text-destructive size-4" aria-hidden />;
    case "skipped":
    case "unavailable":
      return (
        <MinusCircle className="text-muted-foreground size-4" aria-hidden />
      );
    default:
      return (
        <Loader2
          className="text-muted-foreground size-4 animate-spin"
          aria-hidden
        />
      );
  }
}

/**
 * The live "building your workspace" screen: every ticked item appears as
 * its progress event arrives (staggered cards with a checkmark), counters
 * per kind count up, the brand colour of the plan tints everything, and a
 * short confetti burst closes it. Then: undo (24 h) or invite the team.
 */
export function BuildScreen({
  run,
  onContinue,
  onUndo,
  undoing,
}: {
  run: SetupRun;
  onContinue: () => void;
  onUndo: () => void;
  undoing: boolean;
}) {
  const { t, locale } = useLocale();
  const reduced = usePrefersReducedMotion();
  const applying = run.status === "queued" || run.status === "running";
  const undone = run.status === "undone";
  const brand = run.plan.branding.primary_color || "var(--setup-accent)";

  const items = run.items.filter((i) => i.selected);
  const landed = items.filter((i) => FINISHED_ITEM.has(i.status));
  // The job is faster than the eye: reveal what landed one by one so the
  // workspace visibly assembles (everything at once with reduced motion).
  const revealed = useReveal(landed.length, reduced || run.status === "undone");
  const arrived = landed.slice(0, revealed);
  const working = applying || revealed < landed.length;
  const finished = !working;
  const done = arrived.length;
  const created = arrived.filter(
    (i) => i.status === "created" || i.status === "updated",
  ).length;
  const failed = arrived.filter((i) => i.status === "failed").length;
  const total = Math.max(run.total, items.length);
  const ratio = total ? Math.min(1, done / total) : 0;

  const perKind = useMemo(
    () =>
      SETUP_KINDS.map(({ kind }) => ({
        kind,
        count: arrived.filter(
          (i) =>
            i.kind === kind &&
            (i.status === "created" || i.status === "updated"),
        ).length,
      })).filter((k) => k.count > 0),
    [arrived],
  );

  const confettiColors = useMemo(
    () =>
      [
        run.plan.branding.primary_color,
        "#f59e0b",
        "#10b981",
        "#6366f1",
        "#ec4899",
      ].filter((c): c is string => Boolean(c)),
    [run.plan.branding.primary_color],
  );

  return (
    <div className="relative flex flex-col gap-8" aria-live="polite">
      {finished && !undone && failed < total ? (
        <Confetti colors={confettiColors} />
      ) : null}
      <header className="flex flex-col items-center gap-4 text-center">
        <span
          className="relative grid size-16 place-items-center rounded-2xl text-white shadow-lg"
          style={{ background: brand }}
        >
          {working && !reduced ? (
            <motion.span
              className="absolute inset-0 rounded-2xl"
              style={{ background: brand }}
              animate={{ scale: [1, 1.35], opacity: [0.45, 0] }}
              transition={{ duration: 1.6, repeat: Infinity }}
            />
          ) : null}
          {working ? (
            <Loader2 className="relative size-7 animate-spin" aria-hidden />
          ) : undone ? (
            <Undo2 className="size-7" aria-hidden />
          ) : (
            <PartyPopper className="size-7" aria-hidden />
          )}
        </span>
        <div className="space-y-1.5">
          <h1 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            {working
              ? t("onboarding.setup.build.title_working", {
                  name: run.plan.summary.company_name,
                })
              : undone
                ? t("onboarding.setup.build.title_undone")
                : failed > 0
                  ? t("onboarding.setup.build.title_partial")
                  : t("onboarding.setup.build.title_done")}
          </h1>
          <p className="text-muted-foreground text-sm">
            {t("onboarding.setup.build.progress", {
              done: done.toLocaleString(locale),
              total: total.toLocaleString(locale),
            })}
          </p>
        </div>
        <div
          className="bg-muted h-2 w-full max-w-md overflow-hidden rounded-full"
          role="progressbar"
          aria-valuemin={0}
          aria-valuemax={total}
          aria-valuenow={done}
          aria-label={t("onboarding.setup.build.progress_label")}
        >
          <motion.div
            className="h-full rounded-full"
            style={{ background: brand }}
            initial={false}
            animate={{ width: `${ratio * 100}%` }}
            transition={{ duration: reduced ? 0 : 0.5, ease: BRAND_EASE }}
          />
        </div>
      </header>

      {finished ? (
        <motion.div
          initial={reduced ? false : { opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.4, ease: BRAND_EASE, delay: 0.2 }}
          className="flex flex-col items-center gap-3 text-center"
        >
          {!undone ? (
            <p className="text-muted-foreground text-sm">
              {t("onboarding.setup.build.summary", {
                created: created.toLocaleString(locale),
                skipped: (arrived.length - created - failed).toLocaleString(
                  locale,
                ),
              })}
            </p>
          ) : null}
          <div className="flex flex-wrap justify-center gap-2">
            {run.can_undo ? (
              <Button variant="outline" onClick={onUndo} disabled={undoing}>
                {undoing ? <Loader2 className="animate-spin" /> : <Undo2 />}
                {t("onboarding.setup.build.undo")}
              </Button>
            ) : null}
            <Button
              size="lg"
              onClick={onContinue}
              className="text-white"
              style={{ background: brand }}
            >
              {undone
                ? t("onboarding.setup.build.start_over")
                : t("onboarding.setup.build.continue")}
              <ArrowRight />
            </Button>
          </div>
          {run.can_undo && run.undo_until ? (
            <p className="text-muted-foreground text-xs">
              {t("onboarding.setup.build.undo_hint", {
                time: new Date(run.undo_until).toLocaleString(locale, {
                  dateStyle: "medium",
                  timeStyle: "short",
                }),
              })}
            </p>
          ) : null}
        </motion.div>
      ) : null}

      {perKind.length > 0 && !undone ? (
        <ul className="grid grid-cols-2 gap-2 sm:grid-cols-4">
          {perKind.map(({ kind, count }) => {
            const Icon = kindIcon(kind);
            return (
              <motion.li
                key={kind}
                layout={!reduced}
                initial={reduced ? false : { opacity: 0, scale: 0.9 }}
                animate={{ opacity: 1, scale: 1 }}
                transition={{ duration: 0.3, ease: BRAND_EASE }}
                className="bg-card flex items-center gap-2.5 rounded-xl border px-3 py-2.5"
              >
                <Icon
                  className="size-4 shrink-0"
                  style={{ color: brand }}
                  aria-hidden
                />
                <span className="min-w-0 flex-1 truncate text-xs">
                  {t(`onboarding.setup.kinds.${kind}`)}
                </span>
                <span className="font-display text-lg font-semibold">
                  <Counter value={count} />
                </span>
              </motion.li>
            );
          })}
        </ul>
      ) : null}

      <ul className="grid gap-2 sm:grid-cols-2">
        <AnimatePresence initial={false}>
          {arrived.map((item, index) => {
            const Icon = kindIcon(item.kind);
            return (
              <motion.li
                key={item.key}
                initial={reduced ? false : { opacity: 0, y: 12, scale: 0.98 }}
                animate={{ opacity: 1, y: 0, scale: 1 }}
                transition={{
                  duration: 0.35,
                  ease: BRAND_EASE,
                  delay: reduced ? 0 : Math.min(index, 6) * 0.03,
                }}
                className={cn(
                  "bg-card flex items-center gap-3 rounded-xl border px-3 py-2.5",
                  item.undone && "opacity-60",
                )}
              >
                <Icon
                  className="text-muted-foreground size-4 shrink-0"
                  aria-hidden
                />
                <div className="min-w-0 flex-1">
                  <p
                    className={cn(
                      "truncate text-sm",
                      item.undone && "line-through",
                    )}
                  >
                    {item.label}
                  </p>
                  <p className="text-muted-foreground truncate text-xs">
                    {item.status === "failed" && item.reason
                      ? item.reason
                      : t(`onboarding.setup.build.item_${item.status}`)}
                  </p>
                </div>
                <ItemState status={item.status} />
              </motion.li>
            );
          })}
        </AnimatePresence>
      </ul>
    </div>
  );
}
