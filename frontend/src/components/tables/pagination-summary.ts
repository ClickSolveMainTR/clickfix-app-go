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
