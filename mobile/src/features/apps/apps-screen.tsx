import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { P, useCan } from '@/core/auth/permissions';
import { EmptyState } from '@/ui/empty-state';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { type SidebarApp, useSidebarApps } from './api';
import { appHost, groupApps, placementKey, readable, scopeKey } from './core';

/** Name, where it lives and what it may do: one registry row, never a launcher. */
export function AppRow({ app, last }: { app: SidebarApp; last: boolean }) {
  const { t } = useTranslation('admin');
  const scopes = app.scopes.map((s) => {
    const key = scopeKey(s);
    return key ? t(`apps.scope_${key}`) : readable(s);
  });
  return (
    <ListRow
      icon="grid"
      title={app.name}
      titleLines={2}
      subtitle={[appHost(app.url), scopes.join(', ')].filter(Boolean).join(' · ') || undefined}
      last={last}
    />
  );
}

/**
 * Sidebar apps registered for the organization (CF-172), as a list.
 *
 * The apps themselves are NOT opened here. On the panel each one runs in a
 * sandboxed iframe (`frontend/src/features/sidebar-apps/components/
 * app-frame.tsx`): no top navigation or popups, messages accepted only from
 * that frame and its exact origin, the context token handed over only after
 * the frame says it is ready. The app has no WebView to host a frame with
 * those limits, and opening the app's address in a browser would run it
 * outside them. So the registry is shown, and the apps stay on the panel.
 * Discovery needs `tenant.conversations.read`, like there.
 */
export function AppsScreen() {
  const { t } = useTranslation('admin');
  const can = useCan();
  const apps = useSidebarApps();
  const refresh = useManualRefresh(apps);
  if (!can(P.conversationsRead))
    return (
      <>
        <Stack.Screen options={{ title: t('apps.title') }} />
        <Screen>
          <EmptyState icon="grid" title={t('apps.no_access')} />
        </Screen>
      </>
    );
  return (
    <>
      <Stack.Screen options={{ title: t('apps.title') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={apps}>
          {(d) => {
            const groups = groupApps(d.items ?? []);
            if (!groups.length) return <EmptyState icon="grid" title={t('apps.empty')} body={t('apps.panel_only')} />;
            return (
              <View className="gap-3">
                {groups.map((g) => {
                  const placement = placementKey(g.placement);
                  return (
                    <View key={g.placement}>
                      <SectionHeader title={placement ? t(`apps.placement_${placement}`) : readable(g.placement)} />
                      <List>
                        {g.apps.map((app, i) => (
                          <AppRow key={app.uuid} app={app} last={i === g.apps.length - 1} />
                        ))}
                      </List>
                    </View>
                  );
                })}
                <Text variant="footnote" tone="muted" className="px-4">
                  {t('apps.panel_only')}
                </Text>
              </View>
            );
          }}
        </QueryState>
      </Screen>
    </>
  );
}

/**
 * "Apps" card of a conversation: the enabled apps placed in the conversation
 * sidebar, by name. Nothing is launched (see `AppsScreen`); the card is not
 * rendered when there is no such app or the list cannot be read.
 */
export function ConversationApps() {
  const { t } = useTranslation('admin');
  const apps = useSidebarApps();
  const placed = (apps.data?.items ?? []).filter((a) => a.enabled && a.placement === 'conversation_sidebar');
  if (!placed.length) return null;
  return (
    <View>
      <SectionHeader title={t('apps.conversation_title')} />
      <List>
        {placed.map((app, i) => (
          <AppRow key={app.uuid} app={app} last={i === placed.length - 1} />
        ))}
      </List>
      <Text variant="footnote" tone="muted" className="px-4 pt-2">
        {[t('apps.conversation_hint'), t('apps.panel_only')].join(' ')}
      </Text>
    </View>
  );
}
