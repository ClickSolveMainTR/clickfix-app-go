"use client";

import { useQuery } from "@tanstack/react-query";
import { format, subDays } from "date-fns";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import { SettingsPageHeader } from "@/features/settings/components/settings-page-header";
import {
  marketingService,
  type MarketingSources,
} from "@/features/settings/services/marketing.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Row = { key: string; label: string; detail?: string; count: number };

function group(
  rows: MarketingSources["rows"],
  keyOf: (row: MarketingSources["rows"][number]) => [string, string?],
): Row[] {
  const out = new Map<string, Row>();
  for (const row of rows) {
    const [label, detail] = keyOf(row);
    const key = `${label}\u0000${detail ?? ""}`;
    const current = out.get(key) ?? { key, label, detail, count: 0 };
    current.count += row.organizations;
    out.set(key, current);
  }
  return [...out.values()].sort((a, b) => b.count - a.count);
}

/**
 * "Kaynaklar" (CF-144): organizations opened in a date range by the source,
 * medium and campaign they signed up from.
 */
export function MarketingSourcesPage() {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.organizations.read);
  const today = new Date();
  const [from, setFrom] = useState(format(subDays(today, 89), "yyyy-MM-dd"));
  const [to, setTo] = useState(format(today, "yyyy-MM-dd"));

  const query = useQuery({
    queryKey: ["platform", "marketing", "sources", from, to],
    queryFn: () => marketingService.sources(from, to),
    enabled: canRead && Boolean(from && to) && from <= to,
  });

  const direct = t("settings.marketing.sources_direct");
  const tables = useMemo(() => {
    const rows = query.data?.rows ?? [];
    return {
      sources: group(rows, (r) =>
        r.source ? [r.source, r.medium || undefined] : [direct],
      ),
      campaigns: group(
        rows.filter((r) => r.campaign),
        (r) => [r.campaign, r.source],
      ),
    };
  }, [query.data, direct]);

  if (!canRead) {
    return (
      <ErrorState
        title={t("common.error_forbidden")}
        description={t("settings.forbidden")}
      />
    );
  }

  const total = query.data?.total ?? 0;
  const attributed =
    query.data?.rows.reduce(
      (n, r) => n + (r.source ? r.organizations : 0),
      0,
    ) ?? 0;
  const number = new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US");
  const percent = new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    style: "percent",
    maximumFractionDigits: 1,
  });

  const table = (caption: string, head: [string, string], rows: Row[]) => (
    <section className="space-y-2">
      <h2 className="text-sm font-semibold">{caption}</h2>
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-sm">
          <thead className="text-muted-foreground border-b text-left text-xs">
            <tr>
              <th scope="col" className="px-3 py-2 font-medium">
                {head[0]}
              </th>
              <th scope="col" className="px-3 py-2 font-medium">
                {head[1]}
              </th>
              <th scope="col" className="px-3 py-2 text-right font-medium">
                {t("settings.marketing.sources_count")}
              </th>
              <th scope="col" className="px-3 py-2 text-right font-medium">
                {t("settings.marketing.sources_share")}
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.key} className="border-b last:border-0">
                <th scope="row" className="px-3 py-2 text-left font-medium">
                  {row.label}
                </th>
                <td className="text-muted-foreground px-3 py-2">
                  {row.detail ?? "—"}
                </td>
                <td className="px-3 py-2 text-right tabular-nums">
                  {number.format(row.count)}
                </td>
                <td className="px-3 py-2 text-right tabular-nums">
                  {total ? percent.format(row.count / total) : "—"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={t("settings.marketing.sources_title")}
        description={t("settings.marketing.sources_description")}
      />
      <div className="flex flex-wrap items-end gap-4">
        <div className="space-y-1.5">
          <Label htmlFor="sources-from">
            {t("settings.marketing.sources_from")}
          </Label>
          <DatePicker id="sources-from" value={from} onChange={setFrom} />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="sources-to">
            {t("settings.marketing.sources_to")}
          </Label>
          <DatePicker id="sources-to" value={to} onChange={setTo} />
        </div>
      </div>

      {query.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : total === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("settings.marketing.sources_empty")}
        </p>
      ) : (
        <>
          <dl className="grid gap-3 sm:grid-cols-2">
            {[
              [t("settings.marketing.sources_total"), number.format(total)],
              [
                t("settings.marketing.sources_attributed"),
                `${number.format(attributed)} (${percent.format(attributed / total)})`,
              ],
            ].map(([label, value]) => (
              <div key={label} className="rounded-lg border p-4">
                <dt className="text-muted-foreground text-xs">{label}</dt>
                <dd className="mt-1 text-2xl font-semibold tabular-nums">
                  {value}
                </dd>
              </div>
            ))}
          </dl>
          {table(
            t("settings.marketing.sources_by_source"),
            [
              t("settings.marketing.sources_source"),
              t("settings.marketing.sources_medium"),
            ],
            tables.sources,
          )}
          {tables.campaigns.length > 0
            ? table(
                t("settings.marketing.sources_by_campaign"),
                [
                  t("settings.marketing.sources_campaign"),
                  t("settings.marketing.sources_source"),
                ],
                tables.campaigns,
              )
            : null}
        </>
      )}
    </div>
  );
}
