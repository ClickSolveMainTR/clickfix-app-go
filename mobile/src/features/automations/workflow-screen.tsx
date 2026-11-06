import { Stack, useLocalSearchParams } from 'expo-router';
import { RefreshControl, View } from 'react-native';

import { formatShort } from '@/core/format';
import { EmptyState } from '@/ui/empty-state';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { useAutomationScope, useTargetNames, useToggleWorkflow, useWorkflowLastRun, useWorkflows } from './api';
import {
  describeNode,
  hasUnpublishedChanges,
  orderedNodes,
  runTone,
  triggerLabel,
  triggerNode,
  workflowState,
} from './core';
import { EnabledSwitch, SummaryLines, toggleFailed, useAutomationT } from './parts';

const STATE_TONE = { draft: 'neutral', enabled: 'done', disabled: 'neutral' } as const;

/**
 * Read-only summary of one workflow (CF-164): state and switch, trigger,
 * version, the latest run and every step in reading order. It shows the
 * draft, the same graph the panel's editor opens.
 */
export function WorkflowScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const t = useAutomationT();
  const { canWrite } = useAutomationScope();
  const list = useWorkflows();
  const workflow = list.data?.items.find((w) => w.uuid === uuid);
  const lastRun = useWorkflowLastRun(uuid, !!workflow && workflow.published_version !== null);
  const names = useTargetNames();
  const toggle = useToggleWorkflow();
  const refresh = useManualRefresh({
    isRefetching: list.isRefetching || lastRun.isRefetching,
    refetch: () => Promise.all([list.refetch(), lastRun.refetch()]),
  });

  return (
    <>
      <Stack.Screen options={{ title: workflow?.name ?? t('automations.workflows') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={list}>
          {() => {
            if (!workflow) return <EmptyState icon="split" title={t('automations.workflow_missing')} />;
            const state = workflowState(workflow);
            const trigger = triggerNode(workflow);
            const run = lastRun.data;
            const steps = orderedNodes(workflow.draft);
            return (
              <View className="gap-4">
                <View>
                  <List>
                    <ListRow
                      title={workflow.name}
                      titleLines={3}
                      trailing={<StatusPill label={t(`automations.state_${state}`)} tone={STATE_TONE[state]} />}
                    />
                    <ListRow
                      title={t('automations.enabled')}
                      trailing={
                        <EnabledSwitch
                          label={t('automations.enabled')}
                          value={workflow.enabled}
                          disabled={!canWrite || state === 'draft' || toggle.isPending}
                          onChange={(enabled) =>
                            toggle.mutate(
                              { uuid: workflow.uuid, enabled },
                              {
                                onSuccess: (row) =>
                                  toast.success(
                                    t(row.enabled ? 'automations.workflow_enabled' : 'automations.workflow_disabled'),
                                  ),
                                onError: (e) => toggleFailed(t, e),
                              },
                            )
                          }
                        />
                      }
                      last
                    />
                  </List>
                  <Text variant="footnote" tone="muted" className="px-4 pt-2">
                    {!canWrite
                      ? t('automations.read_only')
                      : state === 'draft'
                        ? t('automations.draft_hint')
                        : t('automations.edit_on_panel')}
                  </Text>
                </View>

                <View>
                  <SectionHeader title={t('automations.summary')} />
                  <List>
                    <ListRow
                      title={t('automations.trigger')}
                      subtitle={triggerLabel(t, trigger?.trigger, trigger?.schedule_minutes)}
                    />
                    <ListRow
                      title={t('automations.version')}
                      subtitle={[
                        workflow.published_version === null
                          ? t('automations.not_published')
                          : t('automations.version_value', { version: workflow.published_version }),
                        t('automations.revision_value', { revision: workflow.revision }),
                        hasUnpublishedChanges(workflow) ? t('automations.unpublished') : '',
                      ]
                        .filter(Boolean)
                        .join(' · ')}
                    />
                    <ListRow
                      title={t('automations.last_run')}
                      subtitle={
                        run
                          ? [formatShort(run.created_at), run.error].filter(Boolean).join(' · ')
                          : lastRun.isLoading
                            ? '…'
                            : lastRun.isError
                              ? t('automations.last_run_unknown')
                              : t('automations.never_ran')
                      }
                      trailing={
                        run ? (
                          <StatusPill label={t(`automations.run_status.${run.status}`)} tone={runTone(run.status)} />
                        ) : undefined
                      }
                    />
                    <ListRow title={t('automations.updated')} subtitle={formatShort(workflow.updated_at)} last />
                  </List>
                </View>

                <View>
                  <SectionHeader title={t('automations.steps', { count: steps.length })} />
                  <List>
                    {steps.map((node, i) => {
                      const step = describeNode(t, node, names);
                      return (
                        <ListRow
                          key={node.id}
                          title={`${i + 1}. ${step.title}`}
                          subtitle={<SummaryLines lines={step.lines} />}
                          last={i === steps.length - 1}
                        />
                      );
                    })}
                  </List>
                  {hasUnpublishedChanges(workflow) ? (
                    <Text variant="footnote" tone="muted" className="px-4 pt-2">
                      {t('automations.steps_draft_hint')}
                    </Text>
                  ) : null}
                </View>
              </View>
            );
          }}
        </QueryState>
      </Screen>
    </>
  );
}
