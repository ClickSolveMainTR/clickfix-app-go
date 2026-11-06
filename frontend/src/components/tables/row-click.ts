import type React from "react";

const ROW_CLICK_IGNORE_SELECTOR = [
  "a",
  "button",
  "input",
  "textarea",
  "select",
  "label",
  "[role='button']",
  "[role='checkbox']",
  "[role='menuitem']",
  "[role='option']",
  "[role='switch']",
  "[data-no-row-click]",
].join(", ");

export function isRowClickIgnored(event: {
  defaultPrevented: boolean;
  target: EventTarget | null;
}): boolean {
  if (event.defaultPrevented) return true;
  if (!(event.target instanceof Element)) return true;
  if (event.target.closest(ROW_CLICK_IGNORE_SELECTOR)) return true;

  const selection = window.getSelection();
  if (selection && !selection.isCollapsed && selection.toString().trim()) {
    return true;
  }

  return false;
}

/**
 * Keyboard activation of a clickable row / card: Enter or Space while the row
 * itself has focus. Keys pressed on a control inside the row (button, link,
 * checkbox) belong to that control and must not also open the row.
 */
export function isRowKeyActivation(event: {
  key: string;
  defaultPrevented: boolean;
  target: EventTarget | null;
  currentTarget: EventTarget | null;
}): boolean {
  if (event.defaultPrevented) return false;
  if (event.target !== event.currentTarget) return false;
  return event.key === "Enter" || event.key === " ";
}

/** Props that make a row / card with `onRowClick` reachable by keyboard. */
export function rowClickProps<TData, TElement extends HTMLElement>(
  row: TData,
  onRowClick: ((row: TData) => void) | undefined,
  hintId: string | undefined,
) {
  if (!onRowClick) return {};
  return {
    tabIndex: 0,
    "aria-describedby": hintId,
    onClick: (event: React.MouseEvent<TElement>) => {
      if (isRowClickIgnored(event)) return;
      onRowClick(row);
    },
    onKeyDown: (event: React.KeyboardEvent<TElement>) => {
      if (!isRowKeyActivation(event)) return;
      // Space would scroll the page.
      event.preventDefault();
      onRowClick(row);
    },
  };
}

/** Focus ring of a clickable row / card (keyboard focus only). */
export const ROW_FOCUS_CLASS =
  "cursor-pointer focus-visible:outline-ring outline-none focus-visible:outline-2 focus-visible:-outline-offset-2";
