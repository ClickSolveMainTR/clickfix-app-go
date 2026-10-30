import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { formatNumber, formatRelative, formatShort } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { Card, Stat } from '@/ui/card';
import { Sparkline } from '@/ui/charts';
import { cn } from '@/ui/cn';
import { Chips } from '@/ui/chips';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import type { IconName } from '@/ui/icons';
import { SectionHeader } from '@/ui/list';
import { Input } from '@/ui/input';
import { QueryState } from '@/ui/query-state';
import { FormScreen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import {
  type CompanyOverview,
  type CompanyTimelineItem,
  useAddCompanyNote,
  useCompany,
  useCompanyHealth,
  useCompanyTimeline,
  useCustomFields,
  usePeopleScope,
} from './api';
import { CustomFieldsView } from './custom-fields';
import { healthTone } from './health';
import { ConversationHistory } from './record-conversations';

type Section = 'overview' | 'timeline';

const KIND_ICON: Record<CompanyTimelineItem['kind'], IconName> = {
  conversation_opened: 'inbox',
  conversation_resolved: 'check',
  csat: 'star',
  note: 'note',
  event: 'history',
  revenue: 'wallet',
  health: 'health',
};

/**
 * A company (CF-79, CF-85): overview with health score and trend, revenue,
 * CSAT and conversations; the account timeline with notes. Like HubSpot's
 * company record, sized for a phone.
 */
export function CompanyScreen() {
  const { t } = useTranslation('people');
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const overview = useCompany(uuid);
  const [section, setSection] = useState<Section>('overview');

  return (
    <>
      <Stack.Screen options={{ title: overview.data?.company.name ?? t('companies') }} />
      <FormScreen>
        <QueryState query={overview}>
          {(o) => (
            <View className="gap-4">
              <Header overview={o} />
              <Chips<Section>
                value={section}
                onChange={setSection}
                options={[
                  { value: 'overview', label: t('overview') },
                  { value: 'timeline', label: t('timeline') },
                ]}
              />
              {section === 'overview' ? <Overview overview={o} /> : <Timeline uuid={o.company.uuid} />}
            </View>
          )}
        </QueryState>
      </FormScreen>
    </>
  );
}

function Header({ overview: o }: { overview: CompanyOverview }) {
  const { t } = useTranslation('people');
  const c = o.company;
  const score = c.health?.score;
  const tone = healthTone(score, c.health?.below_threshold);
  return (
    <View className="flex-row items-center gap-3">
      <View className="size-14 items-center justify-center rounded-xl bg-secondary">
        <Icon name="building" size={26} tone="primary" />
      </View>
      <View className="flex-1 gap-0.5">
        <Text variant="title" numberOfLines={2} selectable>
          {c.name}
        </Text>
        <Text variant="footnote" tone="muted" numberOfLines={1}>
          {[c.domain, c.tier, c.account_owner?.name && t('owner', { name: c.account_owner.name })]
            .filter(Boolean)
            .join(' · ')}
        </Text>
      </View>
      {score != null ? (
        <View
          accessible
          accessibilityLabel={`${t('health_score', { score })}, ${t(`health.${tone.key}`)}`}
          className={cn('items-center rounded-xl px-3 py-2', tone.bg)}>
          <Text variant="headline" className="tabular-nums">
            {score}
          </Text>
          <Text variant="caption">{t(`health.${tone.key}`)}</Text>
        </View>
      ) : null}
    </View>
  );
}

function Overview({ overview: o }: { overview: CompanyOverview }) {
  const { t } = useTranslation('people');
  const c = o.company;
  const health = useCompanyHealth(c.uuid);
  const fields = useCustomFields('company');
  const s = o.stats;

  return (
    <View className="gap-4">
      <Card>
        <View className="flex-row gap-3">
          <Stat label={t('stats.open')} value={formatNumber(s.open_conversations)} />
          <Stat label={t('stats.closed')} value={formatNumber(s.closed_conversations)} />
          <Stat
            label={t('stats.csat', { count: s.csat_count })}
            value={s.csat_average != null ? formatNumber(s.csat_average, 1, 1) : '—'}
          />
        </View>
        {s.last_conversation_at ? (
          <Text variant="footnote" tone="muted">
            {t('stats.last', { at: formatRelative(s.last_conversation_at) })}
          </Text>
        ) : null}
      </Card>

      {health.data && health.data.score != null ? (
        <Card title={t('health_title')}>
          <Text variant="footnote" tone="muted">
            {t('health_threshold', { threshold: health.data.threshold })}
            {health.data.computed_at ? ` · ${formatShort(health.data.computed_at)}` : ''}
          </Text>
          <Sparkline
            values={[...health.data.history].reverse().map((h) => h.score)}
            summary={t('health_trend', {
              values: [...health.data.history]
                .reverse()
                .map((h) => h.score)
                .join(', '),
            })}
          />
        </Card>
      ) : null}

      {c.revenue ? (
        <Card title={t('revenue')}>
          <View className="flex-row gap-3">
            <Stat label="MRR" value={`${formatNumber(c.revenue.mrr_minor / 100, 2)} ${c.revenue.currency}`} />
            <Stat
              label={t('ltv')}
              value={`${formatNumber(c.revenue.lifetime_value_minor / 100)} ${c.revenue.currency}`}
            />
          </View>
          <View className="flex-row flex-wrap items-center gap-2">
            {c.revenue.plan ? <StatusPill label={c.revenue.plan} /> : null}
            <StatusPill
              label={t(`billing_status.${c.revenue.billing_status}`)}
              tone={
                c.revenue.billing_status === 'past_due'
                  ? 'problem'
                  : c.revenue.billing_status === 'active'
                    ? 'done'
                    : 'neutral'
              }
            />
            {c.revenue.renewal_at ? (
              <Text variant="footnote" tone="muted">
                {t('renews', { at: formatShort(c.revenue.renewal_at) })}
              </Text>
            ) : null}
          </View>
        </Card>
      ) : null}

      <View className="gap-2">
        <SectionHeader title={t('open_conversations')} />
        <ConversationHistory items={o.open_conversations} emptyText={t('no_open')} />
      </View>
      {o.recently_closed.length ? (
        <View className="gap-2">
          <SectionHeader title={t('recently_closed')} />
          <ConversationHistory items={o.recently_closed} emptyText="" />
        </View>
      ) : null}

      {Object.keys(c.custom_fields ?? {}).length ? (
        <Card title={t('custom_fields')}>
          <CustomFieldsView
            defs={fields.data?.items ?? []}
            values={(c.custom_fields ?? {}) as Record<string, unknown>}
          />
        </Card>
      ) : null}
      {c.notes ? (
        <Card title={t('fields.notes')}>
          <Text selectable>{c.notes}</Text>
        </Card>
      ) : null}
    </View>
  );
}

function Timeline({ uuid }: { uuid: string }) {
  const { t } = useTranslation('people');
  const timeline = useCompanyTimeline(uuid);
  const { canWrite } = usePeopleScope();
  const addNote = useAddCompanyNote(uuid);
  const [note, setNote] = useState('');
  const items = timeline.data?.pages.flatMap((p) => p.items) ?? [];

  return (
    <View className="gap-3">
      {canWrite ? (
        <Card>
          <Input
            label={t('add_note')}
            value={note}
            onChangeText={setNote}
            multiline
            placeholder={t('note_placeholder')}
          />
          <Button
            title={t('save_note')}
            disabled={!note.trim()}
            loading={addNote.isPending}
            onPress={() =>
              addNote.mutate(note.trim(), {
                onSuccess: () => {
                  setNote('');
                  toast.success(t('note_saved'));
                },
                onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
              })
            }
          />
        </Card>
      ) : null}
      <QueryState query={timeline}>
        {() =>
          items.length ? (
            <View className="gap-0">
              {items.map((it, i) => (
                <View key={`${it.kind}-${it.ref ?? i}-${it.occurred_at}`} className="flex-row gap-3">
                  <View className="items-center">
                    <View className="size-8 items-center justify-center rounded-full bg-secondary">
                      <Icon name={KIND_ICON[it.kind] ?? 'history'} size={15} tone="primary" />
                    </View>
                    {i < items.length - 1 ? <View className="w-px flex-1 bg-border" /> : null}
                  </View>
                  <View className="flex-1 gap-0.5 pb-4">
                    <Text variant="callout" className="font-sans-semibold">
                      {it.title}
                    </Text>
                    {it.body ? (
                      <Text variant="footnote" selectable>
                        {it.body}
                      </Text>
                    ) : null}
                    <Text variant="footnote" tone="muted">
                      {[it.actor, formatShort(it.occurred_at)].filter(Boolean).join(' · ')}
                    </Text>
                  </View>
                </View>
              ))}
              {timeline.hasNextPage ? (
                <Button
                  title={t('load_more')}
                  variant="ghost"
                  loading={timeline.isFetchingNextPage}
                  onPress={() => void timeline.fetchNextPage()}
                />
              ) : null}
            </View>
          ) : timeline.isFetching ? (
            <ActivityIndicator />
          ) : (
            <EmptyState icon="history" title={t('timeline_empty')} />
          )
        }
      </QueryState>
    </View>
  );
}
