import { createElement } from 'react';
import { Platform, Text, type ColorValue } from 'react-native';
import { SymbolView } from 'expo-symbols';
import { useCSSVariable } from 'uniwind';

import { MATERIAL_FONT } from '@/core/material-font';
import { ICONS, lucideFor, type IconName } from '@/ui/icons';
import { MATERIAL_SYMBOLS } from '@/ui/material-symbols';

export type IconTone =
  | 'on-tile'
  | 'ink'
  | 'foreground'
  | 'muted-foreground'
  | 'primary'
  | 'primary-foreground'
  | 'secondary-foreground'
  | 'destructive'
  | 'success'
  | 'signal-text'
  | 'signal-foreground'
  | 'warning';

const IOS_MAJOR = Platform.OS === 'ios' ? parseInt(String(Platform.Version), 10) || 0 : 0;

type Props = {
  name: IconName;
  tone?: IconTone;
  size?: number;
  weight?: 'regular' | 'medium' | 'semibold';
  color?: string;
  /** Semantic UIKit color for system selection and disabled controls. */
  iosColor?: ColorValue;
  hierarchical?: boolean;
};

/**
 * Semantic icon in the platform's own set: SF Symbol on iOS, Material Symbol
 * on Android (a glyph of the font preloaded with the app fonts), Lucide
 * elsewhere; colored from a theme token.
 */
export function Icon({
  name,
  tone = 'foreground',
  size = 22,
  weight = 'regular',
  color: customColor,
  iosColor,
  hierarchical = false,
}: Props) {
  const raw = useCSSVariable(`--color-${tone}`);
  const color = customColor ?? (typeof raw === 'string' ? raw : undefined);
  const entry = ICONS[name];
  if (Platform.OS !== 'ios' && MATERIAL_FONT) {
    const code = MATERIAL_SYMBOLS[entry.md];
    if (code)
      return (
        <Text
          aria-hidden
          allowFontScaling={false}
          style={{
            fontFamily: MATERIAL_FONT.name,
            fontSize: size,
            lineHeight: size,
            width: size,
            height: size,
            color,
            textAlign: 'center',
            includeFontPadding: false,
          }}>
          {String.fromCharCode(code)}
        </Text>
      );
  }
  const fallback = createElement(lucideFor(name), { size, color, strokeWidth: 1.75 });
  if (Platform.OS !== 'ios') return fallback;
  return (
    <SymbolView
      style={{ width: size, height: size, flexShrink: 0 }}
      name={IOS_MAJOR < (entry.minIOS ?? 0) ? 'circle' : entry.sf}
      size={size}
      tintColor={iosColor ?? color}
      weight={weight}
      type={hierarchical ? 'hierarchical' : 'monochrome'}
    />
  );
}
