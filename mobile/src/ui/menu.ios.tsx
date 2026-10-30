import { Button, ContextMenu, Host, Menu, RNHostView, Toggle, VStack } from '@expo/ui/swift-ui';
import {
  accessibilityHint,
  accessibilityLabel,
  accessibilityValue,
  frame,
  contentShape,
  shapes,
  disabled as nativeDisabled,
} from '@expo/ui/swift-ui/modifiers';
import * as Haptics from 'expo-haptics';
import { useState } from 'react';
import { View } from 'react-native';

import { usePreferences } from '@/core/preferences';

import { menuSymbol } from './menu-symbol';
import type { ActionMenuProps, MenuOption } from './menu.types';

function Items<T extends string>({ options, value, onPick }: Pick<ActionMenuProps<T>, 'options' | 'value' | 'onPick'>) {
  return options.map((item: MenuOption<T>) => {
    if (item.children) {
      return (
        <Menu key={item.value} label={item.label} systemImage={menuSymbol(item)}>
          <Items options={item.children} value={value} onPick={onPick} />
        </Menu>
      );
    }
    const props = {
      label: [item.label, item.description].filter(Boolean).join(' · '),
      systemImage: menuSymbol(item),
      modifiers: [
        nativeDisabled(!!item.disabled),
        accessibilityLabel([item.label, item.description].filter(Boolean).join(', ')),
      ],
    };
    const pick = () => {
      void Haptics.selectionAsync();
      onPick(item.value);
    };
    return value === undefined ? (
      <Button key={item.value} {...props} role={item.destructive ? 'destructive' : 'default'} onPress={pick} />
    ) : (
      <Toggle key={item.value} {...props} isOn={item.value === value} onIsOnChange={pick} />
    );
  });
}

/** The visible control is the native menu's label, so menus stay anchored to their source. */
export function ActionMenu<T extends string>({
  title,
  trigger,
  options,
  value,
  onPick,
  context,
  preview,
  disabled,
  fullWidth,
  maxWidth,
  alignment = 'leading',
}: ActionMenuProps<T>) {
  const theme = usePreferences((s) => s.theme);
  const [availableWidth, setAvailableWidth] = useState(0);
  const [size, setSize] = useState({ width: 0, height: 0 });
  if (!context && (disabled || !options.length)) return trigger;
  const triggerProps = trigger.props as {
    accessibilityLabel?: string;
    accessibilityHint?: string;
    label?: string;
    accessibilityValue?: { text?: string };
  };
  const triggerModifiers = [
    accessibilityLabel(triggerProps.accessibilityLabel ?? triggerProps.label ?? title),
    ...(triggerProps.accessibilityHint ? [accessibilityHint(triggerProps.accessibilityHint)] : []),
    ...(triggerProps.accessibilityValue?.text ? [accessibilityValue(triggerProps.accessibilityValue.text)] : []),
  ];
  const label = (
    <RNHostView boundedByHost={fullWidth} matchContents={fullWidth ? { vertical: true } : true}>
      <View
        onLayout={({ nativeEvent: { layout } }) => {
          if (layout.width !== size.width || layout.height !== size.height) {
            setSize({ width: layout.width, height: layout.height });
          }
        }}
        pointerEvents={context ? 'auto' : 'none'}
        collapsable={false}
        style={{
          maxWidth,
          width: fullWidth ? availableWidth : undefined,
          alignItems: fullWidth ? 'stretch' : alignment === 'trailing' ? 'flex-end' : 'flex-start',
        }}>
        {trigger}
      </View>
    </RNHostView>
  );
  const menu = (
    <Host
      ignoreSafeArea="all"
      alignment={alignment}
      colorScheme={theme === 'system' ? undefined : theme}
      matchContents={fullWidth ? { vertical: true } : true}
      style={{
        maxWidth,
        width: fullWidth ? availableWidth : undefined,
        flexShrink: 1,
        alignSelf: fullWidth ? 'stretch' : alignment === 'trailing' ? 'flex-end' : 'flex-start',
      }}>
      {context ? (
        <ContextMenu modifiers={[contentShape(shapes.rectangle())]}>
          <ContextMenu.Trigger>
            <VStack modifiers={[contentShape(shapes.rectangle())]}>{label}</VStack>
          </ContextMenu.Trigger>
          <ContextMenu.Items>
            {disabled ? null : <Items options={options} value={value} onPick={onPick} />}
          </ContextMenu.Items>
          {preview && size.width > 0 && size.height > 0 ? (
            <ContextMenu.Preview>
              <VStack modifiers={[frame({ width: size.width, height: Math.min(size.height, 320) })]}>
                <RNHostView>
                  <View
                    pointerEvents="none"
                    style={{
                      width: size.width,
                      height: Math.min(size.height, 320),
                      overflow: 'hidden',
                      borderRadius: 18,
                    }}>
                    {preview}
                  </View>
                </RNHostView>
              </VStack>
            </ContextMenu.Preview>
          ) : null}
        </ContextMenu>
      ) : (
        <Menu label={label} modifiers={triggerModifiers}>
          <Items options={options} value={value} onPick={onPick} />
        </Menu>
      )}
    </Host>
  );
  // Measure the actual list/split pane, never the window or SwiftUI's intrinsic width.
  return fullWidth ? (
    <View
      style={{ width: '100%', minWidth: 0, maxWidth }}
      onLayout={({ nativeEvent: { layout } }) => setAvailableWidth(layout.width)}>
      {availableWidth > 0 ? menu : null}
    </View>
  ) : (
    menu
  );
}
