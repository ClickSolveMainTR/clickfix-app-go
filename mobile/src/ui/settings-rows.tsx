import type { ReactNode } from 'react';
import { TextInput, View, type TextInputProps } from 'react-native';

import { Avatar } from './avatar';
import { List, ListRow, SectionHeader } from './list';
import { Text } from './text';

/** The account card used by Settings and its pushed profile form. */
export function AccountRow({ name, email, onPress }: { name: string; email?: string; onPress?: () => void }) {
  return (
    <List>
      <ListRow
        leading={<Avatar name={name} size={56} />}
        title={name}
        titleLines={2}
        subtitle={email}
        onPress={onPress}
        last
      />
    </List>
  );
}

/** Native text input inside a shared Settings row; no nested card or field border. */
export function SettingsTextRow({ label, last, ...props }: TextInputProps & { label: string; last?: boolean }) {
  return (
    <ListRow
      title={label}
      titleLines={2}
      last={last}
      trailing={
        <TextInput
          accessibilityLabel={label}
          placeholderTextColorClassName="accent-muted-foreground"
          className="min-h-11 min-w-0 flex-1 font-sans text-[17px] text-foreground"
          textAlign="right"
          returnKeyType="done"
          {...props}
        />
      }
    />
  );
}

export function SettingsSection({
  title,
  footer,
  children,
}: {
  title?: string;
  footer?: string;
  children: ReactNode;
}) {
  return (
    <View>
      {title ? <SectionHeader title={title} /> : null}
      <List>{children}</List>
      {footer ? (
        <Text variant="footnote" tone="muted" className="px-4 pt-2">
          {footer}
        </Text>
      ) : null}
    </View>
  );
}
