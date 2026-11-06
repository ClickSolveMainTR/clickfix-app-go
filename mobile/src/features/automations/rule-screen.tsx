import { Stack, useLocalSearchParams } from 'expo-router';
import { RefreshControl, View } from 'react-native';

import { formatShort } from '@/core/format';
import { EmptyState } from '@/ui/empty-state';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { useAutomationScope, useRuleExecutions, useRules, useTargetNames } from './api';
import {
  conditionLines,
  countConditions,
  describeAction,
  emptyCondition,
  executionStatusLabel,
  lastExecution,
  type RuleExecution,
  runTone,
  type T,
  triggerLabel,
} from './core';
import { EnabledSwitch, SummaryLines, useAutomationT } from './parts';
import { useRuleSwitch } from './rules-screen';

/** The run log's wording for one evaluation (`rules-page.tsx`, `RunLog`). */
function executionDetail(t: T, e: RuleExecution) {
  if (e.status === 'skipped') return t('automations.not_matched');
  if (e.status === 'failed') return t('automations.failed_at', { step: e.action_index + 1, error: e.error || '—' });
  if (e.status === 'succeeded') return t('automations.actions_ran', { count: e.action_index });
  return '';
}

/**
 * Read-only summary of one automation rule (CF-164): switch, trigger, order,
 * the latest evaluation, the conditions and the actions in the order they run.
 */
export function RuleScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const t = useAutomationT();
  const { canRead, canWrite } = useAutomationScope();
  const list = useRules();
  const log = useRuleExecutions();
  const names = useTargetNames();
  const sw = useRuleSwitch();
  const rule = list.data?.items.find((r) => r.uuid === uuid);
  const refresh = useManualRefresh({
    isRefetching: list.isRefetching || log.isRefetching,
    refetch: () => Promise.all([list.refetch(), log.refetch()]),
  });

  if (!canRead)
    return (
      <>
        <Stack.Screen options={{ title: t('automations.rules') }} />
        <Screen>
          <EmptyState icon="macro" title={t('automations.rules_no_permission')} />
        </Screen>
      </>
    );

  return (
    <>
      <Stack.Screen options={{ title: rule?.name ?? t('automations.rules') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={list}>
          {() => {
            if (!rule) return <EmptyState icon="macro" title={t('automations.rule_missing')} />;
            const last = lastExecution(log.data, rule.id);
            const lines = conditionLines(t, rule.conditions, names);
            const empty = emptyCondition(rule.conditions);
            return (
              <View className="gap-4">
                <View>
                  <List>
                    <ListRow title={rule.name} titleLines={3} />
                    <ListRow
                      title={t('automations.enabled')}
                      trailing={
                        <EnabledSwitch
                          label={t('automations.enabled')}
                          value={rule.enabled}
                          disabled={sw.disabled}
                          onChange={(enabled) => sw.change(rule, enabled)}
                        />
                      }
                      last
                    />
                  </List>
                  <Text variant="footnote" tone="muted" className="px-4 pt-2">
                    {canWrite ? t('automations.edit_on_panel') : t('automations.read_only')}
                  </Text>
                </View>

                <View>
                  <SectionHeader title={t('automations.summary')} />
                  <List>
                    <ListRow
                      title={t('automations.trigger')}
                      subtitle={triggerLabel(t, rule.trigger_event, rule.schedule_minutes)}
                    />
                    <ListRow
                      title={t('automations.order')}
                      subtitle={[String(rule.position), rule.stop_on_match ? t('automations.stops') : '']
                        .filter(Boolean)
                        .join(' · ')}
                    />
                    <ListRow
                      title={t('automations.last_run')}
                      subtitle={
                        last
                          ? [formatShort(last.created_at), executionDetail(t, last)].filter(Boolean).join(' · ')
                          : log.isLoading
                            ? '…'
                            : log.isError
                              ? t('automations.last_run_unknown')
                              : t('automations.no_recent_run')
                      }
                      trailing={
                        last ? (
                          <StatusPill label={executionStatusLabel(t, last.status)} tone={runTone(last.status)} />
                        ) : undefined
                      }
                    />
                    <ListRow title={t('automations.updated')} subtitle={formatShort(rule.updated_at)} last />
                  </List>
                </View>

                <View>
                  <SectionHeader title={t('automations.conditions')} />
                  <List>
                    <ListRow
                      title={
                        empty
                          ? t(`automations.empty_${empty}`)
                          : t('automations.conditions_count', { count: countConditions(rule.conditions) })
                      }
                      titleLines={3}
                      subtitle={empty ? undefined : <SummaryLines lines={lines} />}
                      last
                    />
                  </List>
                </View>

                <View>
                  <SectionHeader title={t('automations.actions')} />
                  <List>
                    {rule.actions.map((action, i) => {
                      const a = describeAction(t, action, names);
                      return (
                        <ListRow
                          key={i}
                          title={`${i + 1}. ${a.title}`}
                          titleLines={2}
                          subtitle={a.detail || undefined}
                          last={i === rule.actions.length - 1}
                        />
                      );
                    })}
                  </List>
                  <Text variant="footnote" tone="muted" className="px-4 pt-2">
                    {t('automations.actions_hint')}
                  </Text>
                </View>
              </View>
            );
          }}
        </QueryState>
      </Screen>
    </>
  );
}
