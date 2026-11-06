import { useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { EmptyState } from '@/ui/empty-state';
import { Screen } from '@/ui/screen';
import { toast } from '@/ui/toast';

import type { T } from './core';

/** `t` of the `admin` namespace in the shape the label helpers take. */
export function useForumT(): T {
  const { t } = useTranslation('admin');
  return (key, params) => t(key, params ?? {});
}

/** A refused write, in the API's words. */
export function forumFailed(t: T, e: unknown) {
  toast.error(`${t('forum.failed')} ${errorMessage(isApiError(e) ? e.code : undefined)}`);
}

/** Asks before an action that cannot be taken back from the app. */
export function confirmAction(
  t: T,
  o: { title: string; body: string; action: string; destructive?: boolean; run: () => void },
) {
  Alert.alert(o.title, o.body, [
    { text: t('forum.cancel'), style: 'cancel' },
    { text: o.action, style: o.destructive ? 'destructive' : 'default', onPress: o.run },
  ]);
}

/** The whole screen when the member cannot manage the forum, or the link names nothing. */
export function ForumNotice({ kind }: { kind: 'forbidden' | 'missing' }) {
  const t = useForumT();
  return (
    <Screen>
      <EmptyState
        icon="chat"
        title={t(kind === 'forbidden' ? 'forum.no_permission' : 'forum.topic_missing')}
        body={kind === 'forbidden' ? t('forum.no_permission_hint') : undefined}
      />
    </Screen>
  );
}

/**
 * One write at a time, also for two taps inside the same frame (before the
 * mutation's pending state reaches the buttons): `start` answers false while
 * a write is on its way; call `end` when it settled.
 */
export function useSingleFlight() {
  const flying = useRef(false);
  return {
    start: () => {
      if (flying.current) return false;
      flying.current = true;
      return true;
    },
    end: () => {
      flying.current = false;
    },
  };
}
