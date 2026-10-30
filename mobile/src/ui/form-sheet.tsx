import { useTranslation } from 'react-i18next';
import { Alert, View } from 'react-native';
import { Button } from './button';
import { DateTimeField } from './date-time-field';
import { SwitchRow } from './fields';
import type { FormSheetProps } from './form-sheet.types';
import { Input } from './input';
import { MultiSelect } from './multi-select';
import { FormScreen } from './screen';
import { Select } from './select';
import { Sheet } from './sheet';
import { Text } from './text';
export type { FormField, FormSection, FormSheetProps } from './form-sheet.types';

export function FormSheet({ visible, title, onCancel, primary, sections = [], dirty, children }: FormSheetProps) {
  const { t } = useTranslation('conversation');
  const cancel = () =>
    dirty
      ? Alert.alert(t('form.discard'), undefined, [
          { text: t('common:cancel'), style: 'cancel' },
          { text: t('form.discard_action'), style: 'destructive', onPress: onCancel },
        ])
      : onCancel();
  return (
    <Sheet visible={visible} onClose={cancel}>
      <View className="flex-1 bg-background">
        <View className="min-h-14 flex-row items-center justify-between border-b border-border px-3">
          <Button title={t('common:cancel')} variant="ghost" onPress={cancel} />
          <Text variant="headline">{title}</Text>
          {primary ? (
            <Button
              title={primary.label}
              variant="ghost"
              disabled={primary.disabled}
              loading={primary.loading}
              onPress={primary.onPress}
            />
          ) : (
            <View className="w-10" />
          )}
        </View>
        <FormScreen>
          {sections.map((section, i) => (
            <View key={i} className="gap-3">
              {section.title ? (
                <Text tone="muted" variant="footnote">
                  {section.title}
                </Text>
              ) : null}
              {section.fields.map((field) => (
                <View key={field.key}>
                  {field.kind === 'text' ? (
                    <Input
                      label={field.label}
                      value={field.value}
                      onChangeText={field.onChange}
                      multiline={field.multiline}
                      placeholder={field.placeholder}
                      editable={!field.disabled}
                      keyboardType={
                        field.keyboard === 'email'
                          ? 'email-address'
                          : field.keyboard === 'number'
                            ? 'number-pad'
                            : field.keyboard === 'phone'
                              ? 'phone-pad'
                              : 'default'
                      }
                      autoCapitalize={field.keyboard === 'email' ? 'none' : 'sentences'}
                    />
                  ) : field.kind === 'toggle' ? (
                    <SwitchRow
                      label={field.label}
                      value={field.value}
                      onChange={field.onChange}
                      disabled={field.disabled}
                    />
                  ) : field.kind === 'pick' ? (
                    <Select
                      label={field.label}
                      value={field.value}
                      options={field.options}
                      onChange={field.onChange}
                      disabled={field.disabled}
                    />
                  ) : field.kind === 'multi' ? (
                    <MultiSelect
                      label={field.label}
                      values={field.values}
                      options={field.options}
                      onChange={field.onChange}
                    />
                  ) : field.kind === 'date' ? (
                    <DateTimeField label={field.label} value={field.value} onChange={field.onChange} min={field.min} />
                  ) : field.kind === 'button' ? (
                    <Button title={field.label} variant="ghost" onPress={field.onPress} />
                  ) : (
                    <Text>
                      {field.label}: {field.value}
                    </Text>
                  )}
                  {field.error || field.hint ? (
                    <Text variant="footnote" tone={field.error ? 'danger' : 'muted'}>
                      {field.error ?? field.hint}
                    </Text>
                  ) : null}
                </View>
              ))}
              {section.footer ? (
                <Text variant="footnote" tone="muted">
                  {section.footer}
                </Text>
              ) : null}
            </View>
          ))}
          {children}
        </FormScreen>
      </View>
    </Sheet>
  );
}
