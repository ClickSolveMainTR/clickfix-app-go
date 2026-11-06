import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { formatNumber } from '@/core/format';
import { formatDuration } from '@/features/inbox/helpers';
import { useTeamPresence } from '@/features/team/api';
import { useMemberNames } from '@/features/team/names';
import { Avatar } from '@/ui/avatar';
import { Chips } from '@/ui/chips';
import { EmptyState } from '@/ui/empty-state';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { Select } from '@/ui/select';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import {
  type CoachingItem,
  ITEMS_PAGE,
  useCalibrations,
  useCoachingItems,
  useCoachingScope,
  useGameRules,
  useLeaderboard,
  useLeaderboardPreference,
} from './api';
import {
  groupItems,
  itemState,
  kindKey,
  LEADERBOARD_RANGES,
  metricKey,
  readable,
  shortSeconds,
  thresholdParts,
} from './core';
import { coachingDate } from './dates';

const TABS = ['leaderboard', 'rules', 'calibrations', 'coaching'] as const;
type Tab = (typeof TABS)[number];

/** Shown instead of a coaching screen the member may not read (also when opened by a link). */
export function NoAccess({ title, body }: { title: string; body: string }) {
  return (
    <>
      <Stack.Screen options={{ title }} />
      <Screen>
        <EmptyState icon="chart" title={body} />
      </Screen>
    </>
  );
}

/**
 * Coaching and gamification, read-only (CF-170): the panel's Coaching page
 * (`frontend/src/features/coaching/components/coaching-page.tsx`) with the
 * same tabs per permission: leaderboard and badge rules with
 * `tenant.schedules.read`, calibration and coaching with
 * `tenant.quality.manage`. Nothing is created or changed here.
 */
export function CoachingScreen() {
  const { t } = useTranslation('coaching');
  const { canGame, canQuality } = useCoachingScope();
  const tabs = TABS.filter((k) => (k === 'leaderboard' || k === 'rules' ? canGame : canQuality));
  const [picked, setPicked] = useState<Tab>('leaderboard');
  const tab = tabs.includes(picked) ? picked : tabs[0];
  if (!tab) return <NoAccess title={t('title')} body={t('forbidden')} />;
  const chips = (
    <Chips<Tab> value={tab} onChange={setPicked} options={tabs.map((k) => ({ value: k, label: t(`tabs.${k}`) }))} />
  );
  return (
    <>
      <Stack.Screen options={{ title: t('title') }} />
      {tab === 'leaderboard' ? (
        <Leaderboard tabs={chips} />
      ) : tab === 'rules' ? (
        <Rules tabs={chips} />
      ) : tab === 'calibrations' ? (
        <Calibrations tabs={chips} />
      ) : (
        <Items tabs={chips} />
      )}
    </>
  );
}

function Leaderboard({ tabs }: { tabs: React.ReactNode }) {
  const { t } = useTranslation('coaching');
  const [days, setDays] = useState<number>(30);
  const query = useLeaderboard(days);
  const preference = useLeaderboardPreference();
  const refresh = useManualRefresh(query);
  const nameOf = useMemberNames();
  return (
    <Screen refreshControl={<RefreshControl {...refresh} />}>
      {tabs}
      <Chips<string>
        value={String(days)}
        onChange={(v) => setDays(Number(v))}
        options={LEADERBOARD_RANGES.map((d) => ({
          value: String(d),
          label: t('work:coaching.last_days', { count: d }),
        }))}
      />
      {preference.data?.opted_out ? (
        <List>
          <ListRow
            icon="alert"
            title={t('work:coaching.opted_out')}
            subtitle={t('work:coaching.opted_out_hint')}
            last
          />
        </List>
      ) : null}
      <QueryState query={query}>
        {(d) =>
          d.items.length ? (
            <View className="gap-2">
              <List>
                {d.items.map((s, i) => (
                  <ListRow
                    key={s.agent_uuid}
                    leading={
                      <Text
                        tone={s.rank <= 3 && s.points > 0 ? 'primary' : 'muted'}
                        className="w-7 text-center tabular-nums">
                        {s.rank}
                      </Text>
                    }
                    title={nameOf(s.agent_uuid)}
                    subtitle={
                      s.badges.length
                        ? s.badges
                            .map(
                              (b) => `${b.name} (${t('leaderboard.badge_points', { points: formatNumber(b.points) })})`,
                            )
                            .join(', ')
                        : undefined
                    }
                    trailing={<Text className="tabular-nums">{formatNumber(s.points)}</Text>}
                    last={i === d.items.length - 1}
                  />
                ))}
              </List>
              <Text variant="footnote" tone="muted" className="px-4">
                {[
                  d.total > d.items.length
                    ? t('work:coaching.first_of', { shown: d.items.length, total: d.total })
                    : null,
                  t('leaderboard.hint'),
                ]
                  .filter(Boolean)
                  .join('\n\n')}
              </Text>
            </View>
          ) : (
            <EmptyState icon="chart" title={t('leaderboard.empty')} />
          )
        }
      </QueryState>
    </Screen>
  );
}

