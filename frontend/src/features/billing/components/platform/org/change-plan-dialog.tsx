"use client";

import { ArrowRight, CalendarClock, Zap } from "lucide-react";
import { useMemo, useState } from "react";

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
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  ChoiceCard,
  ChoiceCards,
} from "@/features/billing/components/platform/org/choice-cards";
import {
  findPrice,
  publishedPlanOptions,
} from "@/features/billing/components/platform/org/plan-options";
import { formatMinor } from "@/features/billing/lib/money";
import type {
  BillingPlan,
  BillingPrice,
  BillingSubscription,
} from "@/features/billing/types";
import { useChangeOrganizationPlan } from "@/features/platform-organizations/hooks/use-commerce";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const INTERVAL_KEYS = {
  monthly: "commerce.org.price.per_month",
  yearly: "commerce.org.price.per_year",
} as const;

export function usePriceLabel() {
  const { t, locale } = useLocale();
  return (
    price: Pick<BillingPrice, "amount_minor" | "currency" | "interval">,
  ) =>
    t(INTERVAL_KEYS[price.interval], {
      amount: formatMinor(price.amount_minor, price.currency, locale, {
        compact: true,
      }),
    });
}

/** Plan and price the subscription is on (any version, also unpublished). */
function currentPrice(plans: BillingPlan[], priceUuid: string) {
  for (const plan of plans) {
    for (const version of plan.versions) {
      const price = version.prices.find((p) => p.uuid === priceUuid);
      if (price) return { plan, price };
    }
  }
  return null;
}

/**
 * "Planı değiştir": the operator picks another published price (same
 * currency) and whether it applies now (no invoice, period allowance topped
 * up) or at the period end. The reason lands in the subscription history and
 * the audit log.
 */
export function ChangePlanDialog({
  organization,
  subscription,
  plans,
  onClose,
}: {
  organization: string;
  subscription: BillingSubscription;
  plans: BillingPlan[];
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const priceLabel = usePriceLabel();
  const change = useChangeOrganizationPlan(organization);
  const options = useMemo(() => publishedPlanOptions(plans), [plans]);
  const current = currentPrice(plans, subscription.price_uuid);
  const choices = options
    .map((option) => ({
      ...option,
      prices: option.prices.filter(
        (p) =>
          p.uuid !== subscription.price_uuid &&
          (!current || p.currency === current.price.currency),
      ),
    }))
    .filter((option) => option.prices.length > 0);
  const [priceUuid, setPriceUuid] = useState("");
  const [timing, setTiming] = useState<"now" | "period_end">("now");
  const [reason, setReason] = useState("");
  const target = priceUuid ? findPrice(choices, priceUuid) : null;
  const trimmed = reason.trim();
  const valid = Boolean(target) && trimmed.length > 0;

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[92vh] overflow-y-auto sm:max-w-2xl">
        <form
          className="space-y-5"
          onSubmit={(e) => {
            e.preventDefault();
            if (!valid || !target) return;
            change.mutate(
              { price_uuid: target.price.uuid, timing, reason: trimmed },
              { onSuccess: onClose },
            );
          }}
        >
          <DialogHeader>
            <DialogTitle>{t("commerce.org.change_plan.title")}</DialogTitle>
            <DialogDescription>
              {current
                ? t("commerce.org.change_plan.current", {
                    plan: current.plan.name,
                    price: priceLabel(current.price),
                  })
                : t("commerce.org.change_plan.description")}
            </DialogDescription>
          </DialogHeader>

          <fieldset className="space-y-3">
            <legend className="mb-2 text-sm font-medium">
              {t("commerce.org.change_plan.pick_plan")}
            </legend>
            {choices.length === 0 ? (
              <p className="text-muted-foreground rounded-lg border border-dashed p-4 text-sm">
                {t("commerce.org.change_plan.no_other_plans")}
              </p>
            ) : (
              <ChoiceCards
                value={priceUuid}
                onValueChange={setPriceUuid}
                aria-label={t("commerce.org.change_plan.pick_plan")}
                className="max-h-72 overflow-y-auto p-0.5"
              >
                {choices.flatMap((option) =>
                  option.prices.map((price) => (
                    <ChoiceCard
                      key={price.uuid}
                      value={price.uuid}
                      title={
                        <span className="flex items-center gap-2">
                          {option.plan.name}
                          {option.plan.uuid === current?.plan.uuid ? (
                            <Badge variant="outline" className="font-normal">
                              {t("commerce.org.change_plan.same_plan")}
                            </Badge>
                          ) : null}
                        </span>
                      }
                      aside={priceLabel(price)}
                      description={
                        option.version.trial_days
                          ? t("commerce.org.plan.trial_days", {
                              days: option.version.trial_days,
                            })
                          : t("commerce.org.plan.no_trial")
                      }
                    />
                  )),
                )}
              </ChoiceCards>
            )}
          </fieldset>

          <fieldset className="space-y-3">
            <legend className="mb-2 text-sm font-medium">
              {t("commerce.org.change_plan.timing")}
            </legend>
            <ChoiceCards
              value={timing}
              onValueChange={(v) => setTiming(v as "now" | "period_end")}
              className="sm:grid-cols-2"
              aria-label={t("commerce.org.change_plan.timing")}
            >
              <ChoiceCard
                value="now"
                icon={<Zap className="size-4" />}
                title={t("commerce.org.change_plan.timing_now")}
                description={t("commerce.org.change_plan.timing_now_hint")}
              />
              <ChoiceCard
                value="period_end"
                icon={<CalendarClock className="size-4" />}
                title={t("commerce.org.change_plan.timing_period_end")}
                description={t(
                  "commerce.org.change_plan.timing_period_end_hint",
                  { date: date(subscription.period_end, undefined, locale) },
                )}
              />
            </ChoiceCards>
          </fieldset>

          <div className="space-y-2">
            <Label htmlFor="change-plan-reason">
              {t("commerce.org.common.reason")}
            </Label>
            <Textarea
              id="change-plan-reason"
              required
              maxLength={1000}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={t("commerce.org.change_plan.reason_placeholder")}
            />
            <p className="text-muted-foreground text-xs">
              {t("commerce.org.common.reason_hint")}
            </p>
          </div>

          {target ? (
            <div className="bg-muted/50 flex flex-wrap items-center gap-2 rounded-lg px-4 py-3 text-sm">
              <span className="text-muted-foreground">
                {current ? current.plan.name : "—"}
              </span>
              <ArrowRight className="text-muted-foreground size-4" />
              <span className="font-medium">
                {target.plan.name} · {priceLabel(target.price)}
              </span>
              <span className="text-muted-foreground">
                (
                {timing === "now"
                  ? t("commerce.org.change_plan.summary_now")
                  : t("commerce.org.change_plan.summary_period_end", {
                      date: date(subscription.period_end, undefined, locale),
                    })}
                )
              </span>
            </div>
          ) : null}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!valid || change.isPending}>
              {t("commerce.org.change_plan.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
