"use client";

import { ArrowLeft, ChevronDown, Info, Rocket, Sparkles } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { BRAND_EASE, usePrefersReducedMotion } from "@/lib/motion";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { SETUP_KINDS, kindIcon } from "../../lib/setup-kinds";
import type { SetupItem, SetupRun } from "../../services/setup.service";

function StatusBadge({ status }: { status: SetupItem["status"] }) {
  const { t } = useLocale();
  return (
    <span
      className={cn(
        "shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium",
        status === "new" && "bg-signal-soft text-signal-text",
        status === "exists" && "bg-muted text-muted-foreground",
        status === "unavailable" &&
          "bg-amber-500/10 text-amber-700 dark:text-amber-300",
      )}
    >
      {t(`onboarding.setup.preview.status_${status}`)}
    </span>
  );
}

/**
 * Preview of a draft plan as a diff: what the assistant understood about
 * the business, then every item grouped by kind with a checkbox. Items
 * that already exist are shown but never touched; unavailable ones say
 * why. Unticking a parent (a collection, a team) drops its children.
 */
export function PlanReview({
  run,
  onBack,
  onApply,
  applying,
}: {
  run: SetupRun;
  onBack: () => void;
  onApply: (selection: string[]) => void;
  applying: boolean;
}) {
  const { t, locale } = useLocale();
  const reduced = usePrefersReducedMotion();
  const [selected, setSelected] = useState<Set<string>>(
    () => new Set(run.items.filter((i) => i.selected).map((i) => i.key)),
  );
  const [open, setOpen] = useState<string | null>(null);

  const groups = useMemo(
    () =>
      SETUP_KINDS.map(({ kind }) => ({
        kind,
        items: run.items.filter((i) => i.kind === kind),
      })).filter((g) => g.items.length > 0),
    [run.items],
  );

  const selectable = (item: SetupItem) => item.status === "new";
  const effective = useMemo(
    () =>
      run.items
        .filter((i) => selected.has(i.key))
        .filter(
          (i) =>
            !i.parent ||
            selected.has(i.parent) ||
            run.items.find((p) => p.key === i.parent)?.status === "exists",
        )
        .map((i) => i.key),
    [run.items, selected],
  );

  const toggle = (key: string, on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (on) next.add(key);
      else next.delete(key);
      return next;
    });

  const toggleGroup = (items: SetupItem[], on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev);
      for (const it of items.filter(selectable)) {
        if (on) next.add(it.key);
        else next.delete(it.key);
      }
      return next;
    });

  // AI answers may carry free text (an industry we have no label for).
  const known = (key: string, fallback: string) => {
    const full = `onboarding.setup.${key}`;
    const value = t(full);
    return value === full ? fallback : value;
  };
  const summary = run.plan.summary;
  const brand = run.plan.branding.primary_color;
  const notice = run.discovery.notice;

  return (
    <div className="flex flex-col gap-6">
      <section
        className="bg-card relative overflow-hidden rounded-2xl border p-5 shadow-sm"
        aria-labelledby="setup-summary-title"
      >
        <div
          aria-hidden
          className="absolute inset-x-0 top-0 h-1"
          style={{ background: brand || "var(--setup-accent)" }}
        />
        <div className="flex items-start gap-4">
          <span
            className="grid size-11 shrink-0 place-items-center rounded-xl text-white shadow-sm"
            style={{ background: brand || "var(--setup-accent)" }}
          >
            <Sparkles className="size-5" aria-hidden />
          </span>
          <div className="min-w-0 space-y-1">
            <p className="font-accent text-muted-foreground text-[11px] font-semibold tracking-[0.16em] uppercase">
              {run.source === "template"
                ? t("onboarding.setup.preview.from_template")
                : run.discovery.mode === "fallback"
                  ? t("onboarding.setup.preview.from_fallback")
                  : t("onboarding.setup.preview.from_ai")}
            </p>
            <h2
              id="setup-summary-title"
              className="font-display text-xl font-semibold tracking-tight"
            >
              {summary.company_name || t("onboarding.setup.preview.your_team")}
            </h2>
            {summary.description ? (
              <p className="text-muted-foreground text-sm text-pretty">
                {summary.description}
              </p>
            ) : null}
            <p className="text-muted-foreground flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
              {summary.industry ? (
                <span>
                  {known(`industries.${summary.industry}`, summary.industry)}
                </span>
              ) : null}
              <span>{t(`onboarding.setup.tones.${summary.tone}`)}</span>
              {summary.languages.length ? (
                <span>
                  {summary.languages
                    .map((l) => known(`languages.${l}`, l.toUpperCase()))
                    .join(" · ")}
                </span>
              ) : null}
              {brand ? (
                <span className="inline-flex items-center gap-1">
                  <span
                    className="size-3 rounded-full border"
                    style={{ background: brand }}
                    aria-hidden
                  />
                  {brand}
                </span>
              ) : null}
            </p>
          </div>
        </div>
        {notice ? (
          <p className="mt-4 flex items-start gap-2 rounded-lg bg-amber-500/10 px-3 py-2 text-sm text-amber-800 dark:text-amber-200">
            <Info className="mt-0.5 size-4 shrink-0" aria-hidden />
            {t(`onboarding.setup.notice.${notice}`)}
          </p>
        ) : null}
      </section>

      <ul className="flex flex-col gap-2">
        {groups.map(({ kind, items }, gi) => {
          const Icon = kindIcon(kind);
          const pickable = items.filter(selectable);
          const on = pickable.filter((i) => selected.has(i.key)).length;
          const expanded = open === kind;
          return (
            <motion.li
              key={kind}
              initial={reduced ? false : { opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{
                duration: 0.3,
                ease: BRAND_EASE,
                delay: gi * 0.025,
              }}
              className="bg-card rounded-xl border"
            >
              <div className="flex items-center gap-3 px-4 py-3">
                <Checkbox
                  aria-label={t(`onboarding.setup.kinds.${kind}`)}
                  disabled={pickable.length === 0}
                  checked={
                    on === 0
                      ? false
                      : on === pickable.length
                        ? true
                        : "indeterminate"
                  }
                  onCheckedChange={(v) => toggleGroup(items, v === true)}
                />
                <button
                  type="button"
                  aria-expanded={expanded}
                  onClick={() => setOpen(expanded ? null : kind)}
                  className="flex min-w-0 flex-1 items-center gap-3 text-left"
                >
                  <Icon
                    className="text-muted-foreground size-4 shrink-0"
                    aria-hidden
                  />
                  <span className="flex-1 truncate text-sm font-medium">
                    {t(`onboarding.setup.kinds.${kind}`)}
                  </span>
                  <span className="text-muted-foreground text-xs tabular-nums">
                    {t("onboarding.setup.preview.picked", {
                      on,
                      total: items.length,
                    })}
                  </span>
                  <ChevronDown
                    className={cn(
                      "text-muted-foreground size-4 transition-transform",
                      expanded && "rotate-180",
                    )}
                    aria-hidden
                  />
                </button>
              </div>
              <AnimatePresence initial={false}>
                {expanded ? (
                  <motion.ul
                    initial={reduced ? false : { height: 0, opacity: 0 }}
                    animate={{ height: "auto", opacity: 1 }}
                    exit={reduced ? undefined : { height: 0, opacity: 0 }}
                    transition={{ duration: 0.25, ease: BRAND_EASE }}
                    className="overflow-hidden border-t"
                  >
                    {items.map((item) => (
                      <li
                        key={item.key}
                        className="flex items-start gap-3 px-4 py-2.5"
                      >
                        <Checkbox
                          className="mt-0.5"
                          aria-label={item.label}
                          disabled={!selectable(item)}
                          checked={selectable(item) && selected.has(item.key)}
                          onCheckedChange={(v) => toggle(item.key, v === true)}
                        />
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm">{item.label}</p>
                          {item.detail ? (
                            <p className="text-muted-foreground line-clamp-2 text-xs">
                              {item.detail}
                            </p>
                          ) : null}
                          {item.status === "unavailable" && item.reason ? (
                            <p className="text-xs text-amber-700 dark:text-amber-300">
                              {t(`onboarding.setup.reason.${item.reason}`)}
                            </p>
                          ) : null}
                        </div>
                        <StatusBadge status={item.status} />
                      </li>
                    ))}
                  </motion.ul>
                ) : null}
              </AnimatePresence>
            </motion.li>
          );
        })}
      </ul>

      <div className="bg-background/90 sticky bottom-0 -mx-4 flex flex-wrap items-center justify-between gap-3 border-t px-4 py-4 backdrop-blur sm:-mx-8 sm:px-8">
        <Button variant="ghost" onClick={onBack} disabled={applying}>
          <ArrowLeft />
          {t("onboarding.setup.back")}
        </Button>
        <div className="flex items-center gap-3">
          <span className="text-muted-foreground text-sm tabular-nums">
            {t("onboarding.setup.preview.selected", {
              count: effective.length.toLocaleString(locale),
            })}
          </span>
          <Button
            size="lg"
            disabled={effective.length === 0 || applying}
            onClick={() => onApply(effective)}
            className="text-white"
            style={{ background: "var(--setup-accent)" }}
          >
            <Rocket />
            {t("onboarding.setup.preview.apply")}
          </Button>
        </div>
      </div>
    </div>
  );
}
