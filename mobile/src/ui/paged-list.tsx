import { useHeaderScroll } from '@/features/shell/header-scroll';
import type { UseInfiniteQueryResult } from '@tanstack/react-query';
import type { ReactElement } from 'react';
import { ActivityIndicator, FlatList, RefreshControl } from 'react-native';
import Animated, { LinearTransition, ReduceMotion } from 'react-native-reanimated';

import { cn } from './cn';
import { EmptyState } from './empty-state';
import type { IconName } from './icons';
import { riseIn } from './motion';
import { QueryState } from './query-state';
import { useManualRefresh } from './use-manual-refresh';

const LIST_LAYOUT = LinearTransition.springify().damping(22).stiffness(240).reduceMotion(ReduceMotion.System);

type Page<T> = { items: T[]; total: number; offset: number; limit: number };

/** Offset-paged API list (`items/total/limit/offset`) with pull-to-refresh and endless scroll. */
export function PagedList<T>({
  query,
  header,
  renderItem,
  keyOf,
  empty,
  emptyBody,
  emptyIcon = 'inbox',
  animateLayout = false,
}: {
  query: UseInfiniteQueryResult<{ pages: Page<T>[] }>;
  header: ReactElement;
  renderItem: (item: T, index: number, last: boolean) => ReactElement;
  keyOf: (item: T) => string;
  empty: string;
  emptyBody?: string;
  emptyIcon?: IconName;
  /** Animate edit-mode header and visible row layout without mounting the list again. */
  animateLayout?: boolean;
}) {
  const reportScroll = useHeaderScroll();
  const refresh = useManualRefresh(query);
  const items = query.data?.pages.flatMap((p) => p.items) ?? [];
  return (
    <FlatList
      scrollEventThrottle={16}
      onScroll={(event) => reportScroll(event.nativeEvent.contentOffset.y)}
      data={query.data ? items : []}
      keyExtractor={keyOf}
      contentInsetAdjustmentBehavior="automatic"
      keyboardShouldPersistTaps="handled"
      keyboardDismissMode="interactive"
      automaticallyAdjustKeyboardInsets={false}
      style={{ width: '100%', minWidth: 0 }}
      className="flex-1 bg-background"
      contentContainerStyle={{ width: '100%' }}
      contentContainerClassName="px-4 pb-10 pt-2"
      ListHeaderComponent={
        <Animated.View layout={animateLayout ? LIST_LAYOUT : undefined} className="min-w-0 gap-3 pb-3">
          {header}
        </Animated.View>
      }
      renderItem={({ item, index }) => {
        const last = index === items.length - 1;
        // Rows read as one grouped card: rounded top on the first, bottom on the last.
        // The first screenful rises in row by row; later pages just appear.
        return (
          <Animated.View
            layout={animateLayout ? LIST_LAYOUT : undefined}
            entering={index < 12 ? riseIn(index, 8) : undefined}
            className={cn(
              'w-full min-w-0 overflow-hidden bg-card',
              index === 0 && 'rounded-t-lg',
              last && 'rounded-b-lg',
            )}>
            {renderItem(item, index, last)}
          </Animated.View>
        );
      }}
      ListEmptyComponent={
        query.data ? <EmptyState icon={emptyIcon} title={empty} body={emptyBody} /> : <QueryState query={query}>{() => null}</QueryState>
      }
      onEndReachedThreshold={0.4}
      onEndReached={() => {
        if (query.hasNextPage && !query.isFetchingNextPage) void query.fetchNextPage();
      }}
      ListFooterComponent={query.isFetchingNextPage ? <ActivityIndicator className="py-4" /> : undefined}
      refreshControl={<RefreshControl {...refresh} />}
    />
  );
}

export const PAGE_SIZE = 30;

/** `getNextPageParam` for offset pages. */
export function nextOffset<T>(last: Page<T>) {
  const next = last.offset + last.items.length;
  return next < last.total ? next : undefined;
}
