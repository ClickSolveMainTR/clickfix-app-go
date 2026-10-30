import { Text } from '@/ui/text';
import { useContext } from 'react';
import { HeaderCollapsedContext } from './header-scroll';

/** Keep the leading title outside iOS 26's shared glass background. */
export function useCompactHeader(title: string) {
  const collapsed = useContext(HeaderCollapsedContext);
  const headerLeft = collapsed
    ? undefined
    : () => (
        <Text
          accessibilityRole="header"
          numberOfLines={1}
          variant="compactTitle"
          style={{ maxWidth: 210 }}>
          {title}
        </Text>
      );
  return {
    title,
    headerLargeTitle: false,
    headerTitle: collapsed ? title : '',
    headerLeft,
    unstable_headerLeftItems: headerLeft
      ? () => [{ type: 'custom' as const, element: headerLeft(), hidesSharedBackground: true }]
      : undefined,
  };
}
