"use client";

import { useTheme } from "next-themes";
import { useEffect, type ReactNode } from "react";
import { Toaster, toast } from "sonner";

import { ApiError, setGlobalApiErrorHandler } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

export function ToastProvider({ children }: { children: ReactNode }) {
  const { theme } = useTheme();
  const { t } = useLocale();

  useEffect(() => {
    setGlobalApiErrorHandler((error: ApiError) => {
      // One toast per kind of failure: parallel requests failing the same
      // way (e.g. the API restarting) update it instead of stacking.
      const id = `api-error-${error.status}-${error.code}`;
      if (error.isUnauthorized) {
        toast.error(t("auth.session_expired"), { id });
        return;
      }
      if (error.isForbidden) {
        toast.error(t("common.error_forbidden"), { id });
        return;
      }
      if (error.isNotFound) {
        toast.error(t("common.error_not_found"), { id });
        return;
      }
      if (error.isValidation) {
        toast.error(error.message || t("common.error_validation"), { id });
        return;
      }
      if (error.isServer) {
        // A specific code (UPSTREAM_UNAVAILABLE, AI_DISABLED, …) carries its
        // own localized message; only unexpected failures get the generic
        // text with the request id for support.
        const codeKey = `errors.codes.${error.code}`;
        const byCode = t(codeKey);
        const message =
          byCode !== codeKey && error.code !== "INTERNAL_ERROR"
            ? byCode
            : error.requestId
              ? `${t("common.error_server")} (${error.requestId})`
              : t("common.error_server");
        toast.error(message, { id });
        return;
      }
      toast.error(error.message || t("common.error_generic"), { id });
    });

    return () => setGlobalApiErrorHandler(null);
  }, [t]);

  return (
    <>
      {children}
      <Toaster
        richColors
        position="top-right"
        theme={theme === "dark" ? "dark" : "light"}
      />
    </>
  );
}

export const appToast = {
  success: (message: string) => toast.success(message),
  error: (message: string) => toast.error(message),
  warning: (message: string) => toast.warning(message),
  info: (message: string) => toast.info(message),
};
