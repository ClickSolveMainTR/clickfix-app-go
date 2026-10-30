import { createContext, useContext } from 'react';
export const HeaderCollapsedContext = createContext(false);
export const HeaderScrollContext = createContext<(offset: number) => void>(() => {});
export function useHeaderScroll() {
  return useContext(HeaderScrollContext);
}
