import { useTranslation } from 'react-i18next';
import { FormSheet } from './form-sheet';
import type { NativeOptionListProps } from './native-option-list';
export function NativeOptionList({
  visible,
  title,
  options,
  value,
  query,
  searchable,
  loading,
  footer,
  onSearch,
  onClose,
  onPick,
}: NativeOptionListProps) {
  const { t } = useTranslation();
  return (
    <FormSheet
      visible={visible}
      title={title}
      onCancel={onClose}
      searchable={searchable}
      onSearch={onSearch}
      sections={[
        {
          fields: [
            ...options
              .filter(
                (option) =>
                  !query ||
                  `${option.label} ${option.description ?? ''}`.toLocaleLowerCase().includes(query.toLocaleLowerCase()),
              )
              .map((option) => ({
                key: option.value,
                kind: 'button' as const,
                label: `${option.value === value ? '✓ ' : ''}${option.label}`,
                onPress: () => onPick(option.value),
              })),
            ...(loading || !options.length
              ? [{ key: 'state', kind: 'info' as const, label: loading ? t('loading') : t('no_results'), value: '' }]
              : []),
            ...(footer
              ? [{ key: 'footer', kind: 'button' as const, label: footer.label, onPress: footer.onPress }]
              : []),
          ],
        },
      ]}
    />
  );
}
