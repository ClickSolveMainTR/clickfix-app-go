import { Button, HStack, Host, Image, Popover, Spacer, Text, VStack } from '@expo/ui/swift-ui';
import {
  accessibilityLabel,
  buttonStyle,
  contentShape,
  font,
  foregroundStyle,
  frame,
  padding,
  shapes,
} from '@expo/ui/swift-ui/modifiers';
import * as Haptics from 'expo-haptics';
import { useCSSVariable } from 'uniwind';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { usePreferences } from '@/core/preferences';
import { toast } from '@/ui/toast';

import { PRESENCE_STATUSES, type PresenceStatus, useMyPresence, useSetPresence } from './presence';

/**
 * The agent's status from the inbox header (CF-82): a dot in the status
 * colour; a tap opens a popover with available / break / busy / offline (CF-54 API).
 */
export function PresenceButton() {
  const { t } = useTranslation('settings');
  const [success, warning, destructive, muted, primary] = useCSSVariable(['--color-success', '--color-warning', '--color-destructive', '--color-muted-foreground', '--color-primary']);
  const tint: Record<PresenceStatus, string> = { online: String(success), away: String(warning), busy: String(destructive), offline: String(muted) };
  const theme = usePreferences((s) => s.theme);
  const me = useMyPresence();
  const set = useSetPresence();
  const [open, setOpen] = useState(false);
  if (!me.data) return null;
  const current = me.data.manual_status;

  const pick = (s: PresenceStatus) => {
    setOpen(false);
    if (s === current) return;
    void Haptics.selectionAsync();
    set.mutate(s, { onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)) });
  };

  return (
    <Host matchContents colorScheme={theme === 'system' ? undefined : theme}>
      <Popover isPresented={open} onIsPresentedChange={setOpen} arrowEdge="top">
        <Popover.Trigger>
          <Button
            onPress={() => setOpen(true)}
            modifiers={[
              buttonStyle('plain'),
              frame({ width: 36, height: 36 }),
              contentShape(shapes.circle()),
              accessibilityLabel(`${t('presence')}: ${t(`presence_${current}`)}`),
            ]}>
            <Image systemName="circle.fill" modifiers={[font({ size: 14 }), foregroundStyle(tint[current])]} />
          </Button>
        </Popover.Trigger>
        <Popover.Content>
          <VStack alignment="leading" spacing={0} modifiers={[padding({ vertical: 6 }), frame({ width: 280 })]}>
            {PRESENCE_STATUSES.map((s) => (
              <Button
                key={s}
                onPress={() => pick(s)}
                modifiers={[buttonStyle('plain'), accessibilityLabel(t(`presence_${s}`))]}>
                <HStack
                  spacing={12}
                  modifiers={[padding({ horizontal: 16, vertical: 11 }), contentShape(shapes.rectangle())]}>
                  <Image systemName="circle.fill" modifiers={[font({ size: 12 }), foregroundStyle(tint[s])]} />
                  <VStack alignment="leading" spacing={2}>
                    <Text modifiers={[font({ textStyle: 'body' })]}>{t(`presence_${s}`)}</Text>
                    {s === 'online' ? (
                      <Text
                        modifiers={[
                          font({ textStyle: 'footnote' }),
                          foregroundStyle({ type: 'hierarchical', style: 'secondary' }),
                        ]}>
                        {t('presence_hint')}
                      </Text>
                    ) : null}
                  </VStack>
                  <Spacer />
                  {s === current ? (
                    <Image
                      systemName="checkmark"
                      modifiers={[font({ size: 15, weight: 'semibold' }), foregroundStyle(String(primary))]}
                    />
                  ) : null}
                </HStack>
              </Button>
            ))}
          </VStack>
        </Popover.Content>
      </Popover>
    </Host>
  );
}
