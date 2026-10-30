/**
 * ClickFix mark geometry, taken verbatim from the official logo kit
 * (`src/assets/brand/clickfix-*.svg`). Inlined so no request is made for it.
 *
 * The same cursor appears in three artboards; the polygon is translated, never
 * redrawn, so keep these in sync with the SVG files.
 */

/** Circle artboard (kit Assets 2-4): disc + cursor. */
export const MARK_VIEWBOX = "0 0 1832 1832";
export const MARK_DISC = { cx: 916, cy: 916, r: 916 } as const;
export const MARK_CURSOR_POINTS = "411 1319 347 575 1627 743 635 751 411 1319";

/** Square tile artboard (kit Assets 1, 7-9): cursor centred on a square. */
export const TILE_VIEWBOX = "0 0 2050 2050";
export const TILE_CURSOR_POINTS =
  "482.33 1391 418.33 647 1698.33 815 706.33 823 482.33 1391";

/** Bare cursor artboard (kit Assets 5-6). */
export const CURSOR_ONLY_VIEWBOX = "0 0 1280 744";
export const CURSOR_ONLY_POINTS = "64 744 0 0 1280 168 288 176 64 744";
