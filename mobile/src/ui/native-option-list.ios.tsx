import { usePreferences } from '@/core/preferences';
import {
  BottomSheet,
  Button,
  Group,
  HStack,
  Host,
  Image,
  List,
  NavigationStack,
  ProgressView,
  Section,
  Spacer,
  Text,
  Toolbar,
  ToolbarItem,
  VStack,
  useNativeState,
} from '@expo/ui/swift-ui';
import {
  accessibilityLabel,
  buttonStyle,
  font,
  foregroundStyle,
  navigationBarTitleDisplayMode,
  navigationTitle,
  presentationDetents,
  presentationDragIndicator,
  searchable,
} from '@expo/ui/swift-ui/modifiers';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import type { NativeOptionListProps } from './native-option-list';
export function NativeOptionList({
  visible,
  title,
  options,
  value,
  query,
  searchable: hasSearch,
  loading,
  footer,
  onSearch,
  onClose,
  onPick,
}: NativeOptionListProps) {
  const { t } = useTranslation();
  const theme = usePreferences((s) => s.theme);
  const search = useNativeState(query);
  useEffect(() => {
    search.set(query);
  }, [query, search]);
  return (
    <Host
      style={{ position: 'absolute', width: 0, height: 0 }}
      useViewportSizeMeasurement
      colorScheme={theme === 'system' ? undefined : theme}>
      <BottomSheet
        isPresented={visible}
        onIsPresentedChange={(v) => {
          if (!v) onClose();
        }}>
        <Group modifiers={[presentationDetents(['medium', 'large']), presentationDragIndicator('visible')]}>
          <NavigationStack>
            <Toolbar modifiers={[navigationTitle(title), navigationBarTitleDisplayMode('inline')]}>
              <List modifiers={hasSearch ? [searchable(search, { prompt: t('search'), onChange: onSearch })] : []}>
                <Section>
                  {options.map((option) => (
                    <Button
                      key={option.value}
                      onPress={() => onPick(option.value)}
                      modifiers={[
                        buttonStyle('plain'),
                        accessibilityLabel([option.label, option.description].filter(Boolean).join(', ')),
                      ]}>
                      <HStack>
                        <VStack alignment="leading">
                          <Text>{option.label}</Text>
                          {option.description ? (
                            <Text modifiers={[font({ size: 13 }), foregroundStyle('secondary')]}>
                              {option.description}
                            </Text>
                          ) : null}
                        </VStack>
                        <Spacer />
                        {option.value === value ? <Image systemName="checkmark" color="accentColor" /> : null}
                      </HStack>
                    </Button>
                  ))}
                  {loading ? <ProgressView /> : !options.length ? <Text>{t('no_results')}</Text> : null}
                </Section>
                {footer ? (
                  <Section>
                    <Button label={footer.label} onPress={footer.onPress} />
                  </Section>
                ) : null}
              </List>
              <Toolbar.Content>
                <ToolbarItem placement="cancellationAction">
                  <Button label={t('close')} onPress={onClose} />
                </ToolbarItem>
              </Toolbar.Content>
            </Toolbar>
          </NavigationStack>
        </Group>
      </BottomSheet>
    </Host>
  );
}
