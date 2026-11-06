import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { formatNumber, formatRelative, formatShort } from '@/core/format';
import { EmptyState } from '@/ui/empty-state';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import {
  type UptimeMonitor,
  useEscalationPolicies,
  useOncallScope,
  useStatusComponents,
  useUptimeMonitors,
  useUptimeResults,
} from './api';
import { resultsSummary } from './oncall-core';
import { NoAccess, useSetupLabels } from './oncall-setup-parts';

/** How many recent checks the detail lists; the summary covers the same 30 the panel's sparkline does. */
const SUMMARY_CHECKS = 30;
const LISTED_CHECKS = 10;

/**
 * Uptime monitors, read-only (CF-168): the panel's "Uptime" tab
 * (`frontend/src/features/oncall/components/monitors-tab.tsx`). Monitors are
 * created and edited on the panel.
 */
export function MonitorsScreen() {
  const { t } = useTranslation('team');
  const { canSetup } = useOncallScope();
  const monitors = useUptimeMonitors();
  const refresh = useManualRefresh(monitors);
  const labels = useSetupLabels();
  if (!canSetup) return <NoAccess title={t('setup.monitors')} body={t('setup.no_access')} />;
  return (
    <>
      <Stack.Screen options={{ title: t('setup.monitors') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={monitors}>
          {(d) =>
            d.items.length ? (
              <View className="gap-2">
                <List>
                  {d.items.map((m, i) => {
                    const state = labels.monitor(m);
                    return (
                      <ListRow
                        key={m.uuid}
                        title={m.name}
                        subtitle={
                          <View className="gap-0.5">
                            <Text variant="footnote" tone="muted" numberOfLines={1}>
                              {m.url}
                            </Text>
                            <Text variant="footnote" tone="muted" numberOfLines={1}>
                              {[
                                t('setup.every', { interval: labels.interval(m.interval_seconds) }),
                                m.last_checked_at
                                  ? t('setup.checked', { at: formatRelative(m.last_checked_at) })
                                  : null,
                              ]
                                .filter(Boolean)
                                .join(' · ')}
                            </Text>
                          </View>
                        }
                        trailing={<StatusPill label={state.label} tone={state.tone} />}
                        onPress={() => router.push({ pathname: '/more/oncall-monitor', params: { uuid: m.uuid } })}
                        last={i === d.items.length - 1}
                      />
                    );
                  })}
                </List>
                <Text variant="footnote" tone="muted" className="px-4">
                  {t('setup.read_only')}
                </Text>
              </View>
            ) : (
              <EmptyState icon="chart" title={t('setup.monitors_empty')} body={t('setup.read_only')} />
            )
          }
        </QueryState>
      </Screen>
    </>
  );
}

/** One monitor: what it checks, what it drives and its recent checks. */
export function MonitorScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t } = useTranslation('team');
  const { canSetup } = useOncallScope();
  const monitors = useUptimeMonitors();
  const refresh = useManualRefresh(monitors);
  if (!canSetup) return <NoAccess title={t('setup.monitor')} body={t('setup.no_access')} />;
  const monitor = monitors.data?.items.find((m) => m.uuid === uuid);
  return (
    <>
      <Stack.Screen options={{ title: monitor?.name ?? t('setup.monitor') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={monitors}>
          {() =>
            monitor ? <MonitorDetail monitor={monitor} /> : <EmptyState icon="chart" title={t('setup.not_found')} />
          }
        </QueryState>
      </Screen>
    </>
  );
}

function MonitorDetail({ monitor: m }: { monitor: UptimeMonitor }) {
  const { t } = useTranslation('team');
  const labels = useSetupLabels();
  const state = labels.monitor(m);
  // Names of the linked component and policy; each list is read only by a member who may read it.
  const components = useStatusComponents();
  const policies = useEscalationPolicies();
  const results = useUptimeResults(m.uuid, m.last_checked_at);
  const component = components.data?.items?.find((c) => c.uuid === m.component_uuid)?.name;
  const policy = m.policy_uuid ? policies.data?.items.find((p) => p.uuid === m.policy_uuid)?.name : null;
  const recent = (results.data?.items ?? []).slice(0, SUMMARY_CHECKS);
  const summary = resultsSummary(recent);
  return (
    <View className="gap-4">
      <List>
        <ListRow
          title={m.name}
          titleLines={2}
          subtitle={m.url}
          trailing={<StatusPill label={state.label} tone={state.tone} />}
        />
        <Row label={t('setup.interval')} value={labels.interval(m.interval_seconds)} />
        <Row label={t('setup.expected_status')} value={String(m.expected_status)} />
        {m.keyword ? <Row label={t('setup.keyword')} value={m.keyword} /> : null}
        <Row label={t('setup.component')} value={component ?? '—'} />
        <Row label={t('setup.policy')} value={m.policy_uuid ? (policy ?? '—') : t('setup.no_policy')} />
        <Row
          label={t('setup.last_check')}
          value={m.last_checked_at ? formatShort(m.last_checked_at) : t('setup.no_checks')}
        />
        <Row label={t('setup.next_check')} value={m.enabled ? formatShort(m.next_check_at) : '—'} last />
      </List>
      <View>
        <SectionHeader title={t('setup.checks')} />
        <QueryState query={results} rows={3}>
          {() =>
            summary ? (
              <View className="gap-2">
                <List>
                  <Row
                    label={t('setup.uptime', { count: summary.total })}
                    value={t('setup.percent', { value: formatNumber(summary.ratio * 100, 1) })}
                  />
                  <Row label={t('setup.latency')} value={t('setup.ms', { ms: summary.latency })} last />
                </List>
                <List>
                  {recent.slice(0, LISTED_CHECKS).map((r, i, all) => (
                    <ListRow
                      key={r.uuid}
                      title={formatShort(r.checked_at)}
                      subtitle={[r.status_code ? `HTTP ${r.status_code}` : null, t('setup.ms', { ms: r.latency_ms })]
                        .filter(Boolean)
                        .join(' · ')}
                      trailing={
                        <StatusPill
                          label={r.up ? t('setup.check_up') : t('setup.check_down')}
                          tone={r.up ? 'done' : 'problem'}
                        />
                      }
                      last={i === all.length - 1}
                    />
                  ))}
                </List>
              </View>
            ) : (
              <List>
                <ListRow title={t('setup.no_checks')} last />
              </List>
            )
          }
        </QueryState>
      </View>
    </View>
  );
}

function Row({ label, value, last }: { label: string; value: string; last?: boolean }) {
  return (
    <ListRow
      title={label}
      trailing={
        <Text tone="muted" numberOfLines={1} className="max-w-[60%] shrink text-right">
          {value}
        </Text>
      }
      last={last}
    />
  );
}
