"use client";

import {
  Archive,
  ArchiveRestore,
  Bot,
  Pencil,
  Plus,
  Video,
  type LucideIcon,
} from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  BillingTable,
  type BillingColumn,
} from "@/features/billing/components/billing-table";
import {
  CREDIT_UNITS,
  formatUnitPrice,
} from "@/features/billing/components/platform/catalog/catalog-lib";
import { CreditProductDialog } from "@/features/billing/components/platform/catalog/credit-product-dialog";
import {
  useCreditProducts,
  useSaveCreditProduct,
} from "@/features/billing/hooks/use-credit-products";
import { usePlans } from "@/features/billing/hooks/use-platform-billing";
import { formatCount, formatMinor } from "@/features/billing/lib/money";
import type {
  CreditProduct,
  CreditProductUnit,
} from "@/features/billing/services/credit-products.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const UNIT_ICON: Record<CreditProductUnit, LucideIcon> = {
  ai_credits: Bot,
  video_minutes: Video,
};

function byCatalogOrder(a: CreditProduct, b: CreditProduct) {
  if (a.active !== b.active) return a.active ? -1 : 1;
  if (a.sort_order !== b.sort_order) return a.sort_order - b.sort_order;
  return a.credits - b.credits;
}

/**
 * Credit products (CF-143): AI credit and video minute packs tenants buy on
 * top of their plan. Grouped by metered unit; archived packs stay listed so
 * past purchases remain explainable.
 */
