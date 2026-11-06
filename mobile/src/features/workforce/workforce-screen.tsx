import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { P, useCan } from '@/core/auth/permissions';
import { appLocale, formatNumber } from '@/core/format';
import { formatDuration } from '@/features/inbox/helpers';
import { useMemberNames } from '@/features/team/names';
import { Avatar } from '@/ui/avatar';
import { Card, Stat } from '@/ui/card';
import { Chips } from '@/ui/chips';
import { EmptyState } from '@/ui/empty-state';
import { SwitchRow } from '@/ui/fields';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { useForecast, useTimesheets, useWorkforceScope } from './api';
import {
  agentTimes,
  bucketLabel,
  coverage,
  forecastRows,
  forecastTotals,
  type Granularity,
  HORIZONS,
  MODEL,
  TIMESHEET_RANGES,
} from './core';

type Tab = 'forecast' | 'timesheets';

function NoAccess({ title, body }: { title: string; body: string }) {
  return (
    <>
      <Stack.Screen options={{ title }} />
      <Screen>
        <EmptyState icon="calendar" title={body} />
      </Screen>
    </>
  );
}

/**
 * Workforce planning, read-only (CF-170): the panel's Workforce page
 * (`frontend/src/features/workforce/components/workforce-page.tsx`).
 * `tenant.schedules.read` opens the forecast; the timesheets tab is there
 * only with `tenant.schedules.manage`, like on the panel.
 */
export function WorkforceScreen() {
  const { t } = useTranslation('workforce');
  const { canForecast, canTimesheets } = useWorkforceScope();
  const [tab, setTab] = useState<Tab>('forecast');
  if (!canForecast) return <NoAccess title={t('title')} body={t('forbidden')} />;
  const shown: Tab = tab === 'timesheets' && canTimesheets ? 'timesheets' : 'forecast';
  const tabs = canTimesheets ? (
    <Chips<Tab>
      value={shown}
      onChange={setTab}
      options={(['forecast', 'timesheets'] as const).map((k) => ({ value: k, label: t(`tabs.${k}`) }))}
    />
  ) : null;
  return (
    <>
      <Stack.Screen options={{ title: t('title') }} />
      {shown === 'timesheets' ? <Timesheets tabs={tabs} /> : <ForecastView tabs={tabs} />}
    </>
  );
}

const num = (value: number) => formatNumber(value, 1);

function ForecastView({ tabs }: { tabs: React.ReactNode }) {
  const { t } = useTranslation('workforce');
  const [granularity, setGranularity] = useState<Granularity>('day');
  const [days, setDays] = useState(7);
  const [onlyGaps, setOnlyGaps] = useState(false);
  // A horizon the other granularity does not offer falls back to its first one.
  const horizon = HORIZONS[granularity].includes(days) ? days : HORIZONS[granularity][0]!;
  const query = useForecast(horizon, granularity);
  const refresh = useManualRefresh(query);
  const daily = granularity === 'day';
  const unit = daily ? t('forecast.unit_agent_hours') : t('forecast.unit_agents');
  return (
    <Screen refreshControl={<RefreshControl {...refresh} />}>
      {tabs}
      <Chips<Granularity>
        value={granularity}
        onChange={setGranularity}
        options={[
          { value: 'day', label: t('forecast.daily') },
          { value: 'hour', label: t('forecast.hourly') },
        ]}
      />
      <Chips<string>
        value={String(horizon)}
        onChange={(v) => setDays(Number(v))}
        options={HORIZONS[granularity].map((d) => ({
          value: String(d),
          label: t('work:workforce.days', { count: d }),
        }))}
      />
      <QueryState query={query}>
        {(d) => {
          const all = forecastRows(d.buckets);
          const totals = forecastTotals(all);
          const rows = onlyGaps ? all.filter((r) => r.gap > 0) : all;
          const zone = d.timezone || 'UTC';
          return (
            <View className="gap-4">
              <Card>
                <View className="flex-row gap-3">
                  <Stat label={t('forecast.total_volume')} value={num(totals.volume)} />
                  <Stat
                    label={daily ? t('forecast.peak_required_day') : t('forecast.peak_required')}
                    value={`${totals.capacityLimited ? '≥ ' : ''}${num(totals.peakRequired)}`}
                  />
                </View>
                <View className="flex-row gap-3">
                  <Stat
                    label={daily ? t('forecast.gap_days') : t('forecast.gap_hours')}
                    value={String(totals.gapBuckets)}
                  />
                  <Stat
                    label={t('forecast.total_gap')}
                    value={`${totals.capacityLimited ? '≥ ' : ''}${num(totals.gap)}`}
                  />
                </View>
                <Text variant="footnote" tone="muted">
                  {[unit, t('forecast.history', { count: d.history_conversations })].join(' · ')}
                </Text>
              </Card>
              {d.history_conversations === 0 ? (
                <Card title={t('forecast.no_history_title')}>
                  <Text tone="muted">{t('forecast.no_history')}</Text>
                </Card>
              ) : null}
              {totals.capacityLimited ? (
                <Card title={t('forecast.capacity_title')}>
                  <Text tone="muted">{t('forecast.capacity_limited')}</Text>
                </Card>
              ) : null}
              <SwitchRow label={t('forecast.only_gaps')} value={onlyGaps} onChange={setOnlyGaps} />
              {rows.length ? (
                <List>
                  {rows.map((r, i) => {
                    const state = coverage(r);
                    return (
                      <ListRow
                        key={r.key}
                        title={bucketLabel(r, granularity, zone, appLocale(), horizon > 1)}
                        subtitle={[
                          `${t('forecast.volume')} ${num(r.volume)}`,
                          `${t('forecast.required')} ${r.capacityLimited ? '≥ ' : ''}${num(r.required)}`,
                          `${t('forecast.scheduled')} ${num(r.scheduled)}`,
                        ].join(' · ')}
                        trailing={
                          <StatusPill
                            label={state === 'gap' ? `${t('forecast.gap')} ${num(r.gap)}` : t(`forecast.${state}`)}
                            tone={state === 'gap' ? 'problem' : 'done'}
                          />
                        }
                        last={i === rows.length - 1}
                      />
                    );
                  })}
                </List>
              ) : (
                <EmptyState icon="check" title={onlyGaps ? t('forecast.no_gaps') : t('forecast.empty')} />
              )}
              <Text variant="footnote" tone="muted" className="px-4">
                {[
                  daily ? null : t('forecast.timezone', { zone }),
                  t('work:workforce.model', {
                    target: Math.round(MODEL.target_sla * 100),
                    wait: MODEL.sla_seconds,
                    handle: MODEL.handle_seconds,
                  }),
                  t('forecast.model_hint'),
                ]
                  .filter(Boolean)
                  .join('\n\n')}
              </Text>
            </View>
          );
        }}
      </QueryState>
    </Screen>
  );
}

