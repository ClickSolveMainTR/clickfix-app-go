import { useCSSVariable } from 'uniwind';
import { usePreferences } from '@/core/preferences';
import {
  BottomSheet,
  Button,
  ConfirmationDialog,
  DatePicker,
  Form,
  Group,
  HStack,
  Host,
  Image,
  NavigationStack,
  Picker,
  ProgressView,
  RNHostView,
  Section,
  Spacer,
  Text,
  TextField,
  Toggle,
  Toolbar,
  ToolbarItem,
  VStack,
  useNativeState,
} from '@expo/ui/swift-ui';
import {
  accessibilityLabel,
  buttonStyle,
  datePickerStyle,
  disabled,
  environment,
  font,
  foregroundStyle,
  interactiveDismissDisabled,
  keyboardType,
  navigationBarTitleDisplayMode,
  navigationTitle,
  pickerStyle,
  presentationDetents,
  presentationDragIndicator,
  searchable,
  tag,
  textInputAutocapitalization,
} from '@expo/ui/swift-ui/modifiers';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { FormField, FormSheetProps } from './form-sheet.types';
export type { FormField, FormSection, FormSheetProps } from './form-sheet.types';

function NativeTextField({ field }: { field: Extract<FormField, { kind: 'text' }> }) {
  const value = useNativeState(field.value);
  useEffect(() => {
    value.set(field.value);
  }, [field.value, value]);
  return (
    <TextField
      text={value}
      placeholder={field.placeholder ?? field.label}
      axis={field.multiline ? 'vertical' : 'horizontal'}
      onTextChange={field.onChange}
      modifiers={[
        accessibilityLabel(field.label),
        disabled(!!field.disabled),
        keyboardType(
          field.keyboard === 'email'
            ? 'email-address'
            : field.keyboard === 'number'
              ? 'numeric'
              : field.keyboard === 'phone'
                ? 'phone-pad'
                : 'default',
        ),
        textInputAutocapitalization(field.keyboard === 'email' ? 'never' : 'sentences'),
      ]}
    />
  );
}
function Field({ field, query }: { field: FormField; query: string }) {
  const { t } = useTranslation('conversation');
  const common = [accessibilityLabel(field.label), disabled(!!field.disabled)];
  if (field.kind === 'text') return <NativeTextField field={field} />;
  if (field.kind === 'toggle')
    return <Toggle label={field.label} isOn={field.value} onIsOnChange={field.onChange} modifiers={common} />;
  if (field.kind === 'date')
    return (
      <DatePicker
        title={field.label}
        selection={field.value}
        onDateChange={field.onChange}
        range={{ start: field.min }}
        displayedComponents={['date', 'hourAndMinute']}
        modifiers={[...common, datePickerStyle('compact')]}
      />
    );
  if (field.kind === 'pick')
    return (
      <Picker
        label={field.label}
        selection={field.value ?? ''}
        onSelectionChange={(v: string) => field.onChange(v)}
        modifiers={[...common, pickerStyle('menu')]}>
        {field.value === null ? <Text modifiers={[tag('')]}>{t('form.choose')}</Text> : null}
        {field.options.map((option) => (
          <Text key={option.value} modifiers={[tag(option.value)]}>
            {option.label}
          </Text>
        ))}
      </Picker>
    );
  if (field.kind === 'multi')
    return (
      <Group>
        {field.options
          .filter(
            (o) =>
              !query || `${o.label} ${o.description ?? ''}`.toLocaleLowerCase().includes(query.toLocaleLowerCase()),
          )
          .map((option) => (
            <Button
              key={option.value}
              modifiers={[
                buttonStyle('plain'),
                accessibilityLabel(`${option.label}${field.values.includes(option.value) ? ', ✓' : ''}`),
              ]}
              onPress={() =>
                field.onChange(
                  field.values.includes(option.value)
                    ? field.values.filter((v) => v !== option.value)
                    : [...field.values, option.value],
                )
              }>
              <HStack>
                <VStack alignment="leading">
                  <Text>{option.label}</Text>
                  {option.description ? (
                    <Text modifiers={[font({ size: 13 }), foregroundStyle('secondary')]}>{option.description}</Text>
                  ) : null}
                </VStack>
                <Spacer />
                {field.values.includes(option.value) ? <Image systemName="checkmark" color="accentColor" /> : null}
              </HStack>
            </Button>
          ))}
      </Group>
    );
  if (field.kind === 'button') return <Button label={field.label} onPress={field.onPress} modifiers={common} />;
  return (
    <HStack>
      <Text>{field.label}</Text>
      <Spacer />
      <Text modifiers={[foregroundStyle('secondary')]}>{field.value}</Text>
    </HStack>
  );
}

