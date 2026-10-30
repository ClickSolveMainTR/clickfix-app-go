import { cva, type VariantProps } from 'class-variance-authority';
import { Children, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Text as RNText, type TextProps } from 'react-native';

import { cn } from './cn';

/**
 * Type scale in iOS points: Geist for UI text and Space Grotesk for titles,
 * like the panel (weights are families, see `cn`). Dynamic Type still scales
 * every variant.
 */
const text = cva('font-sans text-foreground', {
  variants: {
    variant: {
      compactTitle: 'text-[28px] leading-[34px] font-sans-bold',
      largeTitle: 'text-[34px] leading-[41px] font-display-bold tracking-tight',
      title: 'text-[22px] leading-[28px] font-display',
      headline: 'text-[17px] leading-[22px] font-sans-semibold',
      body: 'text-[17px] leading-[22px]',
      callout: 'text-[15px] leading-[20px]',
      footnote: 'text-[13px] leading-[18px]',
      /** Secondary metadata, using the original localized casing. */
      caption: 'text-[11px] leading-[13px]',
      /** Big figures (money, counts): tabular digits so rows never shift. */
      figure: 'text-[28px] leading-[34px] font-display-bold tracking-tight tabular-nums',
    },
    tone: {
      default: '',
      muted: 'text-muted-foreground',
      primary: 'text-primary',
      /** Green text: the readable signal shade (green itself is a fill only). */
      signal: 'text-signal-text',
      danger: 'text-destructive',
      success: 'text-success',
      onPrimary: 'text-primary-foreground',
    },
  },
  defaultVariants: { variant: 'body', tone: 'default' },
});

export type TextVariant = NonNullable<VariantProps<typeof text>['variant']>;

/** Locale-aware uppercase for string children (RN textTransform is not). */
function upper(children: ReactNode, lang: string): ReactNode {
  return Children.map(children, (c) => (typeof c === 'string' ? c.toLocaleUpperCase(lang) : c));
}

export function Text({
  variant,
  tone,
  className,
  children,
  uppercase = false,
  ...props
}: TextProps & VariantProps<typeof text> & { className?: string; uppercase?: boolean }) {
  const { i18n } = useTranslation();
  return (
    <RNText className={cn(text({ variant, tone }), className)} {...props}>
      {uppercase ? upper(children, i18n.language) : children}
    </RNText>
  );
}
