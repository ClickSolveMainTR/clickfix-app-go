"use client";

import { addDays, addMonths, addYears } from "date-fns";
import {
  ArrowRightLeft,
  CalendarClock,
  CalendarPlus,
  Clock,
  Gift,
  Pause,
  Play,
  PlusCircle,
  RotateCcw,
  ShieldOff,
  SlidersHorizontal,
  XCircle,
} from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Timeline } from "@/components/common/timeline";
import { EntitySectionCard } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DatePicker } from "@/components/ui/date-picker";
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
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { BillingStatus } from "@/features/billing/components/billing-status";
import {
  ChangePlanDialog,
  usePriceLabel,
} from "@/features/billing/components/platform/org/change-plan-dialog";
import {
  ChoiceCard,
  ChoiceCards,
} from "@/features/billing/components/platform/org/choice-cards";
import {
  daysUntil,
  findPrice,
  findVersion,
  publishedPlanOptions,
} from "@/features/billing/components/platform/org/plan-options";
import {
  useAssignPlan,
  useBillingFeatures,
  useExtendTrial,
  useOrgEntitlements,
  useOrgSubscription,
  useOrgUsage,
  usePlans,
  useSetOverride,
  useSubscriptionHistory,
  useTransitionSubscription,
} from "@/features/billing/hooks/use-platform-billing";
import { formatBytes, formatCount } from "@/features/billing/lib/money";
import { featureLabel } from "@/features/billing/lib/plans";
import type {
  BillingHistoryEntry,
  BillingPlan,
  BillingSubscription,
  BillingTransition,
  EntitlementValue,
} from "@/features/billing/types";
import {
  endOfDayISO,
  isoToDateInput,
} from "@/features/platform-organizations/lib/access-dates";
import { cn } from "@/lib/utils";
import { date, datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Action =
  | { kind: "transition"; to: BillingTransition["to"]; titleKey: string }
  | { kind: "free_month" }
  | { kind: "extend_trial" };

/** Transitions offered per status (subset of the server state machine). */
const ACTIONS: Record<
  string,
  { to: BillingTransition["to"]; key: string; icon: typeof Pause }[]
> = {
  pending_payment: [
    { to: "active", key: "activate", icon: Play },
    { to: "canceled", key: "cancel", icon: XCircle },
  ],
  trialing: [
    { to: "active", key: "activate", icon: Play },
    { to: "canceled", key: "cancel", icon: XCircle },
  ],
  active: [
    { to: "paused", key: "pause", icon: Pause },
    { to: "canceled", key: "cancel", icon: XCircle },
  ],
  past_due: [
    { to: "active", key: "activate", icon: Play },
    { to: "canceled", key: "cancel", icon: XCircle },
  ],
  grace: [
    { to: "active", key: "activate", icon: Play },
    { to: "suspended", key: "suspend", icon: ShieldOff },
    { to: "canceled", key: "cancel", icon: XCircle },
  ],
  suspended: [
    { to: "active", key: "reactivate", icon: RotateCcw },
    { to: "canceled", key: "cancel", icon: XCircle },
  ],
  paused: [
    { to: "active", key: "resume", icon: Play },
    { to: "canceled", key: "cancel", icon: XCircle },
  ],
};

/** Statuses the server accepts a platform plan change from. */
const CHANGEABLE = new Set([
  "trialing",
  "active",
  "past_due",
  "grace",
  "pending_payment",
]);

function ActionDialog({
  action,
  subscription,
  organization,
  onClose,
}: {
  action: Action | null;
  subscription: BillingSubscription;
  organization: string;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const transition = useTransitionSubscription(organization);
  const extend = useExtendTrial(organization);
  const [reason, setReason] = useState("");
  const [openedAt] = useState(() => Date.now());
  const [until, setUntil] = useState(() =>
    isoToDateInput(
      addDays(
        subscription.trial_ends_at
          ? new Date(subscription.trial_ends_at)
          : new Date(),
        7,
      ).toISOString(),
    ),
  );
  if (!action) return null;
  const pending = transition.isPending || extend.isPending;
  const freeMonthEnd = addMonths(new Date(subscription.period_end), 1);
  const title =
    action.kind === "transition"
      ? t(action.titleKey)
      : action.kind === "free_month"
        ? t("billing.subscription.actions.free_month")
        : t("billing.subscription.actions.extend_trial");
  const description =
    action.kind === "free_month"
      ? t("billing.subscription.free_month_description", {
          from: date(subscription.period_end, undefined, locale),
          to: date(freeMonthEnd, undefined, locale),
        })
      : action.kind === "extend_trial"
        ? t("billing.subscription.extend_trial_description")
        : action.to === "canceled"
          ? t("billing.subscription.cancel_description")
          : t("billing.subscription.transition_description");
  const trialEnd = endOfDayISO(until);
  const trialInvalid =
    action.kind === "extend_trial" &&
    (!trialEnd ||
      new Date(trialEnd) <= new Date(subscription.trial_ends_at ?? openedAt));

  const submit = () => {
    const done = { onSuccess: onClose };
    if (action.kind === "extend_trial") {
      if (!trialEnd) return;
      extend.mutate(
        { subscription: subscription.uuid, endsAt: trialEnd, reason },
        done,
      );
      return;
    }
    if (action.kind === "free_month") {
      transition.mutate(
        {
          subscription: subscription.uuid,
          body: {
            to: "active",
            reason,
            period_start: subscription.period_end,
            period_end: freeMonthEnd.toISOString(),
          },
        },
        done,
      );
      return;
    }
    transition.mutate(
      { subscription: subscription.uuid, body: { to: action.to, reason } },
      done,
    );
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (reason.trim() && !trialInvalid) submit();
          }}
        >
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>{description}</DialogDescription>
          </DialogHeader>
          {action.kind === "extend_trial" ? (
            <div className="space-y-2">
              <Label htmlFor="trial-until">
                {t("billing.subscription.trial_until")}
              </Label>
              <DatePicker
                id="trial-until"
                value={until}
                onChange={setUntil}
                aria-invalid={trialInvalid}
              />
              {trialInvalid ? (
                <p className="text-destructive text-xs">
                  {t("billing.subscription.trial_until_invalid")}
                </p>
              ) : null}
            </div>
          ) : null}
          <div className="space-y-2">
            <Label htmlFor="action-reason">
              {t("commerce.org.common.reason")}
            </Label>
            <Textarea
              id="action-reason"
              required
              maxLength={1000}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={t("billing.subscription.reason_placeholder")}
            />
            <p className="text-muted-foreground text-xs">
              {t("commerce.org.common.reason_hint")}
            </p>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              variant={
                action.kind === "transition" &&
                (action.to === "canceled" || action.to === "suspended")
                  ? "destructive"
                  : "default"
              }
              disabled={!reason.trim() || trialInvalid || pending}
            >
              {title}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function AssignPlanDialog({
  organization,
  plans,
  onClose,
}: {
  organization: string;
  plans: BillingPlan[];
  onClose: () => void;
}) {
  const { t } = useLocale();
  const priceLabel = usePriceLabel();
  const assign = useAssignPlan(organization);
  const options = useMemo(() => publishedPlanOptions(plans), [plans]);
  const [priceUuid, setPriceUuid] = useState(
    () => options[0]?.prices[0]?.uuid ?? "",
  );
  const selected = priceUuid ? findPrice(options, priceUuid) : null;
  const trialDays = selected?.version.trial_days ?? 0;
  const [trial, setTrial] = useState(true);
  const withTrial = trial && trialDays > 0;

  return (
    <Dialog open onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-h-[92vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("billing.subscription.assign_title")}</DialogTitle>
          <DialogDescription>
            {t("commerce.org.assign.description")}
          </DialogDescription>
        </DialogHeader>
        {options.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("billing.subscription.no_published_plans")}
          </p>
        ) : (
          <div className="space-y-4">
            <ChoiceCards
              value={priceUuid}
              onValueChange={setPriceUuid}
              aria-label={t("billing.subscription.plan")}
              className="max-h-72 overflow-y-auto p-0.5"
            >
              {options.flatMap((option) =>
                option.prices.map((price) => (
                  <ChoiceCard
                    key={price.uuid}
                    value={price.uuid}
                    title={option.plan.name}
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
            {trialDays > 0 ? (
              <Label className="flex items-center gap-2 font-normal">
                <Checkbox
                  checked={trial}
                  onCheckedChange={(v) => setTrial(v === true)}
                />
                {t("billing.subscription.assign_trial", { days: trialDays })}
              </Label>
            ) : null}
          </div>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={!selected || assign.isPending}
            onClick={() => {
              if (!selected) return;
              const now = new Date();
              const end = withTrial
                ? addDays(now, trialDays)
                : selected.price.interval === "yearly"
                  ? addYears(now, 1)
                  : addMonths(now, 1);
              assign.mutate(
                {
                  organization_uuid: organization,
                  version_uuid: selected.version.uuid,
                  price_uuid: selected.price.uuid,
                  status: withTrial ? "trialing" : "active",
                  period_start: now.toISOString(),
                  period_end: end.toISOString(),
                  trial_ends_at: withTrial ? end.toISOString() : null,
                },
                { onSuccess: onClose },
              );
            }}
          >
            {t("billing.subscription.assign")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function OverrideDialog({
  organization,
  value,
  label,
  onClose,
}: {
  organization: string;
  value: EntitlementValue;
  label: string;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const setOverride = useSetOverride(organization);
  const [enabled, setEnabled] = useState(value.enabled ?? false);
  const [unlimited, setUnlimited] = useState(value.unlimited ?? false);
  const [limit, setLimit] = useState(String(value.limit ?? 0));
  const [reason, setReason] = useState("");
  const [expires, setExpires] = useState("");
  const isLimit = value.kind === "limit";
  const limitNumber = Number(limit);
  const limitInvalid =
    isLimit &&
    !unlimited &&
    !(Number.isSafeInteger(limitNumber) && limitNumber >= 0);

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (!reason.trim() || limitInvalid) return;
            setOverride.mutate(
              {
                feature: value.feature,
                body: {
                  feature: value.feature,
                  ...(isLimit
                    ? unlimited
                      ? { unlimited: true }
                      : { limit: limitNumber, unlimited: false }
                    : { enabled }),
                  enforcement: value.enforcement,
                  warn_percent: value.warn_percent ?? 80,
                  reason: reason.trim(),
                  expires_at: expires ? (endOfDayISO(expires) ?? null) : null,
                },
              },
              { onSuccess: onClose },
            );
          }}
        >
          <DialogHeader>
            <DialogTitle>
              {t("commerce.org.usage.override_title", { feature: label })}
            </DialogTitle>
            <DialogDescription>
              {t("commerce.org.usage.override_description")}
            </DialogDescription>
          </DialogHeader>
          {isLimit ? (
            <div className="flex items-end gap-4">
              <div className="flex-1 space-y-2">
                <Label htmlFor="override-limit">
                  {t("billing.entitlements.limit")}
                </Label>
                <Input
                  id="override-limit"
                  type="number"
                  min={0}
                  disabled={unlimited}
                  value={unlimited ? "" : limit}
                  aria-invalid={limitInvalid}
                  onChange={(e) => setLimit(e.target.value)}
                />
              </div>
              <Label className="flex h-9 items-center gap-2 font-normal">
                <Checkbox
                  checked={unlimited}
                  onCheckedChange={(v) => setUnlimited(v === true)}
                />
                {t("billing.common.unlimited")}
              </Label>
            </div>
          ) : (
            <Label className="flex items-center justify-between gap-3 rounded-md border p-3 font-normal">
              {t("billing.entitlements.enabled")}
              <Switch checked={enabled} onCheckedChange={setEnabled} />
            </Label>
          )}
          <div className="space-y-2">
            <Label htmlFor="override-expires">
              {t("billing.entitlements.expires")}
            </Label>
            <DatePicker
              id="override-expires"
              value={expires}
              onChange={setExpires}
              placeholder={t("billing.entitlements.expires_never")}
            />
            <p className="text-muted-foreground text-xs">
              {t("commerce.org.usage.override_expires_hint")}
            </p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="override-reason">
              {t("commerce.org.common.reason")}
            </Label>
            <Textarea
              id="override-reason"
              required
              maxLength={1000}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
            <p className="text-muted-foreground text-xs">
              {t("commerce.org.common.reason_hint")}
            </p>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={!reason.trim() || limitInvalid || setOverride.isPending}
            >
              {t("commerce.org.usage.override_save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function Fact({
  label,
  value,
  hint,
}: {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
}) {
  return (
    <div className="space-y-1">
      <dt className="text-muted-foreground text-xs font-medium">{label}</dt>
      <dd className="text-sm font-medium">{value}</dd>
      {hint ? <dd className="text-muted-foreground text-xs">{hint}</dd> : null}
    </div>
  );
}

function SubscriptionCard({
  organization,
  canWrite,
}: {
  organization: string;
  canWrite: boolean;
}) {
  const { t, locale } = useLocale();
  const priceLabel = usePriceLabel();
  const sub = useOrgSubscription(organization);
  const plans = usePlans({ limit: 100, offset: 0 });
  const [action, setAction] = useState<Action | null>(null);
  const [dialog, setDialog] = useState<"assign" | "change" | null>(null);
  const s = sub.data;
  const resolved = s ? findVersion(plans.data?.items, s.version_uuid) : null;
  const price = resolved?.version.prices.find((p) => p.uuid === s?.price_uuid);
  const pending = s?.pending_version_uuid
    ? findVersion(plans.data?.items, s.pending_version_uuid)
    : null;
  const terminal = !s || s.status === "canceled" || s.status === "expired";
  const trialLeft =
    s?.status === "trialing" ? daysUntil(s.trial_ends_at) : null;
  const canChange =
    s && CHANGEABLE.has(s.status) && !s.cancel_at_period_end && plans.data;

  if (sub.isLoading) {
    return (
      <EntitySectionCard title={t("commerce.org.subscription.current_plan")}>
        <Skeleton className="h-48 w-full" />
      </EntitySectionCard>
    );
  }
  if (sub.isError) {
    return (
      <EntitySectionCard title={t("commerce.org.subscription.current_plan")}>
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void sub.refetch()}
          retryLabel={t("common.retry")}
        />
      </EntitySectionCard>
    );
  }

  const assignDialog =
    dialog === "assign" && plans.data ? (
      <AssignPlanDialog
        organization={organization}
        plans={plans.data.items}
        onClose={() => setDialog(null)}
      />
    ) : null;

  if (!s) {
    return (
      <EntitySectionCard title={t("commerce.org.subscription.current_plan")}>
        <EmptyState
          className="py-10"
          icon={<CalendarClock className="text-signal size-5" />}
          title={t("commerce.org.subscription.none_title")}
          description={t("commerce.org.subscription.none_description")}
          action={
            canWrite ? (
              <Button
                type="button"
                onClick={() => setDialog("assign")}
                disabled={!plans.data}
              >
                <PlusCircle className="size-4" />
                {t("billing.subscription.assign")}
              </Button>
            ) : null
          }
        />
        {assignDialog}
      </EntitySectionCard>
    );
  }

  return (
    <EntitySectionCard title={t("commerce.org.subscription.current_plan")}>
      <div className="space-y-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0 space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="font-display text-xl font-semibold tracking-tight">
                {resolved?.plan.name ?? "—"}
              </h3>
              <BillingStatus kind="subscription" status={s.status} />
              {s.cancel_at_period_end ? (
                <Badge variant="outline">
                  {t("billing.subscriptions.cancel_scheduled")}
                </Badge>
              ) : null}
            </div>
            <p className="text-muted-foreground text-sm">
              {price ? priceLabel(price) : "—"}
              {resolved
                ? ` · ${t("commerce.org.subscription.version", { version: resolved.version.version })}`
                : ""}
            </p>
          </div>
          {trialLeft !== null ? (
            <span
              className={cn(
                "inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-sm font-medium",
                trialLeft <= 3
                  ? "bg-amber-500/15 text-amber-700 dark:text-amber-300"
                  : "bg-primary/10 text-primary",
              )}
            >
              <Clock className="size-3.5" />
              {trialLeft === 0
                ? t("commerce.org.trial.ends_today")
                : trialLeft === 1
                  ? t("commerce.org.trial.ends_in_one_day")
                  : t("commerce.org.trial.ends_in", { count: trialLeft })}
            </span>
          ) : null}
        </div>

        <dl className="grid gap-4 rounded-lg border p-4 sm:grid-cols-2 lg:grid-cols-3">
          <Fact
            label={t("billing.subscription.period")}
            value={`${date(s.period_start, undefined, locale)} – ${date(s.period_end, undefined, locale)}`}
          />
          {s.trial_ends_at ? (
            <Fact
              label={t("billing.subscription.trial_ends")}
              value={datetime(s.trial_ends_at, undefined, locale)}
            />
          ) : (
            <Fact
              label={
                s.cancel_at_period_end
                  ? t("commerce.org.subscription.ends_on")
                  : t("commerce.org.subscription.renews_on")
              }
              value={date(s.period_end, undefined, locale)}
            />
          )}
          {s.grace_ends_at ? (
            <Fact
              label={t("billing.subscription.grace_ends")}
              value={datetime(s.grace_ends_at, undefined, locale)}
            />
          ) : null}
        </dl>

        {s.pending_version_uuid ? (
          <div className="flex items-start gap-3 rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm">
            <ArrowRightLeft className="mt-0.5 size-4 shrink-0 text-amber-700 dark:text-amber-300" />
            <div className="space-y-0.5">
              <p className="font-medium">
                {t("commerce.org.subscription.pending_title", {
                  plan: pending?.plan.name ?? "—",
                })}
              </p>
              <p className="text-muted-foreground">
                {t("commerce.org.subscription.pending_hint", {
                  date: date(s.period_end, undefined, locale),
                })}
              </p>
            </div>
          </div>
        ) : null}

        {canWrite ? (
          <div className="flex flex-wrap gap-2 border-t pt-4">
            {canChange ? (
              <Button
                type="button"
                size="sm"
                onClick={() => setDialog("change")}
              >
                <ArrowRightLeft className="size-4" />
                {t("commerce.org.change_plan.title")}
              </Button>
            ) : null}
            {terminal ? (
              <Button
                type="button"
                size="sm"
                onClick={() => setDialog("assign")}
                disabled={!plans.data}
              >
                <PlusCircle className="size-4" />
                {t("billing.subscription.assign")}
              </Button>
            ) : null}
            {s.status === "trialing" ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setAction({ kind: "extend_trial" })}
              >
                <CalendarPlus className="size-4" />
                {t("billing.subscription.actions.extend_trial")}
              </Button>
            ) : null}
            {s.status === "active" && !s.cancel_at_period_end ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setAction({ kind: "free_month" })}
              >
                <Gift className="size-4" />
                {t("billing.subscription.actions.free_month")}
              </Button>
            ) : null}
            {(ACTIONS[s.status] ?? []).map((a) => (
              <Button
                key={a.key}
                type="button"
                size="sm"
                variant="outline"
                className={
                  a.to === "canceled" || a.to === "suspended"
                    ? "text-destructive"
                    : undefined
                }
                onClick={() =>
                  setAction({
                    kind: "transition",
                    to: a.to,
                    titleKey: `billing.subscription.actions.${a.key}`,
                  })
                }
              >
                <a.icon className="size-4" />
                {t(`billing.subscription.actions.${a.key}`)}
              </Button>
            ))}
          </div>
        ) : null}
      </div>
      {action ? (
        <ActionDialog
          key={JSON.stringify(action)}
          action={action}
          subscription={s}
          organization={organization}
          onClose={() => setAction(null)}
        />
      ) : null}
      {dialog === "change" && plans.data ? (
        <ChangePlanDialog
          organization={organization}
          subscription={s}
          plans={plans.data.items}
          onClose={() => setDialog(null)}
        />
      ) : null}
      {assignDialog}
    </EntitySectionCard>
  );
}

// System reasons of subscription transitions are stored in English by the
// lifecycle engine; known ones are translated, free text (admin notes) is
// shown as written.
const TRANSITION_REASONS: Record<string, string> = {
  "Subscription created": "subscription_created",
  "Organization signup trial": "signup_trial",
  "Organization signup awaiting first payment": "signup_awaiting_payment",
  "Invoice payment settled": "invoice_payment_settled",
  "Invoice payment": "invoice_payment",
  "Trial payment settled": "trial_payment_settled",
  "Zero balance invoice settled": "zero_balance_settled",
  "Invoice overdue": "invoice_overdue",
  "Payment grace period": "payment_grace",
  "Immediate cancellation": "immediate_cancellation",
  "Scheduled lifecycle deadline": "scheduled_deadline",
};
const PLAN_CHANGE_PREFIX = "Platform plan change: ";

function transitionReason(
  t: (key: string) => string,
  reason: string | null | undefined,
) {
  if (reason?.startsWith(PLAN_CHANGE_PREFIX)) {
    return reason.slice(PLAN_CHANGE_PREFIX.length);
  }
  const slug = reason ? TRANSITION_REASONS[reason] : undefined;
  return slug ? t(`billing.subscription.reason.${slug}`) : (reason ?? "");
}

function historyTitle(t: (key: string) => string, h: BillingHistoryEntry) {
  if (h.reason?.startsWith(PLAN_CHANGE_PREFIX)) {
    return t("commerce.org.subscription.history_plan_change");
  }
  // The first transition has no previous status (subscription created):
  // show only where it landed.
  return h.from_status && h.from_status !== h.to_status
    ? `${t(`billing.status.subscription.${h.from_status}`)} → ${t(`billing.status.subscription.${h.to_status}`)}`
    : t(`billing.status.subscription.${h.to_status}`);
}

function HistoryCard({ organization }: { organization: string }) {
  const { t, locale } = useLocale();
  const sub = useOrgSubscription(organization);
  const history = useSubscriptionHistory(organization, sub.data?.uuid);
  const items = history.data ?? [];
  return (
    <EntitySectionCard
      title={t("billing.subscription.history")}
      badge={history.data ? items.length : undefined}
    >
      {history.isLoading || sub.isLoading ? (
        <Skeleton className="h-32 w-full" />
      ) : items.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("commerce.org.subscription.history_empty")}
        </p>
      ) : (
        <Timeline
          items={items.map((h, i) => ({
            id: h.uuid ?? String(i),
            title: historyTitle(t, h),
            description: transitionReason(t, h.reason),
            meta: h.created_at ? (
              <time
                dateTime={h.created_at}
                title={datetime(h.created_at, undefined, locale)}
              >
                {relativeDatetime(h.created_at, locale)}
              </time>
            ) : null,
          }))}
        />
      )}
    </EntitySectionCard>
  );
}

function display(feature: string, value: number, locale: "tr" | "en") {
  return feature === "storage.bytes"
    ? formatBytes(value, locale)
    : formatCount(value, locale);
}

function UsageRow({
  value,
  label,
  used,
  canWrite,
  onEdit,
}: {
  value: EntitlementValue;
  label: string;
  used: number;
  canWrite: boolean;
  onEdit: () => void;
}) {
  const { t, locale } = useLocale();
  const isLimit = value.kind === "limit";
  const pct =
    !isLimit || value.unlimited
      ? 0
      : value.limit > 0
        ? Math.min(100, Math.round((used / value.limit) * 100))
        : used > 0
          ? 100
          : 0;
  const over = isLimit && !value.unlimited && used > value.limit;
  const warn = isLimit && !value.unlimited && pct >= (value.warn_percent ?? 80);
  return (
    <li className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center">
      <div className="min-w-0 flex-1 space-y-1.5">
        <div className="flex items-baseline justify-between gap-3">
          <span className="truncate text-sm font-medium">{label}</span>
          {isLimit ? (
            <span className="text-muted-foreground shrink-0 text-xs tabular-nums">
              {display(value.feature, used, locale)} /{" "}
              {value.unlimited
                ? t("billing.common.unlimited")
                : display(value.feature, value.limit, locale)}
            </span>
          ) : (
            <Badge variant={value.enabled ? "success" : "outline"}>
              {value.enabled
                ? t("billing.entitlements.on")
                : t("billing.entitlements.off")}
            </Badge>
          )}
        </div>
        {isLimit ? (
          <>
            <div
              role="meter"
              aria-label={label}
              aria-valuemin={0}
              aria-valuemax={value.unlimited ? undefined : value.limit}
              aria-valuenow={used}
              className="bg-muted h-1.5 overflow-hidden rounded-full"
            >
              <div
                className={cn(
                  "h-full rounded-full transition-[width]",
                  over
                    ? "bg-destructive"
                    : warn
                      ? "bg-amber-500"
                      : "bg-primary",
                  value.unlimited && "bg-primary/30",
                )}
                style={{ width: `${value.unlimited ? 100 : pct}%` }}
              />
            </div>
            <p className="text-muted-foreground text-xs">
              {value.period === "month"
                ? t("billing.usage.monthly")
                : t("billing.usage.current")}
              {over
                ? ` · ${t(value.enforcement === "soft" ? "billing.usage.over_soft" : "billing.usage.over_hard")}`
                : ""}
            </p>
          </>
        ) : null}
      </div>
      {canWrite ? (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="self-start sm:self-center"
          onClick={onEdit}
        >
          <SlidersHorizontal className="size-3.5" />
          {isLimit
            ? t("commerce.org.usage.override")
            : t("commerce.org.usage.override_feature")}
        </Button>
      ) : null}
    </li>
  );
}

function UsageCard({
  organization,
  canWrite,
}: {
  organization: string;
  canWrite: boolean;
}) {
  const { t, locale } = useLocale();
  const ents = useOrgEntitlements(organization);
  const features = useBillingFeatures();
  const usage = useOrgUsage(organization);
  const [editing, setEditing] = useState<EntitlementValue | null>(null);
  const labels = useMemo(
    () =>
      new Map(
        (features.data ?? []).map((f) => [f.key, featureLabel(f, locale)]),
      ),
    [features.data, locale],
  );
  const used = useMemo(() => {
    // Rows come per period, newest (current) first: keep the first per key.
    const map = new Map<string, number>();
    for (const u of usage.data ?? [])
      if (!map.has(u.key)) map.set(u.key, u.quantity);
    return map;
  }, [usage.data]);
  const all = Object.values(ents.data?.values ?? {});
  const byLabel = (a: EntitlementValue, b: EntitlementValue) =>
    (labels.get(a.feature) ?? a.feature).localeCompare(
      labels.get(b.feature) ?? b.feature,
      locale,
    );
  const limits = all.filter((v) => v.kind === "limit").sort(byLabel);
  const toggles = all.filter((v) => v.kind === "boolean").sort(byLabel);
  const label = (v: EntitlementValue) => labels.get(v.feature) ?? v.feature;

  if (ents.isLoading) return <Skeleton className="h-64 w-full" />;
  if (ents.isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void ents.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

  return (
    <div className="space-y-6">
      {ents.data && !ents.data.writable ? (
        <p className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm">
          {t("billing.entitlements.read_only")}
        </p>
      ) : null}
      <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
        <EntitySectionCard
          title={t("commerce.org.usage.limits_title")}
          description={t("commerce.org.usage.limits_description")}
          contentClassName="px-0"
        >
          {limits.length === 0 ? (
            <p className="text-muted-foreground px-6 text-sm">
              {t("commerce.org.usage.empty")}
            </p>
          ) : (
            <ul className="divide-y border-y">
              {limits.map((v) => (
                <UsageRow
                  key={v.feature}
                  value={v}
                  label={label(v)}
                  used={used.get(v.feature) ?? 0}
                  canWrite={canWrite}
                  onEdit={() => setEditing(v)}
                />
              ))}
            </ul>
          )}
        </EntitySectionCard>
        <EntitySectionCard
          title={t("commerce.org.usage.features_title")}
          description={t("commerce.org.usage.features_description")}
          contentClassName="px-0"
        >
          {toggles.length === 0 ? (
            <p className="text-muted-foreground px-6 text-sm">
              {t("commerce.org.usage.empty")}
            </p>
          ) : (
            <ul className="divide-y border-y">
              {toggles.map((v) => (
                <UsageRow
                  key={v.feature}
                  value={v}
                  label={label(v)}
                  used={0}
                  canWrite={canWrite}
                  onEdit={() => setEditing(v)}
                />
              ))}
            </ul>
          )}
        </EntitySectionCard>
      </div>
      {canWrite ? (
        <p className="text-muted-foreground text-xs">
          {t("commerce.org.usage.override_hint")}
        </p>
      ) : null}
      {editing ? (
        <OverrideDialog
          key={editing.feature}
          organization={organization}
          value={editing}
          label={label(editing)}
          onClose={() => setEditing(null)}
        />
      ) : null}
    </div>
  );
}

/**
 * Organization page → "Abonelik" (CF-39, CF-143): current plan with price,
 * period and trial countdown, plan change (now / at period end), manual
 * operations (assign, extend trial, free month, pause / resume / suspend /
 * cancel, every one with a reason, step-up and audit) and the state history.
 */
export function OrganizationSubscriptionTab({
  organizationUuid,
}: {
  organizationUuid: string;
}) {
  const { can } = usePermission();
  return (
    <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
      <SubscriptionCard
        organization={organizationUuid}
        canWrite={can(permissions.billing.subscriptionsWrite)}
      />
      <HistoryCard organization={organizationUuid} />
    </div>
  );
}

/**
 * Organization page → "Kullanım": usage against every limit, feature
 * switches and one way to set an organization-specific value per row.
 */
export function OrganizationUsageTab({
  organizationUuid,
}: {
  organizationUuid: string;
}) {
  const { can } = usePermission();
  return (
    <UsageCard
      organization={organizationUuid}
      canWrite={can(permissions.billing.entitlementsWrite)}
    />
  );
}

/** Subscription and usage on one tab (kept for existing imports). */
export function OrganizationBillingTab({
  organizationUuid,
}: {
  organizationUuid: string;
}) {
  const { can } = usePermission();
  return (
    <div className="space-y-6">
      {can(permissions.billing.subscriptionsRead) ? (
        <OrganizationSubscriptionTab organizationUuid={organizationUuid} />
      ) : null}
      {can(permissions.billing.entitlementsRead) ? (
        <OrganizationUsageTab organizationUuid={organizationUuid} />
      ) : null}
    </div>
  );
}
