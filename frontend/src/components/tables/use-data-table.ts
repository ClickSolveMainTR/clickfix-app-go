"use client";

import {
  getCoreRowModel,
  getFacetedRowModel,
  getFacetedUniqueValues,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type ColumnFiltersState,
  type ColumnOrderState,
  type ColumnPinningState,
  type ColumnSizingState,
  type ExpandedState,
  type PaginationState,
  type RowSelectionState,
  type SortingState,
  type VisibilityState,
} from "@tanstack/react-table";
import { useEffect, useMemo, useState } from "react";

import {
  fromPersistedState,
  toPersistedState,
} from "@/components/tables/persist-state";
import { searchTextIncludes } from "@/components/tables/search-text";
import { capSelection, selectedCount } from "@/components/tables/selection-cap";
import type {
  DataTableFeatures,
  DataTableState,
  TableDensity,
  TableViewMode,
  UseDataTableOptions,
} from "@/components/tables/types";

const DEFAULT_FEATURES: Required<Omit<DataTableFeatures, "persistKey">> & {
  persistKey?: string;
} = {
  sorting: true,
  multiSort: true,
  globalFilter: true,
  columnFilters: true,
  columnVisibility: true,
  columnOrdering: true,
  columnPinning: true,
  columnResizing: true,
  rowSelection: true,
  pagination: true,
  density: true,
  viewMode: true,
  mobileAutoCards: true,
  rowReorder: false,
  inlineEdit: false,
  facetedFilters: true,
  toolbar: true,
};

function readPersisted(
  key: string | undefined,
): Partial<DataTableState> | null {
  if (!key || typeof window === "undefined") return null;
  try {
    const raw = window.localStorage.getItem(key);
    return raw ? fromPersistedState(JSON.parse(raw)) : null;
  } catch {
    return null;
  }
}

function writePersisted(
  key: string | undefined,
  state: Partial<DataTableState>,
) {
  if (!key || typeof window === "undefined") return;
  try {
    window.localStorage.setItem(key, JSON.stringify(toPersistedState(state)));
  } catch {
    // ignore quota
  }
}

function defaultHiddenVisibility<TData>(
  columns: ColumnDef<TData, unknown>[],
): VisibilityState {
  const visibility: VisibilityState = {};
  for (const col of columns) {
    const id =
      ("id" in col && col.id) ||
      ("accessorKey" in col && String(col.accessorKey));
    if (id && col.meta?.defaultHidden) {
      visibility[id] = false;
    }
  }
  return visibility;
}

