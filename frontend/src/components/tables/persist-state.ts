import type { DataTableState } from "@/components/tables/types";

/**
 * What `persistKey` keeps in localStorage: layout and preferences only.
 *
 * The search text and the page index are not preferences: a search typed days
 * ago, or page 4 of another organization's list, would open the screen empty.
 */
export function toPersistedState(
  state: Partial<DataTableState>,
): Partial<DataTableState> {
  const { globalFilter: _search, rowSelection: _selection, ...rest } = state;
  void _search;
  void _selection;
  return {
    ...rest,
    ...(state.pagination
      ? { pagination: { pageIndex: 0, pageSize: state.pagination.pageSize } }
      : {}),
  };
}

/** Reads a stored value, dropping what older versions used to persist. */
export function fromPersistedState(
  raw: unknown,
): Partial<DataTableState> | null {
  if (!raw || typeof raw !== "object") return null;
  return toPersistedState(raw as Partial<DataTableState>);
}
