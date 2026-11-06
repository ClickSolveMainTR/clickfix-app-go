import { router, Stack } from 'expo-router';
import { RefreshControl } from 'react-native';

import { EmptyState } from '@/ui/empty-state';
import { List, ListRow } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { useAutomationScope, useToggleWorkflow, useWorkflows } from './api';
import { hasUnpublishedChanges, triggerLabel, triggerNode, workflowState } from './core';
import { EnabledSwitch, toggleFailed, useAutomationT } from './parts';

/**
 * Workflows (CF-164), the phone version of the panel's list
 * (`frontend/src/features/workflows/components/workflows-page.tsx`): name,
 * trigger, publish state and the on/off switch. A row opens the read-only
 * summary; the canvas editor stays on the panel.
 */
export function WorkflowsScreen() {
  const t = useAutomationT();
  const { canRead, canWrite } = useAutomationScope();
  const list = useWorkflows();
  const refresh = useManualRefresh(list);
  const toggle = useToggleWorkflow();

  if (!canRead)
    return (
      <>
        <Stack.Screen options={{ title: t('automations.workflows') }} />
        <Screen>
          <EmptyState icon="split" title={t('automations.workflows_no_permission')} />
        </Screen>
      </>
    );

  return (
    <>
      <Stack.Screen options={{ title: t('automations.workflows') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={list}>
          {(data) =>
            data.items.length ? (
              <>
                <List>
                  {data.items.map((w, i) => {
                    const state = workflowState(w);
                    const trigger = triggerNode(w);
                    return (
                      <ListRow
                        key={w.uuid}
                        icon="split"
                        title={w.name}
                        subtitle={[
                          triggerLabel(t, trigger?.trigger, trigger?.schedule_minutes),
                          t(`automations.state_${state}`),
                          hasUnpublishedChanges(w) ? t('automations.unpublished') : '',
                        ]
                          .filter(Boolean)
                          .join(' · ')}
                        trailing={
                          <EnabledSwitch
                            label={t('automations.enabled_label', { name: w.name })}
                            value={w.enabled}
                            disabled={!canWrite || state === 'draft' || toggle.isPending}
                            onChange={(enabled) =>
                              toggle.mutate(
                                { uuid: w.uuid, enabled },
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
                        onPress={() => router.push({ pathname: '/more/workflow', params: { uuid: w.uuid } })}
                        last={i === data.items.length - 1}
                      />
                    );
                  })}
                </List>
                <Text variant="footnote" tone="muted" className="px-4">
                  {canWrite ? t('automations.workflows_hint') : t('automations.read_only')}
                </Text>
              </>
            ) : (
              <EmptyState icon="split" title={t('automations.workflows_empty')} body={t('automations.edit_on_panel')} />
            )
          }
        </QueryState>
      </Screen>
    </>
  );
}
