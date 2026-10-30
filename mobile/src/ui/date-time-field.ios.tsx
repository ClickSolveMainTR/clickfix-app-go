import { DatePicker, Host } from '@expo/ui/swift-ui';
import { datePickerStyle, environment } from '@expo/ui/swift-ui/modifiers';
import { useTranslation } from 'react-i18next';
import { usePreferences } from '@/core/preferences';
export function DateTimeField({
  label,
  value,
  onChange,
  min,
}: {
  label: string;
  value: Date;
  onChange: (date: Date) => void;
  min?: Date;
}) {
  const { i18n } = useTranslation();
  const theme = usePreferences((s) => s.theme);
  return (
    <Host style={{ height: 58, alignSelf: 'stretch' }} colorScheme={theme === 'system' ? undefined : theme}>
      <DatePicker
        title={label}
        selection={value}
        onDateChange={onChange}
        displayedComponents={['date', 'hourAndMinute']}
        range={{ start: min }}
        modifiers={[datePickerStyle('compact'), environment('locale', i18n.language)]}
      />
    </Host>
  );
}
