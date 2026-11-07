import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { P, useCan } from '@/core/auth/permissions';
import { formatNumber } from '@/core/format';
import { useMemberNames } from '@/features/team/names';
import { Card, Stat } from '@/ui/card';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { useCalibration, useCoachingScope } from './api';
import { NoAccess } from './coaching-screen';
import { criteriaRows, SPREAD_FLAG } from './core';
import { coachingDate } from './dates';

const nf = (value: number) => formatNumber(value, 1);

/**
 * One calibration session, read-only (CF-170): the panel's session dialog
 * (`frontend/src/features/coaching/components/calibration-dialog.tsx`)
 * without scoring or closing. An open session shows its rubric; a closed
 * one adds mean, variance and range per criterion and every reviewer's
 * total. Needs `tenant.quality.manage`.
 */
export function CalibrationScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t } = useTranslation('coaching');
  const can = useCan();
  const { canQuality } = useCoachingScope();
  const query = useCalibration(uuid);
  const refresh = useManualRefresh(query);
  const nameOf = useMemberNames();
  if (!canQuality) return <NoAccess title={t('calibrations.session')} body={t('forbidden')} />;
  // Opened without a session (a broken link): nothing to load, so no request and no endless skeleton.
  if (!uuid) return <NoAccess title={t('calibrations.session')} body={t('work:coaching.not_found')} />;
  return (
    <>
      <Stack.Screen options={{ title: query.data?.name ?? t('calibrations.session') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={query}>
          {(s) => {
            const closed = !!s.closed_at;
            const rows = criteriaRows(s.rubric ?? [], s.criteria);
            const reviews = s.reviews ?? [];
            return (
              <View className="gap-4">
                <List>
                  <ListRow
                    title={s.name}
                    titleLines={2}
                    subtitle={coachingDate(s.created_at)}
                    trailing={
                      <StatusPill
                        label={closed ? t('calibrations.status_closed') : t('calibrations.status_open')}
                        tone={closed ? 'neutral' : 'progress'}
                      />
                    }
                    last={!can(P.conversationsRead)}
                  />
                  {can(P.conversationsRead) ? (
                    <ListRow
                      icon="chat"
                      title={t('calibrations.open_conversation')}
                      onPress={() => router.push(`/conversation/${s.conversation_uuid}`)}
                      last
                    />
                  ) : null}
                </List>
                <Text variant="footnote" tone="muted" className="px-4">
                  {closed ? t('calibrations.closed_hint') : t('work:coaching.calibration_open')}
                </Text>
                {closed && s.summary ? (
                  <Card>
                    <View className="flex-row gap-3">
                      <Stat label={t('calibrations.reviewers')} value={formatNumber(s.summary.count)} />
                      <Stat label={t('calibrations.mean_score')} value={nf(s.summary.mean)} />
                    </View>
                    <View className="flex-row gap-3">
                      <Stat label={t('calibrations.variance')} value={nf(s.summary.variance)} />
                      <Stat label={t('calibrations.range')} value={nf(s.summary.range)} />
                    </View>
                  </Card>
                ) : null}
                <View>
                  <SectionHeader title={closed ? t('calibrations.by_criterion') : t('calibrations.criterion')} />
                  <List>
                    {rows.map((c, i) => (
                      <ListRow
                        key={c.key}
                        title={c.label}
                        titleLines={2}
                        subtitle={[
                          t('calibrations.weight', { weight: formatNumber(c.weight, 2) }),
                          c.critical ? t('calibrations.critical') : null,
                          c.stats
                            ? `${t('calibrations.mean')} ${nf(c.stats.mean)} · ${t('calibrations.variance')} ${nf(c.stats.variance)}`
                            : null,
                        ]
                          .filter(Boolean)
                          .join(' · ')}
                        trailing={
                          c.stats ? (
                            <StatusPill label={`Δ ${nf(c.stats.range)}`} tone={c.spread ? 'problem' : 'neutral'} />
                          ) : undefined
                        }
                        last={i === rows.length - 1}
                      />
                    ))}
                    {!rows.length ? <ListRow title="—" last /> : null}
                  </List>
                </View>
                {closed && reviews.length ? (
                  <View>
                    <SectionHeader title={t('calibrations.reviewer')} />
                    <List>
                      {reviews.map((r, i) => (
                        <ListRow
                          key={r.reviewer_uuid}
                          title={nameOf(r.reviewer_uuid)}
                          subtitle={r.critical_failure ? t('calibrations.critical_failure') : undefined}
                          trailing={<Text className="tabular-nums">{nf(r.score)}</Text>}
                          last={i === reviews.length - 1}
                        />
                      ))}
                    </List>
                    <Text variant="footnote" tone="muted" className="px-4 pt-2">
                      {t('calibrations.results_hint', { spread: SPREAD_FLAG })}
                    </Text>
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
