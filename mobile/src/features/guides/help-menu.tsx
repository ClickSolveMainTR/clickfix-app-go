import { type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { openAssistant } from '@/features/assistant/open';
import { openFeedback } from '@/features/feedback/store';
import { HeaderButton } from '@/ui/header-button';
import { ActionMenu } from '@/ui/menu';
import { toast } from '@/ui/toast';

import { useHelpAvailability, useScreenGuides } from './api';
import { screenGuide } from './core';
import { useRoutePattern } from './host';
import { startGuide } from './store';

type HelpAction = 'guide' | 'assistant' | 'feedback';

/** The sheet closes before the screenshot, so it never shows in the picture. */
const SHEET_CLOSE_MS = 350;

/**
 * "?" in every screen's header (CF-142): what this screen is for (its
 * guide), ask ClickFix Asistan with this screen as context, and send
 * feedback with a screenshot of it. Hidden when the server has none of them.
 */
export function HelpMenuButton() {
  const { t } = useTranslation('help');
  const route = useRoutePattern();
  const available = useHelpAvailability();
  const guides = useScreenGuides(route);
  const guide = guides.data ? screenGuide(guides.data) : null;

  const actions: HelpAction[] = [
    ...(available.guides ? (['guide'] as const) : []),
    ...(available.assistant ? (['assistant'] as const) : []),
    ...(available.feedback ? (['feedback'] as const) : []),
  ];
  if (!actions.length) return null;

  const pick = (a: HelpAction) => {
    if (a === 'guide') {
      if (guide) setTimeout(() => startGuide(guide, 'manual'), SHEET_CLOSE_MS);
      else toast.error(t('guides.none_here'));
    } else if (a === 'assistant') openAssistant({ route });
    else setTimeout(() => void openFeedback({ route, capture: true }), SHEET_CLOSE_MS);
  };

  return (
    <ActionMenu<HelpAction>
      title={t('menu.title')}
      trigger={<HeaderButton icon="help" guide="assistant-button" label={t('menu.title')} />}
      options={actions.map((a) => ({
        value: a,
        label: t(`menu.${a}`),
        systemImage:
          a === 'guide'
            ? 'questionmark.circle'
            : a === 'assistant'
              ? 'sparkles'
              : 'bubble.left.and.exclamationmark.bubble.right',
        description: a === 'guide' && guide ? guide.title : t(`menu.${a}_hint`),
      }))}
      onPick={pick}
    />
  );
}

/** A screen's own header actions followed by the help menu. */
export function HeaderActions({ children }: { children?: ReactNode }) {
  return (
    <View className="flex-row items-center gap-1">
      {children}
      <HelpMenuButton />
    </View>
  );
}
