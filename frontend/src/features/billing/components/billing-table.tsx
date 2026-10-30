"use client";

import { ChevronLeft, ChevronRight } from "lucide-react";
import type { ReactNode } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export type BillingColumn<T> = {
  key: string;
  header: ReactNode;
  cell: (row: T) => ReactNode;
  align?: "start" | "end";
  className?: string;
};

type BillingTableProps<T> = {
  columns: BillingColumn<T>[];
  rows: T[] | undefined;
  getRowId: (row: T) => string;
  isLoading?: boolean;
  isError?: boolean;
  onRetry?: () => void;
  emptyTitle: string;
  emptyDescription?: string;
  onRowClick?: (row: T) => void;
  /** Server pagination; omit for short, unpaged lists. */
  page?: {
    total: number;
    limit: number;
    offset: number;
    onOffsetChange: (offset: number) => void;
  };
  caption?: string;
};

/**
 * Dense, calm list table for billing screens: tabular figures, end-aligned
 * money, skeleton rows while loading, the brand empty state and simple
 * previous/next paging over `limit`/`offset` envelopes.
 */
export function BillingTable<T>({
  columns,
  rows,
  getRowId,
  isLoading,
  isError,
  onRetry,
  emptyTitle,
  emptyDescription,
  onRowClick,
  page,
  caption,
}: BillingTableProps<T>) {
  const { t, locale } = useLocale();

  if (isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={onRetry}
        retryLabel={t("common.retry")}
      />
    );
  }

  if (!isLoading && rows && rows.length === 0) {
    return (
      <EmptyState
        className="py-10"
        title={emptyTitle}
        description={emptyDescription}
      />
    );
  }

  const nf = new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US");

  return (
    <div className="space-y-3">
      <div className="relative overflow-x-auto rounded-lg border">
        <table className="w-full min-w-[640px] text-sm">
          {caption ? <caption className="sr-only">{caption}</caption> : null}
          <thead className="bg-muted/40 text-muted-foreground text-xs">
            <tr>
              {columns.map((col) => (
                <th
                  key={col.key}
                  scope="col"
                  className={cn(
                    "h-9 px-3 font-medium whitespace-nowrap",
                    col.align === "end" ? "text-end" : "text-start",
                    col.className,
                  )}
                >
                  {col.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y">
            {isLoading && !rows
              ? Array.from({ length: 5 }, (_, i) => (
                  <tr key={`s${i}`}>
                    {columns.map((col) => (
                      <td key={col.key} className="h-11 px-3">
                        <Skeleton className="h-4 w-full max-w-32" />
                      </td>
                    ))}
                  </tr>
                ))
              : (rows ?? []).map((row) => (
                  <tr
                    key={getRowId(row)}
                    className={cn(
                      "hover:bg-muted/30 transition-colors",
                      onRowClick && "cursor-pointer",
                    )}
                    onClick={onRowClick ? () => onRowClick(row) : undefined}
                  >
                    {columns.map((col) => (
                      <td
                        key={col.key}
                        className={cn(
                          "h-11 px-3 align-middle",
                          col.align === "end"
                            ? "text-end tabular-nums"
                            : "text-start",
                          col.className,
                        )}
                      >
                        {col.cell(row)}
                      </td>
                    ))}
                  </tr>
                ))}
          </tbody>
        </table>
      </div>
      {page && page.total > page.limit ? (
        <div className="text-muted-foreground flex items-center justify-end gap-3 text-xs">
          <span className="tabular-nums">
            {t("billing.table.range", {
              from: nf.format(page.offset + 1),
              to: nf.format(Math.min(page.offset + page.limit, page.total)),
              total: nf.format(page.total),
            })}
          </span>
          <Button
            type="button"
            variant="outline"
            size="icon"
            className="size-8"
            aria-label={t("billing.table.previous")}
            disabled={page.offset === 0}
            onClick={() =>
              page.onOffsetChange(Math.max(0, page.offset - page.limit))
            }
          >
            <ChevronLeft className="size-4" />
          </Button>
          <Button
            type="button"
            variant="outline"
            size="icon"
            className="size-8"
            aria-label={t("billing.table.next")}
            disabled={page.offset + page.limit >= page.total}
            onClick={() => page.onOffsetChange(page.offset + page.limit)}
          >
            <ChevronRight className="size-4" />
          </Button>
        </div>
      ) : null}
    </div>
  );
}
