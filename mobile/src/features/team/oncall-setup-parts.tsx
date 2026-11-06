import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';

import { useTeams } from '@/features/inbox/api';
import { EmptyState } from '@/ui/empty-state';
import { Screen } from '@/ui/screen';

import { type EscalationStep, type UptimeMonitor, useRotas } from './api';
import { useMemberNames } from './names';
import { channelKey, intervalParts, monitorState, readable, stepTarget } from './oncall-core';

/** Shown instead of a setup screen the member may not read (also when opened by a link). */
export function NoAccess({ title, body }: { title: string; body: string }) {
  return (
    <>
      <Stack.Screen options={{ title }} />
      <Screen>
        <EmptyState icon="alert" title={body} />
      </Screen>
    </>
  );
}

/** "Rota: Hafta içi", "Ekip: Destek", "Temsilci: Ada Yılmaz"; a deleted or unreadable target shows "—". */
export function useStepTargetLabel(): (step: EscalationStep) => string {
  const { t } = useTranslation('team');
  const rotas = useRotas();
  const teams = useTeams(true);
  const memberName = useMemberNames();
  return (step) => {
    const target = stepTarget(step);
    if (!target) return '—';
    const name =
      target.kind === 'rota'
        ? rotas.data?.items.find((r) => r.uuid === target.uuid)?.name
        : target.kind === 'team'
          ? teams.data?.items.find((x) => x.uuid === target.uuid)?.name
          : memberName(target.uuid);
    return `${t(`setup.target_${target.kind}`)}: ${name ?? '—'}`;
  };
}

/** Labels of on-call values; one the app does not know is shown as readable text, never as a key. */
export function useSetupLabels() {
  const { t } = useTranslation('team');
  return {
    channel: (value: string) => {
      const key = channelKey(value);
      return key ? t(`setup.channel_${key}`) : readable(value);
    },
    delay: (minutes: number) => (minutes > 0 ? t('setup.after', { minutes }) : t('setup.immediately')),
    interval: (seconds: number) => {
      const { unit, count } = intervalParts(seconds);
      return t(`setup.interval_${unit}`, { count });
    },
    monitor: (m: UptimeMonitor) => {
      const state = monitorState(m);
      return { tone: state.tone, label: t(`setup.monitor_${state.key}`, { code: state.code ?? '' }) };
    },
  };
}
