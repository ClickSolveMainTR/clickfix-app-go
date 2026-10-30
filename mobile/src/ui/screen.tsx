import { useHeaderScroll } from '@/features/shell/header-scroll';
import type { ReactNode } from 'react';
import { ScrollView, View, type ScrollViewProps } from 'react-native';
import { KeyboardAwareScrollView, KeyboardStickyView, useKeyboardState } from 'react-native-keyboard-controller';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { withUniwind } from 'uniwind';

import { cn } from './cn';

// Layout mode keeps keyboard-controller from writing contentInset while UIKit
// adjusts the native large-title header. Insets mode can leave a second top gap.
const AwareScrollView = withUniwind(KeyboardAwareScrollView);

/**
 * Scrollable screen body under a native large-title header. The header
 * collapses natively because the scroll view adjusts its insets. Read-only
 * screens never retain keyboard padding when a pushed composer loses focus.
 */
export function Screen({ children, className, ...props }: ScrollViewProps & { className?: string }) {
  const reportScroll = useHeaderScroll();
  return (
    <ScrollView
      automaticallyAdjustKeyboardInsets={false}
      contentInsetAdjustmentBehavior="automatic"
      keyboardShouldPersistTaps="handled"
      keyboardDismissMode="interactive"
      className="flex-1 bg-background"
      contentContainerClassName={cn('gap-4 px-4 pb-10 pt-2', className)}
      {...props}
      scrollEventThrottle={16}
      onScroll={(event) => {
        reportScroll(event.nativeEvent.contentOffset.y);
        props.onScroll?.(event);
      }}>
      {children}
    </ScrollView>
  );
}

/**
 * Form screen: scrollable fields with a bottom action bar that rides on top
 * of the keyboard instead of hiding behind it, and respects the home
 * indicator when the keyboard is closed.
 */
export function FormScreen({
  children,
  footer,
  className,
  ...props
}: ScrollViewProps & { className?: string; footer?: ReactNode }) {
  const insets = useSafeAreaInsets();
  const keyboardVisible = useKeyboardState((state) => state.isVisible);
  return (
    <View className="flex-1 bg-background">
      <AwareScrollView
        mode="layout"
        // Leave room for the footer above the focused field.
        bottomOffset={8}
        contentInsetAdjustmentBehavior="automatic"
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="interactive"
        className="flex-1"
        contentContainerClassName={cn('gap-4 px-4 pt-2', footer ? 'pb-6' : 'pb-10', className)}
        {...props}>
        {children}
      </AwareScrollView>
      {footer ? (
        <KeyboardStickyView>
          <View
            className="gap-2 border-t border-border bg-background px-4 pt-3"
            style={{ paddingBottom: keyboardVisible ? 8 : Math.max(insets.bottom, 12) }}>
            {footer}
          </View>
        </KeyboardStickyView>
      ) : null}
    </View>
  );
}
