import { DropdownMenu, DropdownMenuItem, Host, RNHostView, Text } from '@expo/ui/jetpack-compose';
import * as Haptics from 'expo-haptics';
import { cloneElement, useState } from 'react';
import { type PressableProps } from 'react-native';

import { usePreferences } from '@/core/preferences';

import { FallbackMenu } from './menu-fallback';
import type { ActionMenuProps, MenuOption } from './menu.types';

function TapMenu<T extends string>({
  trigger,
  options,
  value,
  onPick,
  disabled,
  fullWidth,
}: ActionMenuProps<T>) {
  const [open, setOpen] = useState(false);
  const [submenu, setSubmenu] = useState<MenuOption<T>[] | null>(null);
  const theme = usePreferences((s) => s.theme);
  const close = () => {
    setOpen(false);
    setSubmenu(null);
  };
  if (disabled || !options.length) return trigger;
  return (
    <Host
      matchContents={fullWidth ? { vertical: true } : true}
      colorScheme={theme === 'system' ? undefined : theme}
      style={fullWidth ? { alignSelf: 'stretch' } : undefined}>
      <DropdownMenu expanded={open} onDismissRequest={close}>
        <DropdownMenu.Trigger>
          <RNHostView matchContents={fullWidth ? { vertical: true } : true}>
            {cloneElement(trigger as React.ReactElement<PressableProps>, {
              onPress: () => {
                void Haptics.selectionAsync();
                setOpen(true);
              },
            })}
          </RNHostView>
        </DropdownMenu.Trigger>
        <DropdownMenu.Items>
          {(submenu ?? options).map((item) => (
            <DropdownMenuItem
              key={item.value}
              enabled={!item.disabled}
              onClick={() => {
                if (item.children) setSubmenu(item.children);
                else {
                  close();
                  void Haptics.selectionAsync();
                  onPick(item.value);
                }
              }}>
              <DropdownMenuItem.Text>
                <Text>
                  {[item.value === value ? '✓' : '', item.label, item.description]
                    .filter(Boolean)
                    .join(' · ')}
                </Text>
              </DropdownMenuItem.Text>
            </DropdownMenuItem>
          ))}
        </DropdownMenu.Items>
      </DropdownMenu>
    </Host>
  );
}

export function ActionMenu<T extends string>(props: ActionMenuProps<T>) {
  return props.context ? <FallbackMenu {...props} /> : <TapMenu {...props} />;
}
