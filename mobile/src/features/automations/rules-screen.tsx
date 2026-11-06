import { router, Stack } from 'expo-router';
import { RefreshControl } from 'react-native';

import { EmptyState } from '@/ui/empty-state';
import { List, ListRow } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { useAutomationScope, useRules, useToggleRule } from './api';
import { countConditions, type Rule, triggerLabel } from './core';
import { EnabledSwitch, toggleFailed, useAutomationT } from './parts';

/** The switch of a rule row; shared by the list and the summary. */
export function useRuleSwitch() {
  const t = useAutomationT();
  const { canWrite } = useAutomationScope();
  const toggle = useToggleRule();
  return {
    disabled: !canWrite || toggle.isPending,
    change: (rule: Rule, enabled: boolean) =>
      toggle.mutate(
        { uuid: rule.uuid, enabled },
        {
          onSuccess: () => toast.success(t(enabled ? 'automations.rule_enabled' : 'automations.rule_disabled')),
          onError: (e) => toggleFailed(t, e),
        },
      ),
  };
}

/**
 * Automation rules (CF-164), the phone version of the panel's list
 * (`frontend/src/features/automation-rules/components/rules-page.tsx`): the
 * rules in evaluation order with trigger, logic and the on/off switch. A row
 * opens the read-only summary; the editor stays on the panel.
 */
export function RulesScreen() {
  const t = useAutomationT();
  const { canRead, canWrite } = useAutomationScope();
  const list = useRules();
  const refresh = useManualRefresh(list);
  const sw = useRuleSwitch();

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
      <Stack.Screen options={{ title: t('automations.rules') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={list}>
          {(data) =>
            data.items.length ? (
              <>
                <List>
                  {data.items.map((r, i) => (
                    <ListRow
                      key={r.uuid}
                      icon="macro"
                      title={r.name}
                      subtitle={[
                        triggerLabel(t, r.trigger_event, r.schedule_minutes),
                        t('automations.conditions_count', { count: countConditions(r.conditions) }),
                        t('automations.actions_count', { count: r.actions.length }),
                        r.stop_on_match ? t('automations.stops') : '',
                      ]
                        .filter(Boolean)
                        .join(' · ')}
                      trailing={
                        <EnabledSwitch
                          label={t('automations.enabled_label', { name: r.name })}
                          value={r.enabled}
                          disabled={sw.disabled}
                          onChange={(enabled) => sw.change(r, enabled)}
                        />
                      }
                      onPress={() => router.push({ pathname: '/more/rule', params: { uuid: r.uuid } })}
                      last={i === data.items.length - 1}
                    />
                  ))}
                </List>
                <Text variant="footnote" tone="muted" className="px-4">
                  {[t('automations.rules_order'), canWrite ? '' : t('automations.read_only')].filter(Boolean).join(' ')}
                </Text>
              </>
            ) : (
              <EmptyState icon="macro" title={t('automations.rules_empty')} body={t('automations.edit_on_panel')} />
            )
          }
        </QueryState>
      </Screen>
    </>
  );
}