const rangeOf = (raw: string | undefined) => {
  const days = Number(raw);
  return (TIMESHEET_RANGES as readonly number[]).includes(days) ? days : TIMESHEET_RANGES[0];
};

function useDuration() {
  const { i18n } = useTranslation();
  return (seconds: number) => formatDuration(seconds, i18n.language);
}

function Timesheets({ tabs }: { tabs: React.ReactNode }) {
  const { t } = useTranslation('workforce');
  const [days, setDays] = useState<number>(TIMESHEET_RANGES[0]);
  const query = useTimesheets(days);
  const refresh = useManualRefresh(query);
  const nameOf = useMemberNames();
  const dur = useDuration();
  return (
    <Screen refreshControl={<RefreshControl {...refresh} />}>
      {tabs}
      <Chips<string>
        value={String(days)}
        onChange={(v) => setDays(Number(v))}
        options={TIMESHEET_RANGES.map((d) => ({
          value: String(d),
          label: t('work:workforce.last_days', { count: d }),
        }))}
      />
      <QueryState query={query}>
        {(rows) => {
          const agents = agentTimes(rows);
          if (!agents.length) return <EmptyState icon="calendar" title={t('timesheets.empty')} />;
          return (
            <View className="gap-2">
              <List>
                {agents.map((a, i) => (
                  <ListRow
                    key={a.agent}
                    leading={<Avatar name={nameOf(a.agent)} size={36} />}
                    title={nameOf(a.agent)}
                    subtitle={[
                      `${t('state.active')} ${dur(a.active)}`,
                      `${t('state.handling')} ${dur(a.handling)}`,
                      `${t('state.away')} ${dur(a.away)}`,
                    ].join(' · ')}
                    trailing={<Text className="tabular-nums">{dur(a.active + a.handling)}</Text>}
                    onPress={() =>
                      router.push({ pathname: '/more/workforce-agent', params: { agent: a.agent, days: String(days) } })
                    }
                    last={i === agents.length - 1}
                  />
                ))}
              </List>
              <Text variant="footnote" tone="muted" className="px-4">
                {[t('timesheets.working'), t('timesheets.hint')].join('. ')}
              </Text>
            </View>
          );
        }}
      </QueryState>
    </Screen>
  );
}

/** One agent's time in the range: totals, then handling time per conversation, longest first. */
export function AgentTimeScreen() {
  const params = useLocalSearchParams<{ agent: string; days?: string }>();
  const { t } = useTranslation('workforce');
  const can = useCan();
  const { canTimesheets } = useWorkforceScope();
  const days = rangeOf(params.days);
  const query = useTimesheets(days, !!params.agent);
  const refresh = useManualRefresh(query);
  const nameOf = useMemberNames();
  const dur = useDuration();
  const name = nameOf(params.agent);
  if (!canTimesheets) return <NoAccess title={t('tabs.timesheets')} body={t('work:workforce.no_timesheets')} />;
  return (
    <>
      <Stack.Screen options={{ title: name === '—' ? t('tabs.timesheets') : name }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={query}>
          {(rows) => {
            const a = agentTimes(rows).find((x) => x.agent === params.agent);
            if (!a) return <EmptyState icon="calendar" title={t('timesheets.empty')} />;
            const openable = can(P.conversationsRead);
            return (
              <View className="gap-4">
                <Card title={t('work:workforce.last_days', { count: days })}>
                  <View className="flex-row gap-3">
                    <Stat label={t('timesheets.working')} value={dur(a.active + a.handling)} />
                    <Stat label={t('state.away')} value={dur(a.away)} />
                  </View>
                  <View className="flex-row gap-3">
                    <Stat label={t('state.active')} value={dur(a.active)} />
                    <Stat label={t('state.handling')} value={dur(a.handling)} />
                  </View>
                </Card>
                {a.conversations.length ? (
                  <View>
                    <SectionHeader title={t('state.handling')} />
                    <List>
                      {a.conversations.map((c, i) => (
                        <ListRow
                          key={c.uuid ?? `deleted-${i}`}
                          icon="chat"
                          title={
                            c.uuid ? t('timesheets.conversation', { id: c.uuid.slice(0, 8) }) : t('timesheets.deleted')
                          }
                          trailing={
                            <Text tone="muted" className="tabular-nums">
                              {dur(c.seconds)}
                            </Text>
                          }
                          onPress={c.uuid && openable ? () => router.push(`/conversation/${c.uuid}`) : undefined}
                          last={i === a.conversations.length - 1}
                        />
                      ))}
                    </List>
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
