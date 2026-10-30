import { HeaderActions } from '@/features/guides/help-menu';
import { GuideTarget } from '@/ui/guide-target';
import { useCompactHeader } from '@/features/shell/compact-header';
import * as Haptics from 'expo-haptics';
import { type Href, router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { ActionMenu } from '@/ui/menu';
import { formatRelative } from '@/core/format';
import { appRoute } from '@/features/push/links';
import { Chips } from '@/ui/chips';
import { cn } from '@/ui/cn';
import { ConnectionBanner } from '@/ui/connection-banner';
import { HeaderButton } from '@/ui/header-button';
import { Icon } from '@/ui/icon';
import { PagedList } from '@/ui/paged-list';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import {
  type AppNotification,
  useMarkAllNotificationsRead,
  useMarkNotificationRead,
  useNotifications,
  useUnreadNotifications,
} from './api';

import { notificationBody, notificationBodyRoute } from './body';

type Filter = 'all' | 'unread';

/**
 * Notification center (CF-79): the same in-app inbox as the panel's bell
 * (mentions, assignments, SLA, billing, team chat). A tap marks it read and
 * opens what it is about; "Mark all read" clears the tab badge.
 */
export function NotificationsScreen() {
  const { t } = useTranslation('hub');
  const compactHeader = useCompactHeader(t('notifications.title'));
  const [filter, setFilter] = useState<Filter>('all');
  const list = useNotifications(filter === 'unread');
  const unread = useUnreadNotifications();
  const readAll = useMarkAllNotificationsRead();

  return (
    <>
      <Stack.Screen
        options={{
          ...compactHeader,
          headerRight: () => (
            <HeaderActions>
              {unread > 0 ? (
                <HeaderButton
                  icon="check"
                  disabled={readAll.isPending}
                  label={t('notifications.read_all')}
                  onPress={() =>
                    readAll.mutate(undefined, {
                      onSuccess: () => toast.success(t('notifications.read_all_done')),
                      onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
                    })
                  }
                />
              ) : null}
              <ActionMenu<Filter>
                title={t('common:filter')}
                value={filter}
                trigger={<HeaderButton icon="filter" label={t('common:filter')} />}
                options={[
                  { value: 'all', label: t('notifications.all'), systemImage: 'bell' },
                  { value: 'unread', label: t('notifications.unread'), systemImage: 'envelope.badge' },
                ]}
                onPick={setFilter}
              />
            </HeaderActions>
          ),
        }}
      />
      <PagedList<AppNotification>
        query={list}
        keyOf={(n) => n.uuid}
        emptyIcon="bell"
        empty={t('notifications.empty_title')}
        emptyBody={filter === 'unread' ? t('notifications.empty_unread') : t('notifications.empty')}
        header={
          <View className="gap-3">
            <ConnectionBanner offlineText={t('common:offline_body')} className="rounded-lg" />
            <GuideTarget id="notifications.filter">
              <Chips<Filter>
                value={filter}
                onChange={setFilter}
                options={[
                  { value: 'all', icon: 'bell', label: t('notifications.all') },
                  {
                    value: 'unread',
                    icon: 'mail',
                    label: t('notifications.unread'),
                    count: unread,
                  },
                ]}
              />
            </GuideTarget>
          </View>
        }
        renderItem={(n, _i, last) => (
          <GuideTarget id="notifications.row" enabled={_i === 0}>
            <NotificationRow notification={n} last={last} />
          </GuideTarget>
        )}
      />
    </>
  );
}

function NotificationRow({ notification: n, last }: { notification: AppNotification; last: boolean }) {
  const { t } = useTranslation('hub');
  const markRead = useMarkNotificationRead();
  const unread = !n.read_at;
  const body = notificationBody(n.body, t('notifications.billing_page'));
  const target =
    appRoute(n.action_url ?? (n.payload as { url?: unknown } | undefined)?.url) ?? notificationBodyRoute(n.body);

  const open = () => {
    void Haptics.selectionAsync();
    if (unread) markRead.mutate(n.uuid);
    if (target && target !== '/') router.push(target as Href);
  };

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={[unread ? t('notifications.unread') : null, n.title, body, formatRelative(n.created_at)]
        .filter(Boolean)
        .join(', ')}
      accessibilityHint={target ? t('notifications.open_hint') : undefined}
      onPress={open}
      className="active:bg-muted">
      <View className={cn('flex-row gap-3 px-4 py-3', !last && 'border-b border-border')}>
        <View className="items-center pt-1.5" style={{ width: 10 }}>
          {unread ? <View className="size-2.5 rounded-full bg-signal" /> : null}
        </View>
        <View className="min-w-0 flex-1 gap-0.5">
          <View className="flex-row items-center gap-2">
            <Text
              variant="callout"
              numberOfLines={2}
              className={cn('min-w-0 flex-1', unread ? 'font-sans-semibold' : 'font-sans-medium')}>
              {n.title}
            </Text>
            <Text variant="footnote" tone="muted" className="shrink-0">
              {formatRelative(n.created_at)}
            </Text>
          </View>
          {body ? (
            <Text variant="footnote" tone="muted" numberOfLines={3}>
              {body}
            </Text>
          ) : null}
        </View>
        {target && target !== '/' ? <Icon name="forward" size={16} tone="muted-foreground" /> : null}
      </View>
    </Pressable>
  );
}
