import * as Haptics from 'expo-haptics';
import { cloneElement, useState } from 'react';
import type { PressableProps } from 'react-native';
import { Pressable, ScrollView, View } from 'react-native';
import { useTranslation } from 'react-i18next';

import type { ActionMenuProps, MenuOption } from './menu.types';
import { Sheet } from './sheet';
import { Text } from './text';

/** Context-menu fallback for platforms without a native lifted-preview API. */
export function FallbackMenu<T extends string>({
  title,
  trigger,
  options,
  value,
  onPick,
  context,
  disabled,
}: ActionMenuProps<T>) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [submenu, setSubmenu] = useState<MenuOption<T>[] | null>(null);
  const close = () => {
    setOpen(false);
    setSubmenu(null);
  };
  const show = () => {
    if (!disabled && options.length) setOpen(true);
  };
  const control = trigger as React.ReactElement<PressableProps>;
  return (
    <>
      {cloneElement(
        control,
        context
          ? {
              onLongPress: (event) => {
                control.props.onLongPress?.(event);
                show();
              },
            }
          : { onPress: show },
      )}
      <Sheet visible={open} onClose={close}>
        <View className="flex-1 bg-background p-4">
          <View className="flex-row items-center justify-between pb-3">
            <Text variant="headline">{title}</Text>
            <Pressable accessibilityRole="button" accessibilityLabel={t('close')} onPress={close}>
              <Text>{t('close')}</Text>
            </Pressable>
          </View>
          <ScrollView keyboardShouldPersistTaps="handled">
            {(submenu ?? options).map((item) => (
              <Pressable
                key={item.value}
                accessibilityRole="button"
                accessibilityState={{ selected: item.value === value, disabled: item.disabled }}
                disabled={item.disabled}
                onPress={() => {
                  if (item.children) setSubmenu(item.children);
                  else {
                    close();
                    void Haptics.selectionAsync();
                    onPick(item.value);
                  }
                }}
                className="min-h-12 justify-center border-b border-border py-3">
                <Text tone={item.destructive ? 'danger' : 'default'}>
                  {item.label}
                  {item.value === value ? ' ✓' : ''}
                </Text>
                {item.description ? (
                  <Text variant="footnote" tone="muted">
                    {item.description}
                  </Text>
                ) : null}
              </Pressable>
            ))}
          </ScrollView>
        </View>
      </Sheet>
    </>
  );
}