function Rules({ tabs }: { tabs: React.ReactNode }) {
  const { t, i18n } = useTranslation('coaching');
  const query = useGameRules();
  const refresh = useManualRefresh(query);
  const threshold = (metric: string, value: number) => {
    const p = thresholdParts(metric, value);
    // Like the panel: a first response target under a minute reads in seconds, not as "0 min".
    const seconds = p.seconds ? shortSeconds(value) : null;
    const text =
      seconds !== null
        ? t('work:coaching.seconds', { count: seconds })
        : p.seconds
          ? formatDuration(value, i18n.language)
          : formatNumber(value, 2);
    return `${p.sign} ${text}${p.scale ? ` / ${p.scale}` : ''}`;
  };
  return (
    <Screen refreshControl={<RefreshControl {...refresh} />}>
      {tabs}
      <QueryState query={query}>
        {(rules) =>
          rules.length ? (
            <View className="gap-2">
              <List>
                {rules.map((r, i) => {
                  const metric = metricKey(r.metric);
                  return (
                    <ListRow
                      key={r.uuid}
                      title={r.name}
                      subtitle={[
                        `${metric ? t(`metric.${metric}`) : readable(r.metric)} ${threshold(r.metric, r.threshold)}`,
                        t('leaderboard.badge_points', { points: formatNumber(r.points) }),
                        `${t('rules.min_samples')} ${formatNumber(r.min_samples)}`,
                      ].join(' · ')}
                      trailing={
                        <StatusPill
                          label={r.enabled ? t('rules.enabled') : t('work:coaching.rule_off')}
                          tone={r.enabled ? 'done' : 'neutral'}
                        />
                      }
                      last={i === rules.length - 1}
                    />
                  );
                })}
              </List>
              <Text variant="footnote" tone="muted" className="px-4">
                {[t('rules.description'), t('work:coaching.read_only')].join('\n\n')}
              </Text>
            </View>
          ) : (
            <EmptyState icon="chart" title={t('rules.empty_title')} body={t('work:coaching.read_only')} />
          )
        }
      </QueryState>
    </Screen>
  );
}

function Calibrations({ tabs }: { tabs: React.ReactNode }) {
  const { t } = useTranslation('coaching');
  const query = useCalibrations();
  const refresh = useManualRefresh(query);
  return (
    <Screen refreshControl={<RefreshControl {...refresh} />}>
      {tabs}
      <QueryState query={query}>
        {(d) =>
          d.items.length ? (
            <View className="gap-2">
              <List>
                {d.items.map((c, i) => (
                  <ListRow
                    key={c.uuid}
                    title={c.name}
                    subtitle={coachingDate(c.created_at)}
                    trailing={
                      <StatusPill
                        label={c.closed_at ? t('calibrations.status_closed') : t('calibrations.status_open')}
                        tone={c.closed_at ? 'neutral' : 'progress'}
                      />
                    }
                    onPress={() => router.push({ pathname: '/more/coaching-calibration', params: { uuid: c.uuid } })}
                    last={i === d.items.length - 1}
                  />
                ))}
              </List>
              <Text variant="footnote" tone="muted" className="px-4">
                {[
                  d.total > d.items.length
                    ? t('work:coaching.first_of', { shown: d.items.length, total: d.total })
                    : null,
                  t('calibrations.description'),
                  t('work:coaching.read_only'),
                ]
                  .filter(Boolean)
                  .join('\n\n')}
              </Text>
            </View>
          ) : (
            <EmptyState icon="chart" title={t('calibrations.empty_title')} body={t('work:coaching.read_only')} />
          )
        }
      </QueryState>
    </Screen>
  );
}

