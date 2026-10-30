import { useMemo, useState, type ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, KeyboardAvoidingView, Platform, Pressable, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { NativeOptionList } from './native-option-list';

import { useKeyboardState } from 'react-native-keyboard-controller';
import { cn } from './cn';
import { EmptyState } from './empty-state';
import { Icon } from './icon';
import type { IconName } from './icons';
import { ActionMenu } from './menu';
import { SearchField } from './search-field';
import { Sheet } from './sheet';
import { Skeleton } from './skeleton';
import { Text } from './text';

export type SelectOption<T extends string = string> = {
  value: T;
  label: string;
  description?: string;
  destructive?: boolean;
  icon?: IconName;
};

/**
 * A row with the current value: native menus for short fixed lists,
 * compact sheets for long/searchable lists.
 */
export function Select<T extends string>({
  label,
  value,
  options,
  onChange,
  placeholder,
  icon,
  error,
  searchable,
  disabled,
  footer,
}: {
  label: string;
  value: T | null | undefined;
  options: SelectOption<T>[];
  onChange: (v: T) => void;
  placeholder?: string;
  icon?: IconName;
  error?: string;
  searchable?: boolean;
  disabled?: boolean;
  /** Extra row at the end of the sheet (e.g. "new customer"). */
  footer?: { label: string; onPress: () => void };
}) {
  const [open, setOpen] = useState(false);
  const current = options.find((o) => o.value === value);
  return (
    <View className="gap-1.5">
      <View className="flex-row items-center gap-1.5">
        {icon ? <Icon name={icon} tone="muted-foreground" size={14} weight="medium" /> : null}
        <Text variant="footnote" tone="muted" className="font-sans-medium">
          {label}
        </Text>
      </View>

      <OptionSheet
        trigger={
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={label}
            accessibilityValue={{ text: current?.label }}
            disabled={disabled}
            onPress={() => setOpen(true)}
            className={cn(
              'min-h-[52px] flex-row items-center gap-2 rounded-lg border bg-card px-4 active:opacity-80',
              error ? 'border-destructive' : 'border-border',
              disabled && 'opacity-50',
            )}>
            <View className="flex-1">
              <Text numberOfLines={1} tone={current ? 'default' : 'muted'}>
                {current?.label ?? placeholder ?? ''}
              </Text>
              {current?.description ? (
                <Text variant="footnote" tone="muted" numberOfLines={1}>
                  {current.description}
                </Text>
              ) : null}
            </View>
            <Icon name="chevronDown" tone="muted-foreground" size={18} />
          </Pressable>
        }
        disabled={disabled}
        fullWidth
        visible={open}
        title={label}
        options={options}
        value={value}
        searchable={searchable ?? options.length > 8}
        footer={footer ? { ...footer, onPress: () => (setOpen(false), footer.onPress()) } : undefined}
        onClose={() => setOpen(false)}
        onPick={(v) => {
          setOpen(false);
          onChange(v);
        }}
      />
      {error ? (
        <Text variant="footnote" tone="danger">
          {error}
        </Text>
      ) : null}
    </View>
  );
}

/** The sheet behind `Select`; also usable on its own (search-as-you-type lists pass `onSearch`). */
export function OptionSheet<T extends string>({
  trigger,
  disabled,
  fullWidth,
  visible,
  title,
  options,
  value,
  searchable,
  onSearch,
  loading,
  footer,
  initialQuery,
  onClose,
  onPick,
}: {
  trigger?: ReactElement;
  disabled?: boolean;
  fullWidth?: boolean;
  visible: boolean;
  title: string;
  options: SelectOption<T>[];
  value?: T | null;
  searchable?: boolean;
  /** Search text the sheet opens with (e.g. the composer's "/word" shortcut). */
  initialQuery?: string;
  /** Server-side search: the sheet shows `options` as given and reports the text. */
  onSearch?: (q: string) => void;
  loading?: boolean;
  footer?: { label: string; onPress: () => void };
  onClose: () => void;
  onPick: (v: T) => void;
}) {
  const { t } = useTranslation();
  const insets = useSafeAreaInsets();
  const keyboardVisible = useKeyboardState((state) => state.isVisible);
  const [q, setQ] = useState('');
  // Each opening starts from the given text, not from the last search.
  const [wasVisible, setWasVisible] = useState(visible);
  if (visible !== wasVisible) {
    setWasVisible(visible);
    if (visible) setQ(initialQuery ?? '');
  }
  const shown = useMemo(() => {
    if (onSearch || !q.trim()) return options;
    const needle = q.trim().toLocaleLowerCase();
    return options.filter((o) => `${o.label} ${o.description ?? ''}`.toLocaleLowerCase().includes(needle));
  }, [options, q, onSearch]);

  if (trigger && !searchable && !onSearch && !footer && options.length <= 8) {
    return (
      <ActionMenu
        title={title}
        trigger={trigger}
        options={options}
        value={value}
        onPick={onPick}
        disabled={disabled}
        fullWidth={fullWidth}
      />
    );
  }

  if (Platform.OS === 'ios' || Platform.OS === 'android')
    return (
      <>
        {trigger}
        <NativeOptionList
          visible={visible}
          title={title}
          options={shown}
          value={value}
          query={q}
          searchable={!!searchable || !!onSearch}
          loading={loading}
          footer={footer}
          onSearch={(v) => {
            setQ(v);
            onSearch?.(v);
          }}
          onClose={onClose}
          onPick={(v) => onPick(v as T)}
        />
      </>
    );

  return (
    <>
      {trigger}
      <Sheet visible={visible} onClose={onClose}>
        <KeyboardAvoidingView
          behavior={undefined}
          className="flex-1 bg-background"
          style={{ paddingBottom: keyboardVisible ? 0 : insets.bottom }}>
          <View className="flex-row items-center gap-3 border-b border-border px-4 py-3">
            <Text variant="headline" className="flex-1" numberOfLines={1}>
              {title}
            </Text>
            <Pressable accessibilityRole="button" accessibilityLabel={t('close')} onPress={onClose} hitSlop={10}>
              <Icon name="close" tone="muted-foreground" size={22} />
            </Pressable>
          </View>
          {searchable || onSearch ? (
            <View className="px-4 pt-3">
              <SearchField
                value={q}
                onChangeText={(v) => {
                  setQ(v);
                  onSearch?.(v);
                }}
                placeholder={t('search')}
              />
            </View>
          ) : null}
          <FlatList
            style={{ flex: 1 }}
            data={shown}
            keyExtractor={(o) => o.value}
            keyboardShouldPersistTaps="handled"
            keyboardDismissMode="interactive"
            contentContainerClassName="px-4 py-3"
            ListEmptyComponent={
              <View className="gap-2">
                {loading ? (
                  <>
                    <Skeleton className="h-12" />
                    <Skeleton className="h-12" />
                  </>
                ) : (
                  <EmptyState icon="search" title={t('no_results')} />
                )}
              </View>
            }
            renderItem={({ item, index }) => (
              <Pressable
                accessibilityRole="button"
                accessibilityState={{ selected: item.value === value }}
                onPress={() => onPick(item.value)}
                className={cn(
                  'min-h-12 flex-row items-center gap-3 border-x border-border bg-card px-4 py-3 active:bg-muted',
                  index === 0 && 'rounded-t-lg border-t',
                  index === shown.length - 1 ? 'rounded-b-lg border-b' : 'border-b',
                )}>
                {item.icon ? <Icon name={item.icon} size={22} tone="primary" /> : null}
                <View className="flex-1 gap-0.5">
                  <Text>{item.label}</Text>
                  {item.description ? (
                    <Text variant="footnote" tone="muted">
                      {item.description}
                    </Text>
                  ) : null}
                </View>
                {item.value === value ? <Icon name="check" tone="primary" size={18} /> : null}
              </Pressable>
            )}
            ListFooterComponent={
              footer ? (
                <Pressable
                  accessibilityRole="button"
                  onPress={footer.onPress}
                  className="mt-3 min-h-12 flex-row items-center gap-3 rounded-lg border border-dashed border-border px-4 active:bg-muted">
                  <Icon name="add" tone="primary" size={18} />
                  <Text tone="primary" className="font-sans-semibold">
                    {footer.label}
                  </Text>
                </Pressable>
              ) : undefined
            }
          />
        </KeyboardAvoidingView>
      </Sheet>
    </>
  );
}
