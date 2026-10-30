"use client";

import { useEffect } from "react";

/**
 * Asks the browser to confirm leaving (reload, tab close, external link)
 * while `active` is true, e.g. a form with unsaved edits. In-app links are
 * client-side navigations the browser does not intercept.
 */
export function useUnsavedChangesGuard(active: boolean) {
  useEffect(() => {
    if (!active) return;
    const warn = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      // Older Chromium/Safari still need returnValue set.
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [active]);
}
