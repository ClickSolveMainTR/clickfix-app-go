"use client";

import { useEffect, useId, useMemo, useState, type ReactNode } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { DataTablePagination } from "@/components/tables/data-table-pagination";
import { DataTableToolbar } from "@/components/tables/data-table-toolbar";
import { GridView } from "@/components/tables/grid-view";
import { tableEmptyPageFallback } from "@/components/tables/empty-page";
import { createExpandColumnDef } from "@/components/tables/expand-column";
import { createReorderColumnDef } from "@/components/tables/reorder-column";
import { reorderRowsById } from "@/components/tables/reorder-rows";
import { TableView } from "@/components/tables/table-view";
import type { DataTableProps } from "@/components/tables/types";
import { useDataTable } from "@/components/tables/use-data-table";
import { Skeleton } from "@/components/ui/skeleton";
import { useIsMobile } from "@/hooks/use-mobile";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

export function DataTable<TData>({
  columns,
  data,
  getRowId,
  getRowCanSelect,
  maxSelection,
  isLoading,
  errorState,
  emptyTitle,
  emptyDescription,
  features,
  manual,
  state,
  initialState,
  pageCount,
  rowCount,
  pageSizeOptions,
  searchPlaceholder,
  toolbar,
  bulkActions,
  renderGridItem,
  onRowReorder,
  onCellEdit,
  onExportReady,
  onRowClick,
  renderSubRow,
  getRowCanExpand,
  className,
}: DataTableProps<TData>) {
  const { t } = useLocale();
  const isMobile = useIsMobile();
  const rowHintId = useId();
  const [reorderModeRequested, setReorderModeRequested] = useState(false);
  const [editingCell, setEditingCell] = useState<{
    rowId: string;
    columnId: string;
  } | null>(null);

  const resolvedGetRowId =
    getRowId ??
    ((row: TData, index: number) => {
      const maybe = row as { id?: string | number };
      return maybe.id != null ? String(maybe.id) : String(index);
    });

  const forceMobileCardsPreview =
    isMobile && features?.mobileAutoCards !== false;
  const canReorder =
    Boolean(features?.rowReorder) &&
    Boolean(onRowReorder) &&
    !forceMobileCardsPreview;
  const reorderMode = canReorder && reorderModeRequested;

  // Detail rows close while rows are dragged: a row under a row would get in
  // the way of the drop targets.
  const expandable = Boolean(renderSubRow) && !reorderMode;

  const columnsWithReorder = useMemo(() => {
    const lead = [
      ...(reorderMode ? [createReorderColumnDef<TData>()] : []),
      ...(expandable ? [createExpandColumnDef<TData>()] : []),
    ];
    return lead.length ? [...lead, ...columns] : columns;
  }, [columns, reorderMode, expandable]);

  const {
    table,
    features: resolvedFeatures,
    density,
    setDensity,
    viewMode,
    setViewMode,
    resetAll,
  } = useDataTable({
    columns: columnsWithReorder,
    data,
    getRowId: resolvedGetRowId,
    getRowCanSelect,
    maxSelection,
    features,
    manual,
    state,
    initialState,
    pageCount,
    rowCount,
    getRowCanExpand: expandable ? (getRowCanExpand ?? (() => true)) : undefined,
  });

  // An empty page past the first one (last row deleted, list shrank on
  // refresh) has no rows and so no pagination; step back to the last page
  // that has rows. Server mode counts what the server sent, client mode what
  // is left after filtering.
  const fallbackPageIndex = resolvedFeatures.pagination
    ? tableEmptyPageFallback(table, {
        busy: Boolean(isLoading) || Boolean(errorState),
        serverRows: data.length,
        serverRowCount: rowCount,
      })
    : null;
  useEffect(() => {
    if (fallbackPageIndex !== null) table.setPageIndex(fallbackPageIndex);
  }, [fallbackPageIndex, table]);

  const forceMobileCards =
    isMobile && resolvedFeatures.mobileAutoCards !== false;

  const displayMode = reorderMode
    ? "table"
    : forceMobileCards
      ? "grid"
      : viewMode;

  const activeEditingCell = reorderMode ? null : editingCell;

  const toolbarNode: ReactNode =
    typeof toolbar === "function" ? toolbar(table) : toolbar;

  const filteredCount = table.getFilteredRowModel().rows.length;
  const hasRows = table.getRowModel().rows.length > 0;
  const columnCount = table.getVisibleLeafColumns().length;

  const emptyMessage = !data.length
    ? {
        title: emptyTitle ?? t("common.empty_title"),
        description: emptyDescription ?? t("common.empty_description"),
      }
    : !filteredCount
      ? {
          title: t("table.no_results"),
          description: t("table.reset_filters"),
        }
      : null;

  return (
    <div className={cn("space-y-3", className)}>
      <DataTableToolbar
        table={table}
        features={resolvedFeatures}
        density={density}
        setDensity={setDensity}
        viewMode={viewMode}
        setViewMode={setViewMode}
        resetAll={resetAll}
        toolbar={toolbarNode}
        searchPlaceholder={searchPlaceholder}
        bulkActions={bulkActions}
        forceMobileCards={forceMobileCards}
        reorderMode={reorderMode}
        onReorderModeChange={
          canReorder
            ? (next) => {
                if (next) {
                  setViewMode("table");
                  table.resetSorting();
                  setEditingCell(null);
                }
                setReorderModeRequested(next);
              }
            : undefined
        }
        onExport={
          onExportReady
            ? () =>
                onExportReady(
                  table.getFilteredRowModel().rows.map((r) => r.original),
                  table,
                )
            : undefined
        }
      />

      {reorderMode ? (
        <p className="text-muted-foreground text-xs">
          {t("table.reorder_hint")}
        </p>
      ) : null}

      {onRowClick ? (
        <span id={rowHintId} className="sr-only">
          {t("table.row_open_hint")}
        </span>
      ) : null}

      {errorState ? (
        errorState
      ) : isLoading ? (
        // The toolbar above stays mounted: the search box and the filters
        // keep their focus while the rows load.
        displayMode === "grid" ? (
          <div className="grid grid-cols-1 gap-3" aria-busy="true">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-36 w-full rounded-xl" />
            ))}
          </div>
        ) : (
          <div className="space-y-2" aria-busy="true">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        )
      ) : displayMode === "grid" ? (
        emptyMessage && !hasRows ? (
          <div className="space-y-3">
            <EmptyState
              title={emptyMessage.title}
              description={emptyMessage.description}
            />
          </div>
        ) : (
          <GridView
            table={table}
            renderGridItem={renderGridItem}
            mobile={forceMobileCards}
            onRowClick={reorderMode ? undefined : onRowClick}
            rowHintId={rowHintId}
            renderSubRow={expandable ? renderSubRow : undefined}
          />
        )
      ) : (
        <TableView
          table={table}
          density={density}
          showColumnFilters={
            resolvedFeatures.columnFilters && !isMobile && !reorderMode
          }
          emptyMessage={emptyMessage}
          columnCount={columnCount}
          reorderMode={reorderMode}
          onReorder={(activeId, overId) => {
            if (!onRowReorder) return;
            onRowReorder(
              reorderRowsById(data, resolvedGetRowId, activeId, overId),
            );
          }}
          inlineEdit={
            Boolean(resolvedFeatures.inlineEdit) &&
            Boolean(onCellEdit) &&
            !reorderMode
          }
          editingCell={activeEditingCell}
          onEditingCellChange={setEditingCell}
          onCellEdit={onCellEdit}
          onRowClick={reorderMode ? undefined : onRowClick}
          rowHintId={rowHintId}
          renderSubRow={expandable ? renderSubRow : undefined}
        />
      )}

      {resolvedFeatures.pagination &&
      hasRows &&
      !reorderMode &&
      !isLoading &&
      !errorState ? (
        <DataTablePagination
          table={table}
          pageSizeOptions={pageSizeOptions}
          compact={forceMobileCards}
        />
      ) : null}
    </div>
  );
}
