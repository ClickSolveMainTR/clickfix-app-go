"use client";

import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Mutation error toast: the API message, else a generic one. */
export function useOncallError() {
  const { t } = useLocale();
  return (error: unknown) =>
    appToast.error(
      isApiError(error) ? error.message : t("oncall.toast.failed"),
    );
}
