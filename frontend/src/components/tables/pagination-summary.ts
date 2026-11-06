export type PaginationSummaryInput = {
  pageIndex: number;
  pageSize: number;
  /** Server mode: the table only holds the current page. */
  manual: boolean;
  /** Server mode: total rows across all pages, when the caller knows it. */
  rowCount?: number;
  /** Rows rendered on the current page. */
  pageRows: number;
  /** Client mode: rows left after filtering, across all pages. */
  filteredRows: number;
};

export type PaginationSummary = {
  from: number;
  to: number;
  /** `null` when the table is in server mode and no `rowCount` was given. */
  total: number | null;
};

/**
 * Numbers for the "from–to / total" pagination counter.
 *
 * In server mode the table holds one page, so the filtered row model is the
 * page, not the dataset: the total has to come from `rowCount`.
 */
export function paginationSummary({
  pageIndex,
  pageSize,
  manual,
  rowCount,
  pageRows,
  filteredRows,
}: PaginationSummaryInput): PaginationSummary {
  const offset = pageIndex * pageSize;

  if (!manual) {
    const total = filteredRows;
    return {
      from: total === 0 ? 0 : offset + 1,
      to: Math.min(total, offset + pageSize),
      total,
    };
  }

  const from = pageRows === 0 ? 0 : offset + 1;
  const to = pageRows === 0 ? 0 : offset + pageRows;
  if (rowCount === undefined) return { from, to, total: null };
  return { from, to, total: Math.max(rowCount, to) };
}

export type EmptyPageInput = {
  /** Server mode: the table only holds the current page. */
  manual: boolean;
  isLoading: boolean;
  pageIndex: number;
  pageSize: number;
  /** Rows the server returned for the current page. */
  pageRows: number;
  /** Server mode: total rows across all pages, when the caller knows it. */
  rowCount?: number;
};

/**
 * Server mode: page to fall back to when the current page came back empty
 * (its last row was deleted, or the list shrank on refresh). Without this the
 * user is left on an empty page with no pagination to leave it.
 *
 * Returns `null` when the current page is fine.
 */
export function pageIndexAfterEmptyPage({
  manual,
  isLoading,
  pageIndex,
  pageSize,
  pageRows,
  rowCount,
}: EmptyPageInput): number | null {
  if (!manual || isLoading || pageRows > 0 || pageIndex <= 0) return null;
  if (rowCount === undefined || pageSize <= 0) return pageIndex - 1;
  const lastPage = Math.max(0, Math.ceil(rowCount / pageSize) - 1);
  // Never move forward, and always leave the empty page.
  return Math.min(lastPage, pageIndex - 1);
}