export function CreditProductsPanel() {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.billing.subscriptionsWrite);
  const query = useCreditProducts();
  const plans = usePlans({ limit: 100, offset: 0 });
  const save = useSaveCreditProduct();
  const [editing, setEditing] = useState<{
    product: CreditProduct | null;
    unit: CreditProductUnit;
  } | null>(null);
  const [toggling, setToggling] = useState<CreditProduct | null>(null);

  const planNames = useMemo(
    () => new Map((plans.data?.items ?? []).map((p) => [p.uuid, p.name])),
    [plans.data],
  );

  const groups = useMemo(
    () =>
      CREDIT_UNITS.map((unit) => ({
        unit,
        rows: (query.data ?? [])
          .filter((p) => p.unit === unit)
          .sort(byCatalogOrder),
      })),
    [query.data],
  );

  const columns = (unit: CreditProductUnit): BillingColumn<CreditProduct>[] => [
    {
      key: "name",
      header: t("credit_products.columns.product"),
      cell: (row) => (
        <span className="flex max-w-72 flex-col py-1.5">
          <span className="font-medium">{row.name}</span>
          <span className="text-muted-foreground truncate text-xs">
            <code>{row.code}</code>
            {row.description ? ` · ${row.description}` : null}
          </span>
        </span>
      ),
    },
    {
      key: "quantity",
      header: t("credit_products.columns.quantity"),
      align: "end",
      cell: (row) =>
        t(`credit_products.quantity.${unit}`, {
          count: formatCount(row.credits, locale),
        }),
    },
    {
      key: "price",
      header: t("credit_products.columns.price"),
      align: "end",
      cell: (row) => (
        <span className="flex flex-col items-end">
          <span className="font-medium">
            {formatMinor(row.amount_minor, "TRY", locale)}
          </span>
          <span className="text-muted-foreground text-xs">
            {t(`credit_products.unit_price.${unit}`, {
              price: formatUnitPrice(row.amount_minor, row.credits, locale),
            })}
          </span>
        </span>
      ),
    },
    {
      key: "validity",
      header: t("credit_products.columns.validity"),
      cell: (row) =>
        row.expires_days == null
          ? t("credit_products.validity.none")
          : t("credit_products.validity.days", {
              days: formatCount(row.expires_days, locale),
            }),
    },
    {
      key: "plans",
      header: t("credit_products.columns.visibility"),
      cell: (row) =>
        row.plan_uuids.length === 0 ? (
          <span className="text-muted-foreground">
            {t("credit_products.visibility.all")}
          </span>
        ) : (
          <span className="flex max-w-56 flex-wrap gap-1">
            {row.plan_uuids.map((uuid) => (
              <Badge key={uuid} variant="secondary" className="font-normal">
                {planNames.get(uuid) ?? t("credit_products.visibility.unknown")}
              </Badge>
            ))}
          </span>
        ),
    },
    {
      key: "status",
      header: t("credit_products.columns.status"),
      cell: (row) =>
        row.active ? (
          <Badge variant="success">{t("credit_products.status.active")}</Badge>
        ) : (
          <Badge variant="outline">
            {t("credit_products.status.archived")}
          </Badge>
        ),
    },
    {
      key: "actions",
      header: <span className="sr-only">{t("billing.common.actions")}</span>,
      align: "end",
      cell: (row) =>
        canWrite ? (
          <span className="flex justify-end gap-1">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-8"
              onClick={() => setEditing({ product: row, unit })}
            >
              <Pencil className="size-3.5" />
              {t("billing.common.edit")}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-8"
              aria-label={
                row.active
                  ? t("credit_products.archive")
                  : t("credit_products.restore")
              }
              title={
                row.active
                  ? t("credit_products.archive")
                  : t("credit_products.restore")
              }
              onClick={() => setToggling(row)}
            >
              {row.active ? (
                <Archive className="size-3.5" />
              ) : (
                <ArchiveRestore className="size-3.5" />
              )}
            </Button>
          </span>
        ) : null,
    },
  ];

  if (query.isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1">
          <p className="text-muted-foreground text-sm">
            {t("credit_products.hint")}
          </p>
          <p className="text-muted-foreground/80 text-xs">
            {t("credit_products.not_sold_yet")}
          </p>
        </div>
        {canWrite ? (
          <Button
            type="button"
            size="sm"
            className="shrink-0 self-start"
            onClick={() => setEditing({ product: null, unit: "ai_credits" })}
          >
            <Plus className="size-4" />
            {t("credit_products.create")}
          </Button>
        ) : null}
      </div>

      {groups.map(({ unit, rows }) => {
        const Icon = UNIT_ICON[unit];
        const live = rows.filter((r) => r.active).length;
        return (
          <section
            key={unit}
            className="space-y-3"
            aria-labelledby={`cp-${unit}`}
          >
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-2.5">
                <span className="bg-muted text-muted-foreground flex size-8 items-center justify-center rounded-lg">
                  <Icon className="size-4" />
                </span>
                <div>
                  <h3 id={`cp-${unit}`} className="text-sm font-semibold">
                    {t(`credit_products.group.${unit}`)}
                  </h3>
                  <p className="text-muted-foreground text-xs">
                    {query.data
                      ? t("credit_products.group_count", {
                          live: formatCount(live, locale),
                          archived: formatCount(rows.length - live, locale),
                        })
                      : " "}
                  </p>
                </div>
              </div>
              {canWrite && rows.length > 0 ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setEditing({ product: null, unit })}
                >
                  <Plus className="size-4" />
                  {t(`credit_products.add.${unit}`)}
                </Button>
              ) : null}
            </div>
            <BillingTable
              caption={t(`credit_products.group.${unit}`)}
              columns={columns(unit)}
              rows={query.data ? rows : undefined}
              getRowId={(row) => row.uuid}
              isLoading={query.isLoading}
              emptyTitle={t(`credit_products.empty.${unit}`)}
              emptyDescription={t("credit_products.empty_hint")}
            />
          </section>
        );
      })}

      <CreditProductDialog
        open={editing !== null}
        product={editing?.product ?? null}
        defaultUnit={editing?.unit ?? "ai_credits"}
        onClose={() => setEditing(null)}
      />
      <ConfirmDialog
        open={toggling !== null}
        title={
          toggling?.active
            ? t("credit_products.archive_title", { name: toggling.name })
            : t("credit_products.restore_title", { name: toggling?.name ?? "" })
        }
        description={
          toggling?.active
            ? t("credit_products.archive_description")
            : t("credit_products.restore_description")
        }
        confirmLabel={
          toggling?.active
            ? t("credit_products.archive")
            : t("credit_products.restore")
        }
        variant={toggling?.active ? "destructive" : "default"}
        isPending={save.isPending}
        onCancel={() => setToggling(null)}
        onConfirm={() => {
          if (!toggling) return;
          const p = toggling;
          save.mutate(
            {
              uuid: p.uuid,
              toast: p.active ? "archived" : "restored",
              body: {
                unit: p.unit,
                code: p.code,
                name: p.name,
                description: p.description,
                credits: p.credits,
                amount_minor: p.amount_minor,
                plan_uuids: p.plan_uuids,
                sort_order: p.sort_order,
                active: !p.active,
                ...(p.expires_days == null
                  ? {}
                  : { expires_days: p.expires_days }),
              },
            },
            { onSettled: () => setToggling(null) },
          );
        }}
      />
    </div>
  );
}
