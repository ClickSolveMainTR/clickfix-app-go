import * as Haptics from 'expo-haptics';
import { type Href, router } from 'expo-router';
import * as WebBrowser from 'expo-web-browser';
import { useTranslation } from 'react-i18next';
import { Alert, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { useDecideAction } from './api';
import { type ActionStatus, paramLines, type ProposedAction } from './core';

const TONE: Record<ActionStatus, StatusTone> = {
  proposed: 'neutral',
  confirmed: 'progress',
  rejected: 'neutral',
  done: 'done',
  failed: 'problem',
};

/** Where an action points: a full URL (a panel page) opens in the in-app browser, an app route in the app. */
function openRoute(route: string) {
  if (/^https?:\/\//i.test(route)) return void WebBrowser.openBrowserAsync(route);
  router.push(route as Href);
}

/**
 * Something the assistant offers to do (invite a member, open an e-mail
 * channel, set business hours ...): what and with which values, then
 * Onayla / Reddet. Nothing runs before the member confirms it in the
 * native dialog; the API runs it with the member's own permissions.
 */
export function ActionCard({ thread, action }: { thread: string; action: ProposedAction }) {
  const { t } = useTranslation('help');
  const decide = useDecideAction(thread);
  const lines = paramLines(action.params);
  const busy = decide.isPending;
  const route = action.route;

  const run = (decision: 'confirm' | 'reject') =>
    decide.mutate(
      { action: action.id, decision },
      {
        onSuccess: () => {
          void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
          if (decision === 'confirm') toast.success(t('assistant.action_done'));
        },
        onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
      },
    );

  const confirm = () =>
    Alert.alert(action.title, action.summary || t('assistant.confirm_body'), [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t('assistant.confirm'), onPress: () => run('confirm') },
    ]);

  return (
    <View
      className="gap-3 rounded-2xl border border-border bg-card p-4"
      style={{ borderCurve: 'continuous' }}
      accessibilityLabel={[t('assistant.action'), action.title, action.summary, t(`assistant.status_${action.status}`)]
        .filter(Boolean)
        .join(', ')}>
      <View className="flex-row items-center gap-2">
        <View className="size-9 items-center justify-center rounded-xl bg-secondary">
          <Icon
            name={action.status === 'done' ? 'check' : action.status === 'failed' ? 'error' : 'sparkles'}
            size={20}
            tone={action.status === 'failed' ? 'destructive' : action.status === 'done' ? 'success' : 'primary'}
          />
        </View>
        <Text variant="callout" className="flex-1 font-sans-semibold" numberOfLines={2}>
          {action.title}
        </Text>
        <StatusPill label={t(`assistant.status_${action.status}`)} tone={TONE[action.status] ?? 'neutral'} />
      </View>
      {action.summary ? (
        <Text variant="footnote" tone="muted">
          {action.summary}
        </Text>
      ) : null}
      {action.result ? (
        <Text variant="footnote" tone={action.status === 'failed' ? 'danger' : 'muted'}>
          {action.result}
        </Text>
      ) : null}
      {lines.length ? (
        <View className="gap-0.5 rounded-md bg-muted px-2.5 py-2">
          {lines.map((l) => (
            <Text key={l} variant="footnote" numberOfLines={1}>
              {l}
            </Text>
          ))}
        </View>
      ) : null}
      {action.status === 'proposed' ? (
        <View className="flex-row gap-2">
          <Button
            variant="outline"
            className="flex-1"
            title={t('assistant.reject')}
            disabled={busy}
            onPress={() => run('reject')}
          />
          <Button className="flex-1" title={t('assistant.confirm')} loading={busy} onPress={confirm} />
        </View>
      ) : route && (action.status === 'done' || action.kind === 'open_route') ? (
        <Button variant="secondary" icon="forward" title={t('assistant.open')} onPress={() => openRoute(route)} />
      ) : null}
    </View>
  );
}
