import { mentionParts } from '@/features/inbox/thread/composer-tokens';
import { type ComponentRef, type ReactNode, type Ref, useState } from 'react';
import { Text as NativeText, Pressable, TextInput, type TextInputProps, View } from 'react-native';
import { cn } from './cn';
import { Icon } from './icon';

export function MessageComposer({
  value,
  onChangeText,
  placeholder,
  label,
  sendLabel,
  onSend,
  disabled,
  inputDisabled,
  note,
  before,
  leading,
  trailing,
  onContentSizeChange,
  inputRef,
  mentions = [],
  ...props
}: TextInputProps & {
  inputRef?: Ref<ComponentRef<typeof TextInput>>;
  mentions?: { uuid: string; name: string }[];
  label: string;
  sendLabel: string;
  onSend: () => void;
  disabled?: boolean;
  inputDisabled?: boolean;
  note?: boolean;
  before?: ReactNode;
  leading?: ReactNode;
  trailing?: ReactNode;
}) {
  const [height, setHeight] = useState(44);
  return (
    <View className="flex-row items-end gap-2">
      {before}
      <View
        className={cn(
          'min-w-0 flex-1 flex-row items-end rounded-3xl border',
          note ? 'border-warning/40 bg-warning/15' : 'border-border bg-card',
        )}>
        {leading}
        <TextInput
          {...props}
          ref={inputRef}
          value={mentions.length ? undefined : value}
          onChangeText={onChangeText}
          multiline
          editable={!inputDisabled}
          onContentSizeChange={(event) => {
            setHeight(event.nativeEvent.contentSize.height);
            onContentSizeChange?.(event);
          }}
          scrollEnabled={height > 144}
          textAlignVertical="top"
          style={{ height: Math.max(44, Math.min(144, height)) }}
          placeholder={placeholder}
          accessibilityLabel={label}
          placeholderTextColorClassName="accent-muted-foreground"
          className={cn(
            'min-w-0 flex-1 py-2.5 font-sans text-[16px] leading-[22px] text-foreground',
            leading ? 'pl-0.5' : 'pl-3.5',
            trailing ? 'pr-0.5' : 'pr-3.5',
          )}>
          {mentions.length
            ? mentionParts(value ?? '', mentions).map((part, index) => (
                <NativeText key={index} className={part.mention ? 'font-sans-bold text-signal-text' : undefined}>
                  {part.text}
                </NativeText>
              ))
            : null}
        </TextInput>
        {trailing}
      </View>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={sendLabel}
        accessibilityState={{ disabled }}
        disabled={disabled}
        onPress={onSend}
        className={cn('size-11 items-center justify-center rounded-full', disabled ? 'bg-secondary' : 'bg-primary')}>
        <Icon name="send" size={19} tone={disabled ? 'muted-foreground' : 'primary-foreground'} />
      </Pressable>
    </View>
  );
}
