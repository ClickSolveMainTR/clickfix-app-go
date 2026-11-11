import type { RowSelectionState } from "@tanstack/react-table";

/** Number of selected rows in a TanStack selection map. */
export function selectedCount(selection: RowSelectionState): number {
  let n = 0;
  for (const on of Object.values(selection)) if (on) n += 1;
  return n;
}

/**
 * Applies a selection ceiling: rows that stay selected are kept, then new
 * rows are added in the order TanStack listed them until `max` is reached.
 * "Select all" on a page larger than the ceiling therefore selects the first
 * rows only.
 */
export function capSelection(
  current: RowSelectionState,
  next: RowSelectionState,
  max: number,
): RowSelectionState {
  if (selectedCount(next) <= max) return next;
  const out: RowSelectionState = {};
  let n = 0;
  for (const [id, on] of Object.entries(next)) {
    if (on && current[id] && n < max) {
      out[id] = true;
      n += 1;
    }
  }
  for (const [id, on] of Object.entries(next)) {
    if (n >= max) break;
    if (on && !current[id]) {
      out[id] = true;
      n += 1;
    }
  }
  return out;
}
