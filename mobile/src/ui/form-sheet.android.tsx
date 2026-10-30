import { usePreferences } from '@/core/preferences';
import {
  Button,
  Checkbox,
  CircularProgressIndicator,
  Column,
  DateTimePicker,
  Host,
  ModalBottomSheet,
  OutlinedTextField,
  RNHostView,
  Row,
  Switch,
  Text,
  useNativeState,
} from '@expo/ui/jetpack-compose';
import { fillMaxSize, fillMaxWidth, paddingAll, verticalScroll, weight } from '@expo/ui/jetpack-compose/modifiers';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from 'react-native';
import type { FormField, FormSheetProps } from './form-sheet.types';
export type { FormField, FormSection, FormSheetProps } from './form-sheet.types';

function TextField({ field }: { field: Extract<FormField, { kind: 'text' }> }) {
  const value = useNativeState(field.value);
  useEffect(() => {
    value.set(field.value);
  }, [field.value, value]);
  return (
    <OutlinedTextField
      value={value}
      onValueChange={field.onChange}
      enabled={!field.disabled}
      isError={!!field.error}
      singleLine={!field.multiline}
      keyboardOptions={{
        keyboardType: field.keyboard ?? 'text',
        capitalization: field.keyboard === 'email' ? 'none' : 'sentences',
      }}
      modifiers={[fillMaxWidth()]}>
      <OutlinedTextField.Label>
        <Text>{field.label}</Text>
      </OutlinedTextField.Label>
      {field.error || field.hint ? (
        <OutlinedTextField.SupportingText>
          <Text>{field.error ?? field.hint}</Text>
        </OutlinedTextField.SupportingText>
      ) : null}
    </OutlinedTextField>
  );
}
function Field({ field, query }: { field: FormField; query: string }) {
  const [expanded, setExpanded] = useState(false);
  if (field.kind === 'text') return <TextField field={field} />;
  if (field.kind === 'date')
    return (
      <Column>
        <Text>{field.label}</Text>
        <DateTimePicker
          initialDate={field.value.toISOString()}
          onDateSelected={(date) => {
            const next = new Date(field.value);
            next.setFullYear(date.getFullYear(), date.getMonth(), date.getDate());
            field.onChange(next);
          }}
          selectableDates={{ start: field.min }}
          displayedComponents="date"
        />
        <DateTimePicker
          initialDate={field.value.toISOString()}
          onDateSelected={(date) => {
            const next = new Date(field.value);
            next.setHours(date.getHours(), date.getMinutes());
            field.onChange(next);
          }}
          displayedComponents="hourAndMinute"
        />
      </Column>
    );
  if (field.kind === 'toggle')
    return (
      <Row>
        <Text modifiers={[weight(1)]}>{field.label}</Text>
        <Switch value={field.value} onCheckedChange={field.onChange} enabled={!field.disabled} />
      </Row>
    );
  if (field.kind === 'button')
    return (
      <Button onClick={field.onPress} enabled={!field.disabled}>
        <Text>{field.label}</Text>
      </Button>
    );
  if (field.kind === 'info')
    return (
      <Text>
        {field.label}: {field.value}
      </Text>
    );
  if (field.kind === 'pick')
    return (
      <Column>
        <Button onClick={() => setExpanded(!expanded)} enabled={!field.disabled}>
          <Text>
            {field.label}: {field.options.find((o) => o.value === field.value)?.label ?? ''}
          </Text>
        </Button>
        {expanded
          ? field.options.map((o) => (
              <Button
                key={o.value}
                onClick={() => {
                  field.onChange(o.value);
                  setExpanded(false);
                }}>
                <Text>{o.label}</Text>
              </Button>
            ))
          : null}
      </Column>
    );
  return (
    <Column>
      {field.options
        .filter((o) => !query || o.label.toLocaleLowerCase().includes(query.toLocaleLowerCase()))
        .map((option) => (
          <Row key={option.value}>
            <Checkbox
              value={field.values.includes(option.value)}
              onCheckedChange={(checked) =>
                field.onChange(
                  checked ? [...field.values, option.value] : field.values.filter((v) => v !== option.value),
                )
              }
              enabled={!field.disabled}
            />
            <Text>{option.label}</Text>
          </Row>
        ))}
    </Column>
  );
}
export function FormSheet({
  visible,
  title,
  onCancel,
  primary,
  sections = [],
  dirty,
  detents,
  searchable,
  onSearch,
  children,
}: FormSheetProps) {
  const { t } = useTranslation('conversation');
  const theme = usePreferences((s) => s.theme);
  const [query, setQuery] = useState('');
  const cancel = () => {
    if (primary?.loading) return;
    if (!dirty) return onCancel();
    Alert.alert(t('form.discard'), undefined, [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t('form.discard_action'), style: 'destructive', onPress: onCancel },
    ]);
  };
  if (!visible) return null;
  return (
    <Host
      style={{ position: 'absolute', width: 0, height: 0 }}
      useViewportSizeMeasurement
      colorScheme={theme === 'system' ? undefined : theme}>
      <ModalBottomSheet
        showDragHandle
        skipPartiallyExpanded={detents?.length === 1 && detents[0] === 'large'}
        onDismissRequest={cancel}
        sheetGesturesEnabled={!dirty && !primary?.loading}>
        <Column modifiers={[fillMaxSize(), paddingAll(16)]}>
          <Row modifiers={[fillMaxWidth()]}>
            <Button onClick={cancel}>
              <Text>{t('common:cancel')}</Text>
            </Button>
            <Text modifiers={[weight(1)]}>{title}</Text>
            {primary ? (
              primary.loading ? (
                <CircularProgressIndicator />
              ) : (
                <Button onClick={primary.onPress} enabled={!primary.disabled}>
                  <Text>{primary.label}</Text>
                </Button>
              )
            ) : null}
          </Row>
          {children ? (
            <RNHostView modifiers={[weight(1)]}>{children}</RNHostView>
          ) : (
            <Column modifiers={[weight(1), fillMaxWidth(), verticalScroll()]}>
              {searchable ? (
                <TextField
                  field={{
                    key: 'search',
                    kind: 'text',
                    label: t('common:search'),
                    value: query,
                    onChange: (v) => {
                      setQuery(v);
                      onSearch?.(v);
                    },
                  }}
                />
              ) : null}
              {sections.map((section, i) => (
                <Column key={i} modifiers={[paddingAll(8), fillMaxWidth()]}>
                  {section.title ? <Text>{section.title}</Text> : null}
                  {section.fields.map((field) => (
                    <Field key={field.key} field={field} query={query} />
                  ))}
                  {section.footer ? <Text>{section.footer}</Text> : null}
                </Column>
              ))}
            </Column>
          )}
        </Column>
      </ModalBottomSheet>
    </Host>
  );
}
