"use client";

import type { ReactNode } from "react";

import {
  DataTable,
  type DataTableBulkAction,
  type DataTableProps,
} from "@/components/tables";
import { ErrorState } from "@/components/common/error-state";
import { useLocale } from "@/providers/locale-provider";

type EntityTableProps<TData> = Omit<
  DataTableProps<TData>,
  "isLoading" | "errorState" | "emptyTitle" | "emptyDescription" | "rowCount"
> & {
  /**
   * Total rows across all pages, as returned by the list endpoint (`total`).
   * Drives the page count and the "from–to / total" counter; do not compute
   * `pageCount` by hand.
   */
  total?: number;
  isLoading?: boolean;
  isError?: boolean;
  errorTitle?: string;
  errorDescription?: string;
  onRetry?: () => void;
  emptyTitle?: string;
  emptyDescription?: string;
  bulkActions?: DataTableBulkAction<TData>[];
  toolbarExtra?: ReactNode;
};

/**
 * DataTable wrapper with shared loading / empty / error conventions
 * for platform entity list pages.
 *
 * Server mode is the default (`manual` filtering / sorting / pagination), so
 * search, column filters and sorting are shown but only work when the page
 * sends them to the endpoint. Turn off what the endpoint does not support:
 * `features={{ globalFilter: false, columnFilters: false, sorting: false }}`.
 */
export function EntityTable<TData>({
  isLoading,
  isError,
  errorTitle,
  errorDescription,
  onRetry,
  total,
  emptyTitle,
  emptyDescription,
  toolbar,
  toolbarExtra,
  ...props
}: EntityTableProps<TData>) {
  const { t } = useLocale();

  const errorState = isError ? (
    <ErrorState
      title={errorTitle ?? t("common.error_generic")}
      description={errorDescription}
      onRetry={onRetry}
      retryLabel={t("common.retry")}
    />
  ) : undefined;

  const mergedToolbar =
    toolbar || toolbarExtra ? (
      <>
        {toolbarExtra}
        {typeof toolbar === "function" ? null : toolbar}
      </>
    ) : (
      toolbar
    );

  return (
    <DataTable
      {...props}
      rowCount={total}
      isLoading={isLoading}
      errorState={errorState}
      emptyTitle={emptyTitle ?? t("common.empty_title")}
      emptyDescription={emptyDescription ?? t("common.empty_description")}
      toolbar={
        typeof toolbar === "function"
          ? (table) => (
              <>
                {toolbarExtra}
                {toolbar(table)}
              </>
            )
          : mergedToolbar
      }
      manual={{
        filtering: true,
        sorting: true,
        pagination: true,
        ...props.manual,
      }}
      features={{
        sorting: true,
        globalFilter: true,
        columnFilters: true,
        columnVisibility: true,
        columnResizing: true,
        rowSelection: true,
        pagination: true,
        density: true,
        facetedFilters: true,
        ...props.features,
      }}
    />
  );
}
