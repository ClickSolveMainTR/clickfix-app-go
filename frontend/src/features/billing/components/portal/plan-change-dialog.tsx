"use client";

import { AlertTriangle, ArrowRight } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  useApplyPlanChange,
  useCatalog,
  usePlanChangePreview,
} from "@/features/billing/hooks/use-tenant-billing";
import { formatMinor } from "@/features/billing/lib/money";
import { latestPublished } from "@/features/billing/lib/plans";
import type {
  BillingPrice,
  BillingSubscription,
} from "@/features/billing/types";
import { useDebounce } from "@/hooks/use-debounce";
import { ApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Plan / interval / seat change with a server preview (CF-34): upgrades are
 * prorated for the remaining days and take effect after payment; downgrades
 * apply at period end and list what has to be reduced first.
 */
export function PlanChangeDialog({
  subscription,
  currentPrice,
  seatsInUse,
  onClose,
  onPayNow,
}: {
  subscription: BillingSubscription;
  currentPrice: BillingPrice | undefined;
  seatsInUse: number;
  onClose: () => void;
  onPayNow: () => void;
}) {
  const { t, locale } = useLocale();
  const catalog = useCatalog();
  const preview = usePlanChangePreview();
  const apply = useApplyPlanChange();
  const [interval, setInterval] = useState<"monthly" | "yearly">(
    currentPrice?.interval ?? "monthly",
  );
  const currency = currentPrice?.currency ?? "TRY";
  const [priceUuid, setPriceUuid] = useState<string>("");
  const [seats, setSeats] = useState(String(Math.max(1, seatsInUse)));
  const seatCount = Number(seats);
  const seatsValid = Number.isInteger(seatCount) && seatCount >= 1;

  const options = useMemo(
    () =>
      (catalog.data?.items ?? [])
        .map((plan) => {
          const version = latestPublished(plan);
          const price = version?.prices.find(
            (p) => p.interval === interval && p.currency === currency,
          );
          return price ? { plan, price } : null;
        })
        .filter((o): o is NonNullable<typeof o> => o !== null),
    [catalog.data, interval, currency],
  );

  const target = useMemo(
    () => ({ price: priceUuid, seats: seatCount }),
    [priceUuid, seatCount],
  );
  const debounced = useDebounce(target, 300);
  const { mutate: runPreview, reset: resetPreview } = preview;
  useEffect(() => {
    if (
      !debounced.price ||
      !Number.isInteger(debounced.seats) ||
      debounced.seats < 1
    ) {
      resetPreview();
      return;
    }
    runPreview({ price_uuid: debounced.price, seats: debounced.seats });
  }, [debounced, runPreview, resetPreview]);

  const result = preview.data;
  const reductions = Object.entries(result?.reductions ?? {}).filter(
    ([, n]) => n > 0,
  );
  const previewError = preview.error instanceof ApiError ? preview.error : null;

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("billing.portal.change.title")}</DialogTitle>
          <DialogDescription>
            {t("billing.portal.change.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-wrap items-end justify-between gap-3">
          <ToggleGroup
            type="single"
            variant="outline"
            value={interval}
            onValueChange={(v) => {
              if (v) {
                setInterval(v as "monthly" | "yearly");
                setPriceUuid("");
              }
            }}
            aria-label={t("billing.portal.change.interval")}
          >
            <ToggleGroupItem value="monthly">
              {t("billing.interval.monthly")}
            </ToggleGroupItem>
            <ToggleGroupItem value="yearly">
              {t("billing.interval.yearly")}
            </ToggleGroupItem>
          </ToggleGroup>
          <div className="w-32 space-y-1">
            <Label htmlFor="change-seats">
              {t("billing.portal.change.seats")}
            </Label>
            <Input
              id="change-seats"
              type="number"
              min={1}
              value={seats}
              aria-invalid={!seatsValid}
              onChange={(e) => setSeats(e.target.value)}
            />
          </div>
        </div>
        {catalog.isLoading ? <Skeleton className="h-32 w-full" /> : null}
        <div
          role="radiogroup"
          aria-label={t("billing.portal.change.plans")}
          className="grid gap-2 sm:grid-cols-2"
        >
          {options.map(({ plan, price }) => {
            const current = price.uuid === subscription.price_uuid;
            const selected = price.uuid === priceUuid;
            return (
              <button
                key={price.uuid}
                type="button"
                role="radio"
                aria-checked={selected}
                onClick={() => setPriceUuid(price.uuid ?? "")}
                className={cn(
                  "hover:border-foreground/30 rounded-lg border p-3 text-start transition-colors",
                  selected && "border-primary ring-primary/30 ring-2",
                )}
              >
                <span className="flex items-center justify-between gap-2">
                  <span className="font-medium">{plan.name}</span>
                  {current ? (
                    <Badge variant="secondary">
                      {t("billing.portal.change.current")}
                    </Badge>
                  ) : null}
                </span>
                <span className="mt-1 block text-lg font-semibold tabular-nums">
                  {formatMinor(price.amount_minor, price.currency, locale)}
                  <span className="text-muted-foreground ms-1 text-xs font-normal">
                    / {t(`billing.interval.${price.interval}`)}
                  </span>
                </span>
                {price.seat_amount_minor ? (
                  <span className="text-muted-foreground text-xs">
                    {t("billing.portal.change.seat_price", {
                      amount: formatMinor(
                        price.seat_amount_minor,
                        price.currency,
                        locale,
                      ),
                    })}
                  </span>
                ) : null}
              </button>
            );
          })}
          {catalog.data && options.length === 0 ? (
            <p className="text-muted-foreground text-sm sm:col-span-2">
              {t("billing.portal.change.no_options")}
            </p>
          ) : null}
        </div>

        <div aria-live="polite" className="min-h-16">
          {preview.isPending ? <Skeleton className="h-16 w-full" /> : null}
          {previewError ? (
            <p className="text-destructive flex items-start gap-2 text-sm">
              <AlertTriangle className="mt-0.5 size-4 shrink-0" />
              {previewError.status === 409
                ? t("billing.portal.change.not_allowed")
                : previewError.message}
            </p>
          ) : null}
          {result ? (
            <div className="bg-muted/40 space-y-2 rounded-lg border p-3 text-sm">
              <p className="flex items-center gap-2 font-medium">
                {result.immediate
                  ? t("billing.portal.change.immediate", {
                      amount: formatMinor(
                        result.amount_minor,
                        result.currency,
                        locale,
                      ),
                    })
                  : t("billing.portal.change.at_period_end", {
                      date: date(result.effective_at, undefined, locale),
                    })}
              </p>
              <p className="text-muted-foreground text-xs">
                {result.immediate
                  ? t("billing.portal.change.immediate_hint")
                  : t("billing.portal.change.period_end_hint")}
              </p>
              {reductions.length ? (
                <div className="rounded-md border border-amber-500/30 bg-amber-500/10 p-2 text-xs">
                  <p className="font-medium">
                    {t("billing.portal.change.reductions")}
                  </p>
                  <ul className="mt-1 list-disc ps-4">
                    {reductions.map(([key, n]) => (
                      <li key={key}>
                        {t("billing.portal.change.reduce", {
                          count: n,
                          feature: t(`billing.portal.features.${key}`),
                        })}
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null}
            </div>
          ) : null}
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={!result || !seatsValid || apply.isPending}
            onClick={() =>
              apply.mutate(
                { price_uuid: priceUuid, seats: seatCount },
                {
                  onSuccess: () => {
                    onClose();
                    if (result?.immediate && result.amount_minor > 0)
                      onPayNow();
                  },
                },
              )
            }
          >
            {result?.immediate
              ? t("billing.portal.change.confirm_upgrade")
              : t("billing.portal.change.confirm_schedule")}
            <ArrowRight className="size-4" />
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
