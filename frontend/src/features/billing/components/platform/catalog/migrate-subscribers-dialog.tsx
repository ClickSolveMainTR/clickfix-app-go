"use client";

import { Search } from "lucide-react";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import {
  usePlanSubscribers,
  useSchedulePlanMigration,
} from "@/features/billing/hooks/use-credit-products";
import { formatCount, formatMinor } from "@/features/billing/lib/money";
import type {
  BillingAdminSubscription,
  BillingPlan,
  BillingVersion,
} from "@/features/billing/types";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const MAX = 100;

type Candidate = {
  row: BillingAdminSubscription;
  /** Why it cannot move; null when selectable. */
  blocked: "cancel" | "price" | null;
};

/**
 * "Aboneleri taşı": schedule subscribers of a plan's older versions onto a
 * published version. Each move happens at that subscriber's next renewal, on
 * the same billing period and currency; at most 100 per batch.
 */
export function MigrateSubscribersDialog({
  open,
  onClose,
  plan,
  target,
}: {
  open: boolean;
  onClose: () => void;
  plan: BillingPlan;
  target: BillingVersion;
}) {
  const { t, locale } = useLocale();
  const subscribers = usePlanSubscribers(plan.uuid, open);
  const schedule = useSchedulePlanMigration(plan.uuid);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [reason, setReason] = useState("");
  const [q, setQ] = useState("");

  const candidates = useMemo<Candidate[]>(() => {
    const offered = new Set(
      target.prices.map((p) => `${p.interval}:${p.currency}`),
    );
    return (subscribers.data?.items ?? [])
      .filter((row) => row.version_uuid !== target.uuid)
      .map((row) => ({
        row,
        blocked: row.cancel_at_period_end
          ? ("cancel" as const)
          : offered.has(`${row.price_interval}:${row.price_currency}`)
            ? null
            : ("price" as const),
      }))
      .sort(
        (a, b) =>
          Number(Boolean(a.blocked)) - Number(Boolean(b.blocked)) ||
          a.row.organization_name.localeCompare(b.row.organization_name),
      );
  }, [subscribers.data, target]);

  const visible = useMemo(() => {
    const needle = q.trim().toLocaleLowerCase(locale);
    return needle
      ? candidates.filter((c) =>
          c.row.organization_name.toLocaleLowerCase(locale).includes(needle),
        )
      : candidates;
  }, [candidates, q, locale]);

  const selectable = visible.filter((c) => !c.blocked);
  const allVisibleSelected =
    selectable.length > 0 && selectable.every((c) => selected.has(c.row.uuid));

  const toggle = (uuid: string, on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (on && next.size < MAX) next.add(uuid);
      else next.delete(uuid);
      return next;
    });

  const toggleAll = (on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev);
      for (const c of selectable) {
        if (on && next.size < MAX) next.add(c.row.uuid);
        if (!on) next.delete(c.row.uuid);
      }
      return next;
    });

  const close = () => {
    if (schedule.isPending) return;
    setSelected(new Set());
    setReason("");
    setQ("");
    onClose();
  };

  const submit = async () => {
    const rows = candidates.filter((c) => selected.has(c.row.uuid));
    await schedule.mutateAsync({
      version_uuid: target.uuid,
      subscription_uuids: rows.map((c) => c.row.uuid),
      organization_uuids: rows.map((c) => c.row.organization_uuid),
      reason: reason.trim(),
    });
    close();
  };

  return (
    <Dialog open={open} onOpenChange={(v) => !v && close()}>
      <DialogContent className="flex max-h-[92dvh] flex-col sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t("credit_products.migrate.title", { version: target.version })}
          </DialogTitle>
          <DialogDescription>
            {t("credit_products.migrate.description")}
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <div className="relative sm:w-64">
              <Search className="text-muted-foreground pointer-events-none absolute start-2.5 top-1/2 size-4 -translate-y-1/2" />
              <Input
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder={t("credit_products.migrate.search")}
                aria-label={t("credit_products.migrate.search")}
                className="h-9 ps-8"
              />
            </div>
            <p className="text-muted-foreground text-xs tabular-nums">
              {t("credit_products.migrate.selected", {
                count: formatCount(selected.size, locale),
                max: MAX,
              })}
            </p>
          </div>

          {subscribers.isError ? (
            <ErrorState
              title={t("common.error_generic")}
              onRetry={() => void subscribers.refetch()}
              retryLabel={t("common.retry")}
            />
          ) : subscribers.isLoading ? (
            <div className="space-y-2">
              {Array.from({ length: 4 }, (_, i) => (
                <Skeleton key={i} className="h-12 rounded-md" />
              ))}
            </div>
          ) : candidates.length === 0 ? (
            <EmptyState
              className="py-8"
              title={t("credit_products.migrate.empty")}
              description={t("credit_products.migrate.empty_hint")}
            />
          ) : (
            <div className="overflow-hidden rounded-lg border">
              <label className="bg-muted/40 flex items-center gap-3 border-b px-3 py-2 text-xs font-medium">
                <Checkbox
                  checked={allVisibleSelected}
                  disabled={selectable.length === 0}
                  onCheckedChange={(v) => toggleAll(v === true)}
                  aria-label={t("credit_products.migrate.select_all")}
                />
                {t("credit_products.migrate.select_all")}
              </label>
              <ul className="divide-y">
                {visible.map(({ row, blocked }) => (
                  <li key={row.uuid}>
                    <label
                      className={
                        blocked
                          ? "flex items-start gap-3 px-3 py-2.5 opacity-60"
                          : "hover:bg-muted/30 flex cursor-pointer items-start gap-3 px-3 py-2.5"
                      }
                    >
                      <Checkbox
                        className="mt-0.5"
                        checked={selected.has(row.uuid)}
                        disabled={
                          Boolean(blocked) ||
                          (!selected.has(row.uuid) && selected.size >= MAX)
                        }
                        onCheckedChange={(v) => toggle(row.uuid, v === true)}
                        aria-label={row.organization_name}
                      />
                      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                        <span className="flex flex-wrap items-center gap-2 text-sm font-medium">
                          <span className="truncate">
                            {row.organization_name}
                          </span>
                          <Badge variant="secondary" className="font-normal">
                            {t("credit_products.migrate.on_version", {
                              version: row.version_number,
                            })}
                          </Badge>
                        </span>
                        <span className="text-muted-foreground text-xs tabular-nums">
                          {t(`billing.interval.${row.price_interval}`)} ·{" "}
                          {formatMinor(
                            row.price_amount_minor,
                            row.price_currency,
                            locale,
                          )}{" "}
                          ·{" "}
                          {t("credit_products.migrate.renews", {
                            date: date(row.period_end, undefined, locale),
                          })}
                        </span>
                        {blocked ? (
                          <span className="text-xs text-amber-700 dark:text-amber-400">
                            {t(`credit_products.migrate.blocked_${blocked}`)}
                          </span>
                        ) : null}
                      </span>
                    </label>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {subscribers.data?.truncated ? (
            <p className="text-muted-foreground text-xs">
              {t("credit_products.migrate.truncated")}
            </p>
          ) : null}

          <div className="space-y-1.5">
            <Label htmlFor="migrate-reason">
              {t("credit_products.migrate.reason")}
            </Label>
            <Textarea
              id="migrate-reason"
              rows={2}
              maxLength={500}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={t("credit_products.migrate.reason_placeholder")}
            />
            <p className="text-muted-foreground text-xs">
              {t("credit_products.migrate.reason_hint")}
            </p>
          </div>
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={close}
            disabled={schedule.isPending}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={
              selected.size === 0 || !reason.trim() || schedule.isPending
            }
            onClick={() => void submit()}
          >
            {t("credit_products.migrate.submit", {
              count: formatCount(selected.size, locale),
            })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
