import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { EmptyState } from '@/ui/empty-state';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { type StatusComponent, useOncallScope, useStatusComponents, useUptimeMonitors } from './api';
import { componentStatus, groupComponents, readable } from './oncall-core';
import { NoAccess, useSetupLabels } from './oncall-setup-parts';

/** Status of a component as text and tone; one the app does not know is shown as readable text. */
function useComponentStatus() {
  const { t } = useTranslation('team');
  return (c: StatusComponent) => {
    const status = componentStatus(c.status);
    return { tone: status.tone, label: status.key ? t(`setup.component_${status.key}`) : readable(c.status) };
  };
}

/**
 * Status page components, read-only (CF-168): the components the panel's
 * monitors drive (`GET /v1/tenant/status/components`, the list the panel's
 * monitor dialog reads), grouped the way the status page shows them.
 */
export function StatusComponentsScreen() {
  const { t } = useTranslation('team');
  const { canComponents } = useOncallScope();
  const components = useStatusComponents();
  const refresh = useManualRefresh(components);
  const statusOf = useComponentStatus();
  if (!canComponents) return <NoAccess title={t('setup.components')} body={t('setup.no_access_components')} />;
  return (
    <>
      <Stack.Screen options={{ title: t('setup.components') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={components}>
          {(d) => {
            const groups = groupComponents((d.items ?? []).filter((c) => c.uuid));
            if (!groups.length)
              return <EmptyState icon="check" title={t('setup.components_empty')} body={t('setup.read_only')} />;
            return (
              <View className="gap-3">
                {groups.map((g) => (
                  <View key={g.group}>
                    {g.group ? <SectionHeader title={g.group} /> : null}
                    <List>
                      {g.items.map((c, i) => {
                        const status = statusOf(c);
                        return (
                          <ListRow
                            key={c.uuid}
                            title={c.name || '—'}
                            subtitle={c.description || undefined}
                            trailing={<StatusPill label={status.label} tone={status.tone} />}
                            onPress={() =>
                              router.push({ pathname: '/more/status-component', params: { uuid: c.uuid ?? '' } })
                            }
                            last={i === g.items.length - 1}
                          />
                        );
                      })}
                    </List>
                  </View>
                ))}
                <Text variant="footnote" tone="muted" className="px-4">
                  {t('setup.read_only')}
                </Text>
              </View>
            );
          }}
        </QueryState>
      </Screen>
    </>
  );
}

/** One component: status, group, description and the monitors that drive it. */
export function StatusComponentScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t } = useTranslation('team');
  const { canComponents, canSetup } = useOncallScope();
  const components = useStatusComponents();
  // Monitors are listed only for a member who may read them; nobody else's app asks for them.
  const monitors = useUptimeMonitors(canComponents);
  const refresh = useManualRefresh(components);
  const statusOf = useComponentStatus();
  const labels = useSetupLabels();
  if (!canComponents) return <NoAccess title={t('setup.component_title')} body={t('setup.no_access_components')} />;
  const component = components.data?.items?.find((c) => c.uuid === uuid);
  const driving = (monitors.data?.items ?? []).filter((m) => m.component_uuid === uuid);
  return (
    <>
      <Stack.Screen options={{ title: component?.name || t('setup.component_title') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={components}>
          {() => {
            if (!component) return <EmptyState icon="check" title={t('setup.not_found')} />;
            const status = statusOf(component);
            return (
              <View className="gap-4">
                <List>
                  <ListRow
                    title={component.name || '—'}
                    titleLines={2}
                    subtitle={component.description || undefined}
                    trailing={<StatusPill label={status.label} tone={status.tone} />}
                  />
                  <ListRow title={t('setup.group')} subtitle={component.group_name || t('setup.no_group')} last />
                </List>
                {canSetup ? (
                  <View>
                    <SectionHeader title={t('setup.monitors')} />
                    <QueryState query={monitors} rows={2}>
                      {() => (
                        <List>
                          {driving.length ? (
                            driving.map((m, i) => {
                              const state = labels.monitor(m);
                              return (
                                <ListRow
                                  key={m.uuid}
                                  title={m.name}
                                  subtitle={m.url}
                                  trailing={<StatusPill label={state.label} tone={state.tone} />}
                                  onPress={() =>
                                    router.push({ pathname: '/more/oncall-monitor', params: { uuid: m.uuid } })
                                  }
                                  last={i === driving.length - 1}
                                />
                              );
                            })
                          ) : (
                            <ListRow title={t('setup.component_no_monitors')} last />
                          )}
                        </List>
                      )}
                    </QueryState>
                  </View>
                ) : null}
              </View>
            );
          }}
        </QueryState>
      </Screen>
    </>
  );
}
