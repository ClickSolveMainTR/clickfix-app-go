import { isApiError } from "@/lib/api";

/** i18n key of an `ASSISTANT_*` (or shared) API error code. */
export function errorKeyForCode(code: string | undefined): string {
  switch (code) {
    case "ASSISTANT_DISABLED":
      return "assistant.errors.disabled";
    case "ASSISTANT_DAILY_LIMIT":
      return "assistant.errors.daily_limit";
    case "ASSISTANT_AI_UNAVAILABLE":
      return "assistant.errors.ai_unavailable";
    case "ASSISTANT_INTERNAL":
      return "assistant.errors.internal";
    case "ASSISTANT_SUPPORT_UNAVAILABLE":
      return "assistant.errors.support_unavailable";
    case "ASSISTANT_MESSAGE_REJECTED":
      return "assistant.errors.rejected";
    case "ASSISTANT_ACTION_NOT_PENDING":
      return "assistant.errors.not_pending";
    case "ASSISTANT_STREAM_INTERRUPTED":
      return "assistant.errors.interrupted";
    case "FORBIDDEN":
      return "assistant.errors.forbidden";
    case "RATE_LIMITED":
      return "assistant.errors.rate_limited";
    default:
      return "assistant.errors.generic";
  }
}

/** i18n key of a failed assistant request; plain Errors carry their code as message. */
export function errorKey(err: unknown): string {
  if (isApiError(err)) return errorKeyForCode(err.code);
  if (err instanceof Error && err.message.startsWith("ASSISTANT_"))
    return errorKeyForCode(err.message);
  return "assistant.errors.generic";
}

/** i18n key of a document upload / link failure. */
export function documentErrorKey(err: unknown): string {
  const code = isApiError(err)
    ? err.code
    : err instanceof Error
      ? err.message
      : "";
  switch (code) {
    case "ASSISTANT_DOCUMENT_TYPE":
      return "assistant.documents.errors.type";
    case "ASSISTANT_DOCUMENT_TOO_LARGE":
      return "assistant.documents.errors.size";
    case "ASSISTANT_DOCUMENT_LIMIT":
      return "assistant.documents.errors.limit";
    case "ASSISTANT_DOCUMENT_EMPTY":
      return "assistant.documents.errors.empty";
    case "ASSISTANT_DOCUMENT_FETCH":
      return "assistant.documents.errors.fetch";
    case "ASSISTANT_DOCUMENT_PDF":
      return "assistant.documents.errors.pdf";
  }
  return "assistant.documents.errors.generic";
}