/** Native navigation chrome and inset-grouped SwiftUI fields, shared by all forms. */
export function FormSheet({
  visible,
  title,
  onCancel,
  primary,
  sections = [],
  dirty,
  detents = ['medium', 'large'],
  searchable: hasSearch,
  onSearch,
  children,
}: FormSheetProps) {
  const { t, i18n } = useTranslation('conversation');
  const destructive = useCSSVariable('--color-destructive');
  const theme = usePreferences((s) => s.theme);
  const [discard, setDiscard] = useState(false);
  const search = useNativeState('');
  const [query, setQuery] = useState('');
  const cancel = () => {
    if (dirty) setDiscard(true);
    else onCancel();
  };
  return (
    <Host
      style={{ position: 'absolute', width: 0, height: 0 }}
      colorScheme={theme === 'system' ? undefined : theme}
      useViewportSizeMeasurement>
      <BottomSheet
        isPresented={visible}
        onIsPresentedChange={(presented) => {
          if (!presented) cancel();
        }}>
        <Group
          modifiers={[
            presentationDetents(detents),
            presentationDragIndicator('visible'),
            interactiveDismissDisabled(!!dirty || !!primary?.loading),
            environment('locale', i18n.language),
          ]}>
          <NavigationStack>
            <Toolbar modifiers={[navigationTitle(title), navigationBarTitleDisplayMode('inline')]}>
              {children ? (
                <RNHostView>{children}</RNHostView>
              ) : (
                <Form
                  modifiers={
                    hasSearch
                      ? [
                          searchable(search, {
                            prompt: t('common:search'),
                            onChange: (text) => {
                              setQuery(text);
                              onSearch?.(text);
                            },
                          }),
                        ]
                      : []
                  }>
                  {sections.map((section, index) => (
                    <Section
                      key={index}
                      title={section.title}
                      footer={section.footer ? <Text>{section.footer}</Text> : undefined}>
                      {section.fields.map((field) => (
                        <Group key={field.key}>
                          <Field field={field} query={onSearch ? '' : query} />
                          {field.error || field.hint ? (
                            <Text
                              modifiers={[font({ size: 13 }), foregroundStyle(field.error ? String(destructive) : 'secondary')]}>
                              {field.error ?? field.hint}
                            </Text>
                          ) : null}
                        </Group>
                      ))}
                    </Section>
                  ))}
                </Form>
              )}
              <Toolbar.Content>
                <ToolbarItem placement="cancellationAction">
                  <Button label={t('common:cancel')} onPress={cancel} modifiers={[disabled(!!primary?.loading)]} />
                </ToolbarItem>
                {primary ? (
                  <ToolbarItem placement="confirmationAction">
                    {primary.loading ? (
                      <ProgressView />
                    ) : (
                      <Button
                        label={primary.label}
                        onPress={primary.onPress}
                        modifiers={[
                          disabled(!!primary.disabled),
                          font({ weight: 'bold' }),
                          buttonStyle('glassProminent'),
                        ]}
                      />
                    )}
                  </ToolbarItem>
                ) : null}
              </Toolbar.Content>
            </Toolbar>
          </NavigationStack>
          <ConfirmationDialog title={t('form.discard')} isPresented={discard} onIsPresentedChange={setDiscard}>
            <ConfirmationDialog.Trigger>
              <Group>{null}</Group>
            </ConfirmationDialog.Trigger>
            <ConfirmationDialog.Actions>
              <Button
                label={t('form.discard_action')}
                role="destructive"
                onPress={() => {
                  setDiscard(false);
                  onCancel();
                }}
              />
              <Button label={t('common:cancel')} role="cancel" onPress={() => setDiscard(false)} />
            </ConfirmationDialog.Actions>
          </ConfirmationDialog>
        </Group>
      </BottomSheet>
    </Host>
  );
}
