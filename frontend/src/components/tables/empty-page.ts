import type { Table } from "@tanstack/react-table";

import { pageIndexAfterEmptyPage } from "@/components/tables/pagination-summary";

/**
 * Page a table should step back to because its current page is empty, or
 * `null`. Server mode counts what the server sent (`serverRows`,
 * `serverRowCount`); client mode counts the rows left after filtering.
 */
export function tableEmptyPageFallback<TData>(
  table: Table<TData>,
  options: {
    busy: boolean;
    serverRows: number;
    serverRowCount?: number;
  },
): number | null {
  const serverPaged = Boolean(table.options.manualPagination);
  const { pageIndex, pageSize } = table.getState().pagination;
  return pageIndexAfterEmptyPage({
    isLoading: options.busy,
    pageIndex,
    pageSize,
    pageRows: serverPaged
      ? options.serverRows
      : table.getRowModel().rows.length,
    rowCount: serverPaged
      ? options.serverRowCount
      : table.getFilteredRowModel().rows.length,
  });
}
