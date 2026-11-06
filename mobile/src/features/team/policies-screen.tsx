import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { EmptyState } from '@/ui/empty-state';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { type EscalationPolicy, useEscalationPolicies, useOncallScope } from './api';
import { NoAccess, useSetupLabels, useStepTargetLabel } from './oncall-setup-parts';

/** What a policy covers: incidents, urgent conversations, both or neither. */
function useCovers() {
  const { t } = useTranslation('team');
  return (p: EscalationPolicy) =>
    [p.incidents ? t('setup.covers_incidents') : null, p.urgent_conversations ? t('setup.covers_urgent') : null]
      .filter(Boolean)
      .join(', ') || t('setup.covers_none');
}

/**
 * Escalation policies, read-only (CF-168): the panel's "Eskalasyon" tab
 * (`frontend/src/features/oncall/components/policies-tab.tsx`). Policies are
 * created and edited on the panel.
 */
export function PoliciesScreen() {
  const { t } = useTranslation('team');
  const { canSetup } = useOncallScope();
  const policies = useEscalationPolicies();
  const refresh = useManualRefresh(policies);
  const covers = useCovers();
  if (!canSetup) return <NoAccess title={t('setup.policies')} body={t('setup.no_access')} />;
  return (
    <>
      <Stack.Screen options={{ title: t('setup.policies') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={policies}>
          {(d) =>
            d.items.length ? (
              <View className="gap-2">
                <List>
                  {d.items.map((p, i) => (
                    <ListRow
                      key={p.uuid}
                      icon="alert"
                      title={p.name}
                      subtitle={[t('setup.step_count', { count: p.steps.length }), covers(p)].join(' · ')}
                      trailing={p.enabled ? undefined : <StatusPill label={t('setup.disabled')} />}
                      onPress={() => router.push({ pathname: '/more/oncall-policy', params: { uuid: p.uuid } })}
                      last={i === d.items.length - 1}
                    />
                  ))}
                </List>
                <Text variant="footnote" tone="muted" className="px-4">
                  {t('setup.read_only')}
                </Text>
              </View>
            ) : (
              <EmptyState icon="alert" title={t('setup.policies_empty')} body={t('setup.read_only')} />
            )
          }
        </QueryState>
      </Screen>
    </>
  );
}

/** One policy: what it covers and its steps in order (delay, who, how). */
export function PolicyScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t } = useTranslation('team');
  const { canSetup } = useOncallScope();
  const policies = useEscalationPolicies();
  const refresh = useManualRefresh(policies);
  const covers = useCovers();
  const labels = useSetupLabels();
  const targetLabel = useStepTargetLabel();
  if (!canSetup) return <NoAccess title={t('setup.policy')} body={t('setup.no_access')} />;
  const policy = policies.data?.items.find((p) => p.uuid === uuid);
  return (
    <>
      <Stack.Screen options={{ title: policy?.name ?? t('setup.policy') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={policies}>
          {() =>
            policy ? (
              <View className="gap-4">
                <List>
                  <ListRow
                    icon="alert"
                    title={policy.name}
                    titleLines={2}
                    trailing={
                      <StatusPill
                        label={policy.enabled ? t('setup.enabled') : t('setup.disabled')}
                        tone={policy.enabled ? 'done' : 'neutral'}
                      />
                    }
                  />
                  <ListRow title={t('setup.covers')} subtitle={covers(policy)} last />
                </List>
                <View>
                  <SectionHeader title={`${t('setup.steps')} · ${policy.steps.length}`} />
                  <List>
                    {policy.steps.length ? (
                      policy.steps.map((step, i) => (
                        <ListRow
                          key={i}
                          title={targetLabel(step)}
                          titleLines={2}
                          subtitle={[labels.delay(step.after_minutes), step.channels.map(labels.channel).join(', ')]
                            .filter(Boolean)
                            .join(' · ')}
                          leading={
                            <Text tone="muted" className="w-6 text-center">
                              {i + 1}
                            </Text>
                          }
                          last={i === policy.steps.length - 1}
                        />
                      ))
                    ) : (
                      <ListRow title={t('setup.steps_empty')} last />
                    )}
                  </List>
                  <Text variant="footnote" tone="muted" className="px-4 pt-2">
                    {t('setup.steps_hint')}
                  </Text>
                </View>
              </View>
            ) : (
              <EmptyState icon="alert" title={t('setup.not_found')} />
            )
          }
        </QueryState>
      </Screen>
    </>
  );
}
