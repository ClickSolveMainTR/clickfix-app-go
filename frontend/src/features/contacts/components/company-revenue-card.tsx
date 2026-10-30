"use client";

import { useState } from "react";
import { ChevronDown, Pencil, Wallet } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { formatMinor, parseMajorToMinor } from "@/features/billing/lib/money";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  useCompanyRevenue,
  useCompanyRevenueMutations,
} from "../hooks/use-company-revenue";
import { useContactsScope } from "../hooks/use-contacts";
import type { CompanyRevenue } from "../services/revenue.service";

const STATUSES = [
  "none",
  "trialing",
  "active",
  "past_due",
  "canceled",
] as const;
const CURRENCIES = ["TRY", "USD", "EUR", "GBP"];

export type CompanyRevenueCardProps = {
  companyUuid: string;
  className?: string;
};

/**
 * Customer revenue of a company (CF-86): MRR, plan, billing status, renewal
 * date and lifetime value, with a manual editor and the change history.
 * Used on the companies page and the account screen (CF-85).
 */
export function CompanyRevenueCard({
  companyUuid,
  className,
}: CompanyRevenueCardProps) {
  const { t, locale } = useLocale();
  const scope = useContactsScope();
  const q = useCompanyRevenue(companyUuid);
  const [editing, setEditing] = useState(false);
  const rev = q.data?.revenue ?? null;
  const history = q.data?.history ?? [];
  const money = (minor: number, currency: string) =>
    formatMinor(minor, currency, locale, { compact: true });
  const date = (iso: string | null) =>
    iso
      ? new Date(iso).toLocaleDateString(locale === "tr" ? "tr-TR" : "en-US")
      : "—";

  return (
    <section className={cn("bg-card rounded-xl border p-4", className)}>
      <header className="mb-3 flex items-center gap-2">
        <Wallet className="text-muted-foreground size-4" />
        <h3 className="text-sm font-medium">{t("contacts.revenue.title")}</h3>
        {rev && (
          <span className="text-muted-foreground text-[11px]">
            {t(`contacts.revenue.source.${rev.source}`)}
          </span>
        )}
        {scope.canWrite && !editing && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="ml-auto h-7"
            onClick={() => setEditing(true)}
          >
            <Pencil className="size-3.5" />
            {t("contacts.revenue.edit")}
          </Button>
        )}
      </header>
      {q.isLoading ? (
        <Skeleton className="h-16 rounded-lg" />
      ) : editing ? (
        <RevenueForm
          companyUuid={companyUuid}
          current={rev}
          onDone={() => setEditing(false)}
        />
      ) : !rev ? (
        <p className="text-muted-foreground text-sm">
          {t("contacts.revenue.empty")}
        </p>
      ) : (
        <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
          <Item label={t("contacts.revenue.fields.mrr")}>
            <span className="font-semibold tabular-nums">
              {money(rev.mrr_minor, rev.currency)}
            </span>
          </Item>
          <Item label={t("contacts.revenue.fields.plan")}>
            {rev.plan || "—"}
          </Item>
          <Item label={t("contacts.revenue.fields.billing_status")}>
            <span
              className={cn(
                "rounded-md px-1.5 py-0.5 text-xs font-medium",
                rev.billing_status === "active" &&
                  "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
                rev.billing_status === "past_due" &&
                  "bg-amber-500/10 text-amber-700 dark:text-amber-400",
                rev.billing_status === "canceled" &&
                  "bg-destructive/10 text-destructive",
                (rev.billing_status === "none" ||
                  rev.billing_status === "trialing") &&
                  "bg-muted",
              )}
            >
              {t(`contacts.revenue.status.${rev.billing_status}`)}
            </span>
          </Item>
          <Item label={t("contacts.revenue.fields.renewal_at")}>
            {date(rev.renewal_at)}
          </Item>
          <Item label={t("contacts.revenue.fields.lifetime_value")}>
            <span className="tabular-nums">
              {money(rev.lifetime_value_minor, rev.currency)}
            </span>
          </Item>
          <Item label={t("contacts.revenue.fields.updated_at")}>
            {date(rev.updated_at)}
          </Item>
        </dl>
      )}
      {history.length > 0 && !editing && (
        <Collapsible className="mt-3">
          <CollapsibleTrigger className="text-muted-foreground hover:text-foreground group flex items-center gap-1 text-xs">
            <ChevronDown className="size-3.5 transition-transform group-data-[state=open]:rotate-180" />
            {t("contacts.revenue.history", { count: history.length })}
          </CollapsibleTrigger>
          <CollapsibleContent>
            <ul className="mt-2 grid gap-1.5 text-xs">
              {history.map((h, i) => (
                <li
                  key={`${h.created_at}-${i}`}
                  className="flex flex-wrap items-center gap-x-2"
                >
                  <span className="text-muted-foreground w-20 shrink-0">
                    {date(h.created_at)}
                  </span>
                  <span className="tabular-nums">
                    {money(h.mrr_minor, h.currency)}
                  </span>
                  {h.plan && <span>· {h.plan}</span>}
                  <span>
                    · {t(`contacts.revenue.status.${h.billing_status}`)}
                  </span>
                  <span className="text-muted-foreground ml-auto">
                    {h.changed_by?.name ??
                      t(`contacts.revenue.source.${h.source}`)}
                  </span>
                </li>
              ))}
            </ul>
          </CollapsibleContent>
        </Collapsible>
      )}
    </section>
  );
}

