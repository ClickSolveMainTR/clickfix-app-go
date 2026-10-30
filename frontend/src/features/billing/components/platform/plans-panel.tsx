"use client";

import { Plus } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { z } from "zod";

import {
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
  FormActions,
} from "@/components/forms";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BillingTable,
  type BillingColumn,
} from "@/features/billing/components/billing-table";
import {
  useCreatePlan,
  usePlans,
} from "@/features/billing/hooks/use-platform-billing";
import { formatMinor } from "@/features/billing/lib/money";
import { latestPublished, latestDraft } from "@/features/billing/lib/plans";
import type { BillingPlan } from "@/features/billing/types";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const LIMIT = 50;
export const PLAN_VISIBILITIES = ["public", "hidden", "custom"] as const;

export function createPlanSchema(t: (key: string) => string, create: boolean) {
  return z.object({
    code: create
      ? z
          .string()
          .trim()
          .regex(/^[a-z][a-z0-9_-]{0,79}$/, t("billing.validation.plan_code"))
      : z.string(),
    name: z.string().trim().min(1, t("billing.validation.required")).max(200),
    description: z.string().max(10000),
    visibility: z.enum(PLAN_VISIBILITIES),
    sort_order: z.coerce.number().int().min(0).max(10000),
  });
}
export type PlanFormValues = z.infer<ReturnType<typeof createPlanSchema>>;

function CreatePlanDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const create = useCreatePlan();
  const schema = useMemo(() => createPlanSchema(t, true), [t]);
  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("billing.plans.create")}</DialogTitle>
          <DialogDescription>
            {t("billing.plans.create_hint")}
          </DialogDescription>
        </DialogHeader>
        <AppForm<PlanFormValues>
          className="space-y-4"
          schema={schema}
          defaultValues={{
            code: "",
            name: "",
            description: "",
            visibility: "public",
            sort_order: 0,
          }}
          onSubmit={async (values) => {
            const plan = await create.mutateAsync({
              ...values,
              code: values.code.trim(),
              name: values.name.trim(),
            });
            onClose();
            router.push(routes.platform.billing.plan(plan.uuid));
          }}
        >
          {(form) => (
            <PlanFields
              create
              submitting={form.formState.isSubmitting}
              onCancel={onClose}
            />
          )}
        </AppForm>
      </DialogContent>
    </Dialog>
  );
}

export function PlanFields({
  create,
  submitting,
  onCancel,
  disabled,
}: {
  create: boolean;
  submitting: boolean;
  onCancel?: () => void;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  return (
    <>
      <div className="grid gap-4 sm:grid-cols-2">
        <AppInput
          name="name"
          label={t("billing.plans.fields.name")}
          disabled={disabled}
        />
        <AppInput
          name="code"
          label={t("billing.plans.fields.code")}
          description={create ? t("billing.plans.fields.code_hint") : undefined}
          disabled={!create || disabled}
          className="font-mono"
        />
      </div>
      <AppTextarea
        name="description"
        label={t("billing.plans.fields.description")}
        rows={3}
        disabled={disabled}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <AppSelect
          name="visibility"
          label={t("billing.plans.fields.visibility")}
          disabled={disabled}
          options={PLAN_VISIBILITIES.map((v) => ({
            value: v,
            label: t(`billing.plans.visibility.${v}`),
          }))}
        />
        <AppInput
          name="sort_order"
          type="number"
          min={0}
          label={t("billing.plans.fields.sort_order")}
          disabled={disabled}
        />
      </div>
      {disabled ? null : (
        <FormActions>
          {onCancel ? (
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("common.cancel")}
            </Button>
          ) : null}
          <Button type="submit" disabled={submitting}>
            {create ? t("billing.plans.create") : t("billing.common.save")}
          </Button>
        </FormActions>
      )}
    </>
  );
}