export function useDataTable<TData>(options: UseDataTableOptions<TData>) {
  const {
    columns,
    data,
    getRowId,
    getRowCanSelect,
    maxSelection,
    features: featuresProp,
    manual,
    state: controlled,
    initialState,
    pageCount,
    rowCount,
    getRowCanExpand,
  } = options;

  const features = { ...DEFAULT_FEATURES, ...featuresProp };
  const persisted = useMemo(
    () => readPersisted(features.persistKey),
    [features.persistKey],
  );

  const [sorting, setSorting] = useState<SortingState>(
    controlled?.sorting ?? persisted?.sorting ?? initialState?.sorting ?? [],
  );
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>(
    controlled?.columnFilters ??
      persisted?.columnFilters ??
      initialState?.columnFilters ??
      [],
  );
  const [globalFilter, setGlobalFilter] = useState(
    controlled?.globalFilter ?? initialState?.globalFilter ?? "",
  );
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>(
    controlled?.columnVisibility ??
      persisted?.columnVisibility ??
      initialState?.columnVisibility ??
      defaultHiddenVisibility(columns),
  );
  const [columnOrder, setColumnOrder] = useState<ColumnOrderState>(
    controlled?.columnOrder ??
      persisted?.columnOrder ??
      initialState?.columnOrder ??
      [],
  );
  const [columnPinning, setColumnPinning] = useState<ColumnPinningState>(
    controlled?.columnPinning ??
      persisted?.columnPinning ??
      initialState?.columnPinning ??
      {},
  );
  const [columnSizing, setColumnSizing] = useState<ColumnSizingState>(
    controlled?.columnSizing ??
      persisted?.columnSizing ??
      initialState?.columnSizing ??
      {},
  );
  const [rowSelection, setRowSelection] = useState<RowSelectionState>(
    controlled?.rowSelection ?? initialState?.rowSelection ?? {},
  );
  // Open detail rows; UI only, never persisted.
  const [expanded, setExpanded] = useState<ExpandedState>({});
  const [pagination, setPagination] = useState<PaginationState>(
    controlled?.pagination ??
      persisted?.pagination ??
      initialState?.pagination ?? { pageIndex: 0, pageSize: 10 },
  );
  const [density, setDensity] = useState<TableDensity>(
    controlled?.density ??
      persisted?.density ??
      initialState?.density ??
      "comfortable",
  );
  const [viewMode, setViewMode] = useState<TableViewMode>(
    controlled?.viewMode ??
      persisted?.viewMode ??
      initialState?.viewMode ??
      "table",
  );

  // Sync controlled props inward
  useEffect(() => {
    if (controlled?.sorting) setSorting(controlled.sorting);
  }, [controlled?.sorting]);
  useEffect(() => {
    if (controlled?.columnFilters) setColumnFilters(controlled.columnFilters);
  }, [controlled?.columnFilters]);
  useEffect(() => {
    if (controlled?.globalFilter !== undefined) {
      setGlobalFilter(controlled.globalFilter);
    }
  }, [controlled?.globalFilter]);

  useEffect(() => {
    writePersisted(features.persistKey, {
      sorting,
      columnFilters,
      globalFilter,
      columnVisibility,
      columnOrder,
      columnPinning,
      columnSizing,
      pagination,
      density,
      viewMode,
    });
  }, [
    features.persistKey,
    sorting,
    columnFilters,
    globalFilter,
    columnVisibility,
    columnOrder,
    columnPinning,
    columnSizing,
    pagination,
    density,
    viewMode,
  ]);

  const currentSelection = controlled?.rowSelection ?? rowSelection;
  const atSelectionCap =
    maxSelection !== undefined &&
    selectedCount(currentSelection) >= maxSelection;
  const onRowSelectionChange = (
    updater: Parameters<typeof setRowSelection>[0],
  ) => {
    const set = controlled?.onRowSelectionChange ?? setRowSelection;
    if (maxSelection === undefined) return set(updater);
    const next =
      typeof updater === "function" ? updater(currentSelection) : updater;
    set(capSelection(currentSelection, next, maxSelection));
  };

  // Uncontrolled pagination only: a controlled list (`useServerListState`)
  // resets its own page when search, filters or sorting change.
  const toFirstPage = () => {
    if (controlled?.onPaginationChange) return;
    setPagination((prev) =>
      prev.pageIndex === 0 ? prev : { ...prev, pageIndex: 0 },
    );
  };

  // TanStack Table returns functions that React Compiler cannot memoize safely.
  // eslint-disable-next-line react-hooks/incompatible-library -- library API
  const table = useReactTable({
    data,
    columns,
    pageCount,
    rowCount,
    getRowId,
    state: {
      sorting: controlled?.sorting ?? sorting,
      columnFilters: controlled?.columnFilters ?? columnFilters,
      globalFilter: controlled?.globalFilter ?? globalFilter,
      columnVisibility: controlled?.columnVisibility ?? columnVisibility,
      columnOrder: controlled?.columnOrder ?? columnOrder,
      columnPinning: controlled?.columnPinning ?? columnPinning,
      columnSizing: controlled?.columnSizing ?? columnSizing,
      rowSelection: controlled?.rowSelection ?? rowSelection,
      pagination: controlled?.pagination ?? pagination,
      expanded,
    },
    onSortingChange: (updater) => {
      (controlled?.onSortingChange ?? setSorting)(updater);
      toFirstPage();
    },
    onColumnFiltersChange: (updater) => {
      (controlled?.onColumnFiltersChange ?? setColumnFilters)(updater);
      toFirstPage();
    },
    onGlobalFilterChange: (updater) => {
      (controlled?.onGlobalFilterChange ?? setGlobalFilter)(updater);
      toFirstPage();
    },
    onColumnVisibilityChange:
      controlled?.onColumnVisibilityChange ?? setColumnVisibility,
    onColumnOrderChange: controlled?.onColumnOrderChange ?? setColumnOrder,
    onColumnPinningChange:
      controlled?.onColumnPinningChange ?? setColumnPinning,
    onColumnSizingChange: controlled?.onColumnSizingChange ?? setColumnSizing,
    onRowSelectionChange,
    onPaginationChange: controlled?.onPaginationChange ?? setPagination,
    onExpandedChange: setExpanded,
    getRowCanExpand: getRowCanExpand
      ? (row) => getRowCanExpand(row.original)
      : () => false,

    enableSorting: features.sorting,
    // Hiding a column needs a way to bring it back: the toolbar's column
    // menu. Without that menu the header's "hide column" must go too.
    enableHiding: features.columnVisibility && features.toolbar,
    enableMultiSort: features.multiSort,
    enableSortingRemoval: true,
    enableFilters: features.columnFilters || features.globalFilter,
    enableColumnFilters: features.columnFilters,
    enableGlobalFilter: features.globalFilter,
    enableRowSelection:
      features.rowSelection && (getRowCanSelect || atSelectionCap)
        ? (row) =>
            (!getRowCanSelect || getRowCanSelect(row.original)) &&
            (!atSelectionCap || row.getIsSelected())
        : features.rowSelection,
    enableColumnResizing: features.columnResizing,
    columnResizeMode: "onChange",
    enableColumnPinning: features.columnPinning,

    manualSorting: manual?.sorting,
    manualFiltering: manual?.filtering,
    manualPagination: manual?.pagination,

    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: features.sorting ? getSortedRowModel() : undefined,
    getFilteredRowModel:
      features.columnFilters || features.globalFilter
        ? getFilteredRowModel()
        : undefined,
    getPaginationRowModel: features.pagination
      ? getPaginationRowModel()
      : undefined,
    getFacetedRowModel: features.columnFilters
      ? getFacetedRowModel()
      : undefined,
    getFacetedUniqueValues: features.columnFilters
      ? getFacetedUniqueValues()
      : undefined,

    // Turkish-aware and accent-insensitive; TanStack's "includesString" lower
    // cases with toLowerCase(), which never matches İ / I against i / ı.
    globalFilterFn: (row, columnId, filterValue) =>
      searchTextIncludes(row.getValue(columnId), filterValue),

    // Client mode: a data refresh (edit, toggle) must not jump to page 1.
    // The page resets below when search, filters or sorting change.
    autoResetPageIndex: false,
  });

  const resetAll = () => {
    table.resetSorting();
    table.resetColumnFilters();
    table.resetGlobalFilter();
    table.resetColumnVisibility();
    table.resetColumnOrder();
    table.resetColumnPinning();
    table.resetColumnSizing();
    table.resetRowSelection();
    table.resetExpanded();
    // Back to the first page only: `resetPagination()` would also drop the
    // page size to TanStack's default (10), whatever the screen started with.
    table.setPageIndex(0);
    setDensity(initialState?.density ?? "comfortable");
    setViewMode(initialState?.viewMode ?? "table");
    controlled?.onDensityChange?.(initialState?.density ?? "comfortable");
    controlled?.onViewModeChange?.(initialState?.viewMode ?? "table");
  };

  return {
    table,
    features,
    density: controlled?.density ?? density,
    setDensity: controlled?.onDensityChange ?? setDensity,
    viewMode: controlled?.viewMode ?? viewMode,
    setViewMode: controlled?.onViewModeChange ?? setViewMode,
    resetAll,
  };
}