function Items({ tabs }: { tabs: React.ReactNode }) {
  const { t, i18n } = useTranslation('coaching');
  const presence = useTeamPresence();
  // One name lookup and one "now" for every row (overdue compares against when the tab was opened).
  const nameOf = useMemberNames();
  const [now] = useState(() => Date.now());
  const [agent, setAgent] = useState<string | null>(null);
  const query = useCoachingItems(agent ?? '');
  const refresh = useManualRefresh(query);
  const members = [...(presence.data?.items ?? [])].sort((a, b) => a.name.localeCompare(b.name, i18n.language));
  return (
    <Screen refreshControl={agent ? <RefreshControl {...refresh} /> : undefined}>
      {tabs}
      <Select<string>
        label={t('filters.agent')}
        value={agent}
        placeholder={t('filters.pick_agent')}
        options={members.map((m) => ({ value: m.user_uuid, label: m.name }))}
        onChange={setAgent}
        searchable
      />
      {agent ? (
        <QueryState query={query}>
          {(d) => {
            const g = groupItems(d.items);
            return (
              <View className="gap-4">
                <View>
                  <SectionHeader title={t('items.goals')} />
                  {g.goals.length ? (
                    <View className="gap-3">
                      {g.goals.map((goal) => (
                        <List key={goal.uuid}>
                          <ItemRow item={goal} nameOf={nameOf} now={now} last={!g.tasks.get(goal.uuid)?.length} />
                          {(g.tasks.get(goal.uuid) ?? []).map((task, i, all) => (
                            <ItemRow
                              key={task.uuid}
                              item={task}
                              nameOf={nameOf}
                              now={now}
                              indent
                              last={i === all.length - 1}
                            />
                          ))}
                        </List>
                      ))}
                    </View>
                  ) : (
                    <List>
                      <ListRow title={t('items.no_goals')} last />
                    </List>
                  )}
                </View>
                <View>
                  <SectionHeader title={t('items.notes')} />
                  <List>
                    {g.notes.length ? (
                      g.notes.map((n, i) => (
                        <ItemRow key={n.uuid} item={n} nameOf={nameOf} now={now} last={i === g.notes.length - 1} />
                      ))
                    ) : (
                      <ListRow title={t('items.no_notes')} last />
                    )}
                  </List>
                </View>
                {g.other.length ? (
                  <View>
                    <SectionHeader title={t('work:coaching.other')} />
                    <List>
                      {g.other.map((o, i) => (
                        <ItemRow
                          key={o.uuid}
                          item={o}
                          nameOf={nameOf}
                          now={now}
                          showKind
                          last={i === g.other.length - 1}
                        />
                      ))}
                    </List>
                  </View>
                ) : null}
                <Text variant="footnote" tone="muted" className="px-4">
                  {[
                    d.total > d.items.length ? t('items.truncated', { count: ITEMS_PAGE }) : null,
                    t('work:coaching.read_only'),
                  ]
                    .filter(Boolean)
                    .join('\n\n')}
                </Text>
              </View>
            );
          }}
        </QueryState>
      ) : (
        <EmptyState icon="customers" title={t('items.pick_title')} body={t('items.pick')} />
      )}
    </Screen>
  );
}

/** One note, goal or task: its text, who wrote it and its date state. */
function ItemRow({
  item,
  nameOf,
  now,
  last,
  indent,
  showKind,
}: {
  item: CoachingItem;
  nameOf: (uuid: string | null | undefined) => string;
  now: number;
  last: boolean;
  indent?: boolean;
  showKind?: boolean;
}) {
  const { t } = useTranslation('coaching');
  const state = itemState(item, now);
  const kind = kindKey(item.kind);
  const coach = item.coach_uuid ? nameOf(item.coach_uuid) : null;
  return (
    <ListRow
      leading={indent ? <View className="w-4" /> : coach ? <Avatar name={coach} size={28} /> : undefined}
      title={item.body}
      titleLines={6}
      subtitle={[
        showKind ? (kind ? t(`kind.${kind}`) : readable(item.kind)) : null,
        coach && coach !== '—' ? coach : null,
        coachingDate(item.created_at),
      ]
        .filter(Boolean)
        .join(' · ')}
      trailing={
        state === 'none' ? undefined : (
          <StatusPill
            label={
              state === 'completed'
                ? t('items.completed', { date: coachingDate(item.completed_at) })
                : state === 'overdue'
                  ? t('items.overdue', { date: coachingDate(item.due_at) })
                  : t('items.due', { date: coachingDate(item.due_at) })
            }
            tone={state === 'completed' ? 'done' : state === 'overdue' ? 'problem' : 'neutral'}
          />
        )
      }
      last={last}
    />
  );
}