function Item({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="min-w-0">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="truncate">{children}</dd>
    </div>
  );
}

function minorToInput(minor: number | undefined) {
  if (minor == null) return "";
  return (minor / 100).toFixed(2);
}

function RevenueForm({
  companyUuid,
  current,
  onDone,
}: {
  companyUuid: string;
  current: CompanyRevenue | null;
  onDone: () => void;
}) {
  const { t } = useLocale();
  const m = useCompanyRevenueMutations(companyUuid);
  const [form, setForm] = useState({
    mrr: minorToInput(current?.mrr_minor),
    ltv: minorToInput(current?.lifetime_value_minor),
    currency: current?.currency ?? "TRY",
    plan: current?.plan ?? "",
    status: (current?.billing_status ?? "active") as (typeof STATUSES)[number],
    renewal: current?.renewal_at?.slice(0, 10) ?? "",
  });
  const mrr = form.mrr.trim() === "" ? 0 : parseMajorToMinor(form.mrr);
  const ltv = form.ltv.trim() === "" ? 0 : parseMajorToMinor(form.ltv);
  const valid = mrr !== null && ltv !== null;
  const set = <K extends keyof typeof form>(k: K, v: (typeof form)[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  return (
    <form
      className="grid gap-3"
      onSubmit={(e) => {
        e.preventDefault();
        if (!valid) return;
        m.save.mutate(
          {
            mrr_minor: mrr,
            lifetime_value_minor: ltv,
            currency: form.currency,
            plan: form.plan.trim(),
            billing_status: form.status,
            renewal_at: form.renewal || null,
          },
          { onSuccess: onDone },
        );
      }}
    >
      <div className="grid grid-cols-[1fr_6rem] gap-2">
        <div className="grid gap-1.5">
          <Label htmlFor="rev-mrr">{t("contacts.revenue.fields.mrr")}</Label>
          <Input
            id="rev-mrr"
            inputMode="decimal"
            value={form.mrr}
            aria-invalid={mrr === null}
            onChange={(e) => set("mrr", e.target.value)}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="rev-currency">
            {t("contacts.revenue.fields.currency")}
          </Label>
          <Select
            value={form.currency}
            onValueChange={(v) => set("currency", v)}
          >
            <SelectTrigger id="rev-currency">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {CURRENCIES.map((c) => (
                <SelectItem key={c} value={c}>
                  {c}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-2">
        <div className="grid gap-1.5">
          <Label htmlFor="rev-plan">{t("contacts.revenue.fields.plan")}</Label>
          <Input
            id="rev-plan"
            maxLength={120}
            value={form.plan}
            onChange={(e) => set("plan", e.target.value)}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="rev-status">
            {t("contacts.revenue.fields.billing_status")}
          </Label>
          <Select
            value={form.status}
            onValueChange={(v) => set("status", v as (typeof STATUSES)[number])}
          >
            <SelectTrigger id="rev-status">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {STATUSES.map((s) => (
                <SelectItem key={s} value={s}>
                  {t(`contacts.revenue.status.${s}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-2">
        <div className="grid gap-1.5">
          <Label htmlFor="rev-renewal">
            {t("contacts.revenue.fields.renewal_at")}
          </Label>
          <DatePicker
            id="rev-renewal"
            value={form.renewal}
            onChange={(v) => set("renewal", v)}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="rev-ltv">
            {t("contacts.revenue.fields.lifetime_value")}
          </Label>
          <Input
            id="rev-ltv"
            inputMode="decimal"
            value={form.ltv}
            aria-invalid={ltv === null}
            onChange={(e) => set("ltv", e.target.value)}
          />
        </div>
      </div>
      <div className="flex items-center gap-2">
        {current && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="text-destructive"
            disabled={m.clear.isPending}
            onClick={() => m.clear.mutate(undefined, { onSuccess: onDone })}
          >
            {t("contacts.revenue.clear")}
          </Button>
        )}
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="ml-auto"
          onClick={onDone}
        >
          {t("contacts.revenue.cancel")}
        </Button>
        <Button type="submit" size="sm" disabled={!valid || m.save.isPending}>
          {t("contacts.save")}
        </Button>
      </div>
    </form>
  );
}
