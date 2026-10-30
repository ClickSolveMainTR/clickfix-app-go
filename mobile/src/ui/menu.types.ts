import type { ReactElement } from 'react';
import type { SFSymbol } from 'expo-symbols';

export type MenuOption<T extends string = string> = {
  value: T;
  label: string;
  description?: string;
  systemImage?: SFSymbol;
  destructive?: boolean;
  disabled?: boolean;
  children?: MenuOption<T>[];
};

export type ActionMenuProps<T extends string> = {
  title: string;
  trigger: ReactElement;
  options: MenuOption<T>[];
  value?: string | null;
  onPick: (value: T) => void;
  context?: boolean;
  /** Explicitly sized preview, otherwise iOS lifts the actual trigger. */
  preview?: ReactElement;
  disabled?: boolean;
  /** Stretch a row label to its parent's width. */
  fullWidth?: boolean;
  /** Constrain self-sizing labels such as chat bubbles inside the native host. */
  maxWidth?: number;
  alignment?: 'leading' | 'trailing';
};
