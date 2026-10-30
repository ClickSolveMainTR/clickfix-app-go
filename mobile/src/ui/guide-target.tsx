import { type ComponentRef, type ReactNode, useCallback } from 'react';
import { View, type ViewProps } from 'react-native';

/** A view's box in window coordinates. */
export type Rect = { x: number; y: number; width: number; height: number; radius?: number };

type NativeView = ComponentRef<typeof View>;

/**
 * Views a guide step can point at, by guide id (`inbox.filters`,
 * `conversation.composer`, ...). The id is the step's `target` in the
 * CF-141 guide content; the list lives in `docs/mobile/guides.md`. When two
 * mounted screens register one id (a stacked screen under another), the
 * latest one wins.
 */
const targets = new Map<string, { view: NativeView; radius: number }>();

/**
 * Callback ref that registers a view as a guide target; spread it on an
 * existing `View` (`ref={useGuideTarget('inbox.list')}`). `enabled: false`
 * leaves the view unregistered (e.g. every list row but the first).
 */
export function useGuideTarget(id: string, enabled = true, radius = 12) {
  return useCallback(
    (view: NativeView | null) => {
      if (!view || !enabled) return;
      targets.set(id, { view, radius });
      return () => {
        if (targets.get(id)?.view === view) targets.delete(id);
      };
    },
    [id, enabled, radius],
  );
}

/** Wraps children in a measurable view registered as a guide target. */
export function GuideTarget({
  id,
  enabled = true,
  children,
  radius = 12,
  ...props
}: ViewProps & { id: string; enabled?: boolean; radius?: number; children: ReactNode }) {
  const ref = useGuideTarget(id, enabled, radius);
  return (
    <View ref={ref} collapsable={false} {...props}>
      {children}
    </View>
  );
}

/** Where a target is on screen (window coordinates), or null when it is not mounted. */
export function measureTarget(id: string): Promise<Rect | null> {
  const aliases: Record<string, string> = { 'inbox-filter': 'inbox.views', composer: 'conversation.composer' };
  const target = id ? targets.get(aliases[id] ?? id) : undefined;
  const view = target?.view;
  if (!view) return Promise.resolve(null);
  return new Promise((resolve) => {
    // A view that is detached never calls back.
    const timer = setTimeout(() => resolve(null), 300);
    view.measureInWindow((x, y, width, height) => {
      clearTimeout(timer);
      resolve({ x, y, width, height, radius: target?.radius });
    });
  });
}
