"use client";

import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Mutation error toast: the API message, else a generic one. */
export function useReportError() {
  const { t } = useLocale();
  return (error: unknown) =>
    appToast.error(
      isApiError(error) ? error.message : t("reports.toast.failed"),
    );
}
