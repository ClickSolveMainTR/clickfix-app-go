import type { SelectOption } from './select';
export type NativeOptionListProps = {
  visible: boolean;
  title: string;
  options: SelectOption[];
  value?: string | null;
  query: string;
  searchable?: boolean;
  loading?: boolean;
  footer?: { label: string; onPress: () => void };
  onSearch: (query: string) => void;
  onClose: () => void;
  onPick: (value: string) => void;
};
/** Other platforms use OptionSheet's platform implementation. */
export function NativeOptionList(_props: NativeOptionListProps) {
  return null;
}
