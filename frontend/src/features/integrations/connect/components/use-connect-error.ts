import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useConnectError() {
  const { t } = useLocale();
  return (error: unknown) =>
    appToast.error(
      isApiError(error) ? error.message : t("integrations_connect.failed"),
    );
}