/** Plan catalog: published version, draft in progress, TRY prices, trial. */
export function PlansPanel() {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const [offset, setOffset] = useState(0);
  const [creating, setCreating] = useState(false);
  const query = usePlans({ limit: LIMIT, offset });

  const columns: BillingColumn<BillingPlan>[] = [
    {
      key: "name",
      header: t("billing.plans.columns.name"),
      cell: (row) => (
        <span className="flex flex-col">
          <span className="font-medium">{row.name}</span>
          <code className="text-muted-foreground text-xs">{row.code}</code>
        </span>
      ),
    },
    {
      key: "visibility",
      header: t("billing.plans.columns.visibility"),
      cell: (row) =>
        row.archived_at ? (
          <Badge variant="outline">{t("billing.plans.archived")}</Badge>
        ) : (
          <Badge
            variant={row.visibility === "public" ? "success" : "secondary"}
          >
            {t(`billing.plans.visibility.${row.visibility ?? "public"}`)}
          </Badge>
        ),
    },
    {
      key: "version",
      header: t("billing.plans.columns.version"),
      cell: (row) => {
        const published = latestPublished(row);
        const draft = latestDraft(row);
        return (
          <span className="flex flex-wrap items-center gap-1.5">
            {published ? (
              <span className="flex flex-col">
                <span className="tabular-nums">
                  {t("credit_products.plans.live_version", {
                    version: published.version,
                  })}
                </span>
                {published.published_at ? (
                  <span className="text-muted-foreground text-xs">
                    {date(published.published_at, undefined, locale)}
                  </span>
                ) : null}
              </span>
            ) : (
              <span className="text-muted-foreground">
                {t("billing.plans.unpublished")}
              </span>
            )}
            {draft ? (
              <Badge variant="warning">
                {t("billing.plans.draft_badge", { version: draft.version })}
              </Badge>
            ) : null}
          </span>
        );
      },
    },
    ...(["monthly", "yearly"] as const).map(
      (interval): BillingColumn<BillingPlan> => ({
        key: interval,
        header: t(`credit_products.plans.price_${interval}`),
        align: "end",
        cell: (row) => {
          const price = latestPublished(row)?.prices.find(
            (p) => p.currency === "TRY" && p.interval === interval,
          );
          if (!price) return <span className="text-muted-foreground">—</span>;
          return (
            <span className="flex flex-col items-end">
              <span>{formatMinor(price.amount_minor, "TRY", locale)}</span>
              <span className="text-muted-foreground text-xs">
                {price.tax_inclusive
                  ? t("credit_products.price.vat_included")
                  : t("credit_products.price.vat_excluded")}
              </span>
            </span>
          );
        },
      }),
    ),
    {
      key: "trial",
      header: t("billing.plans.columns.trial"),
      align: "end",
      cell: (row) => {
        const v = latestPublished(row);
        return v?.trial_days ? (
          t("billing.plans.trial_days", { days: v.trial_days })
        ) : (
          <span className="text-muted-foreground">
            {t("credit_products.plans.no_trial")}
          </span>
        );
      },
    },
  ];

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-muted-foreground text-sm">
          {t("billing.plans.hint")}
        </p>
        {can(permissions.billing.plansWrite) ? (
          <Button type="button" size="sm" onClick={() => setCreating(true)}>
            <Plus className="size-4" />
            {t("billing.plans.create")}
          </Button>
        ) : null}
      </div>
      <BillingTable
        caption={t("billing.admin.tabs.plans")}
        columns={columns}
        rows={query.data?.items}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("billing.plans.empty")}
        emptyDescription={t("billing.plans.empty_hint")}
        onRowClick={(row) =>
          router.push(routes.platform.billing.plan(row.uuid))
        }
        page={
          query.data
            ? {
                total: query.data.total,
                limit: LIMIT,
                offset,
                onOffsetChange: setOffset,
              }
            : undefined
        }
      />
      <CreatePlanDialog open={creating} onClose={() => setCreating(false)} />
    </div>
  );
}
