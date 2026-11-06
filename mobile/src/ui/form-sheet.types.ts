import type { ReactElement } from 'react';
export type FormChoice = { value: string; label: string; description?: string };
export type FormField = { key: string; label: string; hint?: string; error?: string; disabled?: boolean } & (
  | {
      kind: 'text';
      value: string;
      onChange: (value: string) => void;
      keyboard?: 'email' | 'number' | 'phone';
      multiline?: boolean;
      placeholder?: string;
    }
  | { kind: 'toggle'; value: boolean; onChange: (value: boolean) => void }
  | { kind: 'pick'; value: string | null; options: FormChoice[]; onChange: (value: string) => void }
  | { kind: 'multi'; values: string[]; options: FormChoice[]; onChange: (value: string[]) => void }
  | { kind: 'date'; value: Date; min?: Date; onChange: (value: Date) => void }
  | { kind: 'button'; onPress: () => void }
  | { kind: 'info'; value: string }
);
export type FormSection = { title?: string; footer?: string; fields: FormField[] };
export type FormSheetProps = {
  visible: boolean;
  title: string;
  onCancel: () => void;
  primary?: { label: string; onPress: () => void; disabled?: boolean; loading?: boolean };
  sections?: FormSection[];
  dirty?: boolean;
  detents?: ('medium' | 'large')[];
  searchable?: boolean;
  /** With `onSearch` the caller filters the options itself; without it the sheet filters by label and description. */
  onSearch?: (value: string) => void;
  children?: ReactElement;
};
