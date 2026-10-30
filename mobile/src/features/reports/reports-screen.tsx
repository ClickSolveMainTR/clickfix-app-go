import { changeTone } from './core';
import { ChannelLogo, hasChannelLogo } from '@/ui/brand-logo';
import { useChannelColor, channelIcon } from '@/ui/channel';
import { Icon } from '@/ui/icon';
import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { formatNumber } from '@/core/format';
import { useTeams } from '@/features/inbox/api';
import { formatDuration } from '@/features/inbox/helpers';
import { EmptyState } from '@/ui/empty-state';
import { Card } from '@/ui/card';
import { BarChart } from '@/ui/charts';
import { Chips } from '@/ui/chips';
import { cn } from '@/ui/cn';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { Select } from '@/ui/select';
import { useManualRefresh } from '@/ui/use-manual-refresh';
import { Text } from '@/ui/text';

import { type AnalyticsCards, delta, type Period, useDashboard } from './api';

type Scope = 'me' | 'team' | 'all';

/**
 * Reports lite (CF-79): the KPIs a lead checks on the go (volume, resolved,
 * backlog, first response, resolution, SLA, CSAT) for me, a team or the
 * whole organization, with the change against the previous period and the
 * volume trend. The full report builder stays on the panel.
 */
export function ReportsScreen() {
  const { t, i18n } = useTranslation('reports');
  const me = useSession((s) => s.me?.user?.uuid);
  const [days, setDays] = useState<Period>(7);
  const [scope, setScope] = useState<Scope>('all');
  const teams = useTeams(true);
  const [team, setTeam] = useState<string | null>(null);
  const dash = useDashboard(days, {
    agent: scope === 'me' ? me : undefined,
    team: scope === 'team' ? (team ?? teams.data?.items[0]?.uuid) : undefined,
  });
  const refresh = useManualRefresh(dash);
  const dur = (s: number | null | undefined) => (s == null ? '—' : formatDuration(Math.round(s), i18n.language));

  return (
    <>
      <Stack.Screen options={{ title: t('title') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <Chips<Scope>
          value={scope}
          onChange={setScope}
          options={(['all', 'team', 'me'] as const).map((s) => ({ value: s, label: t(`scope.${s}`) }))}
        />
        {scope === 'team' && teams.data?.items.length ? (
          <Select<string>
            label={t('team')}
            value={team ?? teams.data.items[0]?.uuid ?? null}
            options={teams.data.items.map((tm) => ({ value: tm.uuid, label: tm.name }))}
            onChange={setTeam}
          />
        ) : null}
        <Chips<string>
          value={String(days)}
          onChange={(v) => setDays(Number(v) as Period)}
          options={(['7', '30', '90'] as const).map((d) => ({
            value: d,
            label: t('days', { count: Number(d) }),
          }))}
        />
        <QueryState query={dash}>
          {(d) => {
            const c = d.cards;
            const p = d.comparison?.cards;
            const series = (d.series ?? []).map((s) => ({
              label: s.bucket ? new Date(s.bucket).toLocaleDateString(i18n.language, { weekday: 'short' }) : '',
              value: s.volume ?? 0,
            }));
            return c ? (
              <View className="gap-4">
                <View className="flex-row flex-wrap gap-3">
                  <Kpi label={t('kpi.volume')} value={formatNumber(c.volume)} change={delta(c.volume, p?.volume)} />
                  <Kpi
                    label={t('kpi.resolved')}
                    value={formatNumber(c.resolved)}
                    change={delta(c.resolved, p?.resolved)}
                  />
                  <Kpi
                    label={t('kpi.backlog')}
                    value={formatNumber(c.backlog)}
                    change={delta(c.backlog, p?.backlog)}
                    lowerIsBetter
                  />
                  <Kpi
                    label={t('kpi.first_response')}
                    value={dur(c.first_response_seconds)}
                    change={delta(c.first_response_seconds, p?.first_response_seconds)}
                    lowerIsBetter
                  />
                  <Kpi
                    label={t('kpi.resolution')}
                    value={dur(c.resolution_seconds)}
                    change={delta(c.resolution_seconds, p?.resolution_seconds)}
                    lowerIsBetter
                  />
                  <Kpi
                    label={t('kpi.sla')}
                    value={c.sla_hit_percent == null ? '—' : `%${formatNumber(c.sla_hit_percent, 1)}`}
                    change={delta(c.sla_hit_percent, p?.sla_hit_percent)}
                    hint={c.sla_breached ? t('kpi.sla_breached', { count: c.sla_breached }) : undefined}
                  />
                  <Kpi
                    label={t('kpi.csat')}
                    value={c.csat_average == null ? '—' : formatNumber(c.csat_average, 2, 1)}
                    change={delta(c.csat_average, p?.csat_average)}
                    hint={t('kpi.csat_responses', { count: c.csat_responses })}
                  />
                </View>
                {series.length > 1 ? (
                  <Card title={t('volume_trend')}>
                    <BarChart
                      data={series}
                      format={(v) => formatNumber(v)}
                      summary={t('volume_summary', {
                        values: series.map((s) => `${s.label} ${s.value}`).join(', '),
                      })}
                    />
                  </Card>
                ) : null}
                <Breakdown
                  title={t('by_channel')}
                  rows={d.by_channel ?? []}
                  channel
                  labelOf={(x) => t(`channels:types.${x}`, { defaultValue: x })}
                />
                {scope !== 'team' ? (
                  <Breakdown
                    title={t('by_team')}
                    rows={d.by_team ?? []}
                    labelOf={(x) => teams.data?.items.find((team) => team.uuid === x)?.name ?? t('none')}
                  />
                ) : null}
                <Text variant="footnote" tone="muted">
                  {t('footnote')}
                </Text>
              </View>
            ) : (
              <EmptyState icon="chart" title={t('empty')} />
            );
          }}
        </QueryState>
      </Screen>
    </>
  );
}

function Kpi({
  label,
  value,
  change,
  lowerIsBetter,
  hint,
}: {
  label: string;
  value: string;
  change: number | null;
  lowerIsBetter?: boolean;
  hint?: string;
}) {
  const { t } = useTranslation('reports');
  const tone = changeTone(change, lowerIsBetter);
  const pct = change == null ? null : `${change > 0 ? '+' : ''}${formatNumber(change * 100)}%`;
  return (
    <View
      accessible
      accessibilityLabel={[label, value, pct ? t('change', { pct }) : null, hint].filter(Boolean).join(', ')}
      className="min-w-[46%] flex-1 gap-1 rounded-lg bg-card p-4"
      style={{ borderCurve: 'continuous' }}>
      <Text variant="footnote" tone="muted" numberOfLines={1}>
        {label}
      </Text>
      <Text variant="title" className="tabular-nums" numberOfLines={1} adjustsFontSizeToFit>
        {value}
      </Text>
      <View className="flex-row items-center gap-1.5">
        {pct ? (
          <Text
            variant="footnote"
            className={cn(
              'font-sans-semibold',
              tone === 'neutral' ? 'text-muted-foreground' : tone === 'good' ? 'text-success' : 'text-destructive',
            )}>
            {pct}
          </Text>
        ) : null}
        {hint ? (
          <Text variant="footnote" tone="muted" numberOfLines={1} className="shrink">
            {hint}
          </Text>
        ) : null}
      </View>
    </View>
  );
}

/** Share by a dimension: one bar per row in one hue, the count as text beside it. */
function Breakdown({
  title,
  rows,
  labelOf,
  channel,
}: {
  title: string;
  rows: { dimension: string | null; volume: number }[];
  labelOf: (dimension: string) => string;
  channel?: boolean;
}) {
  const { t } = useTranslation('reports');
  const channelColor = useChannelColor();
  const top = [...rows].sort((a, b) => b.volume - a.volume).slice(0, 6);
  const max = Math.max(1, ...top.map((r) => r.volume));
  if (!top.length) return null;
  return (
    <Card title={title}>
      {top.map((r) => (
        <View
          key={r.dimension ?? '-'}
          accessible
          accessibilityLabel={`${r.dimension ? labelOf(r.dimension) : t('none')}: ${r.volume}`}
          className="gap-1">
          <View className="flex-row items-center gap-2">
            {channel && r.dimension && hasChannelLogo(r.dimension) ? (
              <ChannelLogo type={r.dimension} size={16} />
            ) : channel ? (
              <Icon name={channelIcon(r.dimension ?? '')} color={channelColor(r.dimension ?? '')} size={16} />
            ) : null}
            <Text variant="footnote" numberOfLines={1} className="flex-1">
              {r.dimension ? labelOf(r.dimension) : t('none')}
            </Text>
            <Text variant="footnote" className="font-sans-semibold tabular-nums">
              {formatNumber(r.volume)}
            </Text>
          </View>
          <View className="h-1.5 overflow-hidden rounded-full bg-muted">
            <View
              className="h-full rounded-full bg-signal"
              // An undefined backgroundColor would override the className colour, leaving the bar empty.
              style={
                channel
                  ? { width: `${(r.volume / max) * 100}%`, backgroundColor: channelColor(r.dimension ?? '') }
                  : { width: `${(r.volume / max) * 100}%` }
              }
            />
          </View>
        </View>
      ))}
    </Card>
  );
}

export type { AnalyticsCards };
