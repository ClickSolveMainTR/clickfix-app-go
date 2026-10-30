/**
 * Pure rules of the in-app guides (CF-142): route patterns, which tour starts
 * by itself, and where the coach mark's card sits. Kept free of app imports
 * so they run under `node --test`.
 */

import type { components } from '../../core/api/schema';
export type Guide = components['schemas']['Guide'];
export type GuideStep = components['schemas']['GuideStep'];
export type GuideStepAction = components['schemas']['GuideStepAction'];
export type GuideStatus = components['schemas']['GuideProgress']['status'];
export type Placement = GuideStep['placement'];

/**
 * The route pattern a guide is written for, from expo-router's segments:
 * groups dropped, dynamic segments kept as written
 * (`(app)/(org)/conversation/[uuid]` → `/conversation/[uuid]`, a tab's
 * index → `/inbox`).
 */
export function routePattern(segments: readonly string[]): string {
  const parts = segments.filter((s) => s && !(s.startsWith('(') && s.endsWith(')')) && s !== 'index');
  return `/${parts.join('/')}`;
}

/** A tour the member has not seen yet on this screen starts by itself; lowest `order` first. */
export function autoStartGuide(guides: readonly Guide[], enabled = true): Guide | null {
  if (!enabled) return null;
  const fresh = guides
    .filter((g) => g.kind === 'tour' && g.steps.length > 0 && g.progress.status === 'new')
    .sort((a, b) => a.order - b.order);
  return fresh[0] ?? null;
}

/** "What is this screen for?": the screen's page guide, else its first tour. */
export function screenGuide(guides: readonly Guide[]): Guide | null {
  const usable = guides.filter((g) => g.steps.length > 0).sort((a, b) => a.order - b.order);
  return usable.find((g) => g.kind === 'page') ?? usable[0] ?? null;
}

/** Where a resumed guide continues: its saved step while in progress, else the start. */
export function resumeIndex(guide: Guide): number {
  if (guide.progress.status !== 'in_progress') return 0;
  return Math.min(Math.max(guide.progress.step_index, 0), Math.max(guide.steps.length - 1, 0));
}

/** The next setup guide to run: the first one not completed or skipped, in order. */
export function nextSetupGuide(guides: readonly Guide[]): Guide | null {
  return (
    [...guides]
      .sort((a, b) => a.order - b.order)
      .find((g) => g.steps.length > 0 && (g.progress.status === 'new' || g.progress.status === 'in_progress')) ?? null
  );
}

export type Rect = { x: number; y: number; width: number; height: number; radius?: number };
export type Insets = { top: number; bottom: number; left: number; right: number };

/** Space kept around the highlighted view and between it and the card. */
export const SPOT_PADDING = 6;
const GAP = 12;
const MARGIN = 16;

/** The spotlight hole: the target grown by the padding, kept inside the window. */
export function spotlightRect(target: Rect, win: { width: number; height: number }): Rect {
  const x = Math.max(target.x - SPOT_PADDING, 0);
  const y = Math.max(target.y - SPOT_PADDING, 0);
  const right = Math.min(target.x + target.width + SPOT_PADDING, win.width);
  const bottom = Math.min(target.y + target.height + SPOT_PADDING, win.height);
  return { x, y, width: Math.max(right - x, 0), height: Math.max(bottom - y, 0) };
}

/** A measured view that is on screen and big enough to point at. */
export function isVisibleRect(r: Rect | null | undefined, win: { width: number; height: number }): r is Rect {
  if (!r || r.width < 2 || r.height < 2) return false;
  return r.x < win.width && r.y < win.height && r.x + r.width > 0 && r.y + r.height > 0;
}

/**
 * Top of the step card beside the spotlight. `top` / `bottom` are honored
 * when the card fits there; `auto` (and `left` / `right`, which a phone has
 * no room for) takes the side with more space. The card never leaves the
 * safe area.
 */
export function cardTop(
  hole: Rect,
  cardHeight: number,
  win: { width: number; height: number },
  insets: Insets,
  placement: Placement,
): { top: number; side: 'top' | 'bottom' } {
  const minTop = insets.top + MARGIN;
  const maxTop = win.height - insets.bottom - MARGIN - cardHeight;
  const below = hole.y + hole.height + GAP;
  const above = hole.y - GAP - cardHeight;
  const fitsBelow = below <= maxTop;
  const fitsAbove = above >= minTop;
  let side: 'top' | 'bottom';
  if (placement === 'bottom' && fitsBelow) side = 'bottom';
  else if (placement === 'top' && fitsAbove) side = 'top';
  else if (fitsBelow !== fitsAbove) side = fitsBelow ? 'bottom' : 'top';
  else side = win.height - (hole.y + hole.height) >= hole.y ? 'bottom' : 'top';
  const raw = side === 'bottom' ? below : above;
  return { top: Math.min(Math.max(raw, minTop), Math.max(maxTop, minTop)), side };
}

/** Automatic setup never resumes a partly seen or dismissed sequence. */
export function autoSetupGuide(guides: readonly Guide[], enabled: boolean): Guide | null {
  if (!enabled || guides.some((g) => g.progress.status !== 'new')) return null;
  return [...guides].sort((a, b) => a.order - b.order).find((g) => g.steps.length > 0) ?? null;
}
