import { router } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { openAssistant } from '@/features/assistant/open';
import { openFeedback, setShakeEnabled, useFeedback } from '@/features/feedback/store';
import { SwitchRow } from '@/ui/fields';
import { GuideTarget } from '@/ui/guide-target';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { toast } from '@/ui/toast';

import { useHelpAvailability, useResetGuides, useGuidePreferences, useSetGuidePreferences } from './api';

/**
 * Settings → Help (CF-142): replay the guides, the setup tour, ClickFix
 * Asistan, send feedback, and whether a shake opens the feedback sheet.
 */
export function HelpSettingsCard() {
  const { t } = useTranslation('help');
  const help = useHelpAvailability();
  const shake = useFeedback((s) => s.shake);
  const reset = useResetGuides();
  const preferences = useGuidePreferences();
  const setPreferences = useSetGuidePreferences();
  if (!help.guides && !help.assistant && !help.feedback) return null;

  const rows = [
    help.guides && {
      key: 'setup',
      icon: 'checklist' as const,
      title: t('settings.setup'),
      onPress: () => router.push('/setup-guide'),
    },
    help.guides && {
      key: 'replay',
      icon: 'refresh' as const,
      title: t('settings.replay'),
      subtitle: t('settings.replay_hint'),
      onPress: () =>
        reset.mutate(undefined, {
          onSuccess: () => {
            toast.success(t('settings.replay_done'));
          },
          onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
        }),
    },
    help.assistant && {
      key: 'assistant',
      icon: 'sparkles' as const,
      title: t('more.assistant'),
      onPress: () => openAssistant({ route: '/more/settings' }),
    },
    help.feedback && {
      key: 'feedback',
      icon: 'feedback' as const,
      title: t('more.feedback'),
      onPress: () => void openFeedback({ route: '/more/settings', capture: false }),
    },
  ].filter((r) => !!r);

  return (
    <GuideTarget id="settings.help">
      <SectionHeader title={t('settings.title')} />
      {help.guides ? (
        <SwitchRow
          label={t('settings.auto_guides')}
          value={preferences.data?.auto_start === true}
          onChange={(on) => setPreferences.mutate(on)}
        />
      ) : null}
      <List>
        {rows.map((r, i) => (
          <ListRow
            key={r.key}
            icon={r.icon}
            title={r.title}
            subtitle={'subtitle' in r ? r.subtitle : undefined}
            onPress={r.onPress}
            last={i === rows.length - 1}
          />
        ))}
      </List>
      {help.feedback ? (
        <View className="pt-3">
          <SwitchRow
            label={t('settings.shake')}
            description={t('settings.shake_hint')}
            value={shake}
            onChange={setShakeEnabled}
          />
        </View>
      ) : null}
    </GuideTarget>
  );
}
