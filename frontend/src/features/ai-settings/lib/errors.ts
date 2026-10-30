import { isApiError } from "@/lib/api";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Stale optimistic revision (another admin saved first). */
export function isConflict(error: unknown) {
  return isApiError(error) && error.status === 409;
}

/**
 * Human message for AI API errors. Backend codes come from
 * `aiagents/handler` and `aicore/handler` (`fail`).
 */
export function aiErrorMessage(error: unknown, t: Translate) {
  if (!isApiError(error)) return t("ai_settings.errors.generic");
  switch (error.code) {
    case "CONFLICT":
      return t("ai_settings.errors.conflict");
    case "AI_GUARD_REJECTED":
      return t("ai_settings.errors.guard");
    case "AI_LIMIT_REACHED":
      return t("ai_settings.errors.limit");
    case "AI_CREDITS_EXHAUSTED":
      return t("ai_settings.errors.credits");
    case "AI_PROVIDER_UNAVAILABLE":
      return t("ai_settings.errors.provider");
    case "AI_DISABLED":
      return t("ai_settings.errors.disabled");
    case "AI_NOT_CONFIGURED":
      return t("ai_settings.errors.not_configured");
    case "AI_NOT_ENTITLED":
      return t("ai_settings.errors.not_entitled");
    case "VALIDATION_ERROR":
      return t("ai_settings.errors.validation");
  }
  if (error.status === 403) return t("ai_settings.errors.forbidden");
  if (error.status === 409) return t("ai_settings.errors.conflict");
  return error.message || t("ai_settings.errors.generic");
}
