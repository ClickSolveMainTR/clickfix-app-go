import { useRef, useState } from 'react';

export function useManualRefresh(query: { isRefetching: boolean; refetch: () => Promise<unknown> }) {
  const [manual, setManual] = useState(false);
  const pending = useRef(false);
  const onRefresh = async () => {
    if (pending.current) return;
    pending.current = true;
    setManual(true);
    try {
      await query.refetch();
    } finally {
      pending.current = false;
      setManual(false);
    }
  };
  return { refreshing: manual && query.isRefetching, onRefresh: () => void onRefresh() };
}
