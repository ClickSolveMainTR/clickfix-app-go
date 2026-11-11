/**
 * Taking a published post down and publishing it again (CF-182), the part
 * that needs no React: the action asks first, and nothing is sent until the
 * moderator confirms. Free of app imports so it runs under `node --test`.
 */

export type PostKind = "topic" | "reply";
export type VisibilityAction = "unpublish" | "republish";

export type TakedownRequest = {
  action: VisibilityAction;
  kind: PostKind;
  uuid: string;
};

/** Nothing pending, or one request waiting for the moderator's answer. */
export type TakedownState = { pending: TakedownRequest | null };

export const idle: TakedownState = { pending: null };

/** A button press only stages the request; the dialog decides. */
export function request(
  _state: TakedownState,
  req: TakedownRequest,
): TakedownState {
  return { pending: req };
}

export function cancel(_state: TakedownState): TakedownState {
  return idle;
}

/**
 * The moderator confirmed: returns what to send and the idle state. Without
 * a pending request nothing is sent (a stray confirm from a closed dialog).
 */
export function confirm(
  state: TakedownState,
  reason = "",
): {
  send: (TakedownRequest & { reason: string }) | null;
  state: TakedownState;
} {
  if (!state.pending) return { send: null, state: idle };
  // Only a takedown carries a reason; it is optional and bounded by the API.
  const trimmed = state.pending.action === "unpublish" ? reason.trim() : "";
  return { send: { ...state.pending, reason: trimmed }, state: idle };
}

/** The API caps the optional reason at 500 characters. */
export const REASON_MAX = 500;

export function reasonTooLong(reason: string) {
  return reason.trim().length > REASON_MAX;
}

/** Message keys of the confirmation (`forum` namespace) for one request. */
export function dialogCopy(req: Pick<TakedownRequest, "action" | "kind">) {
  return {
    title: `forum.visibility.${req.action}_title_${req.kind}`,
    body: `forum.visibility.${req.action}_body_${req.kind}`,
    action: `forum.visibility.${req.action}`,
    destructive: req.action === "unpublish",
  };
}

/**
 * Why the API refused, as a message key, or null to show the API's own
 * text. 409: the post is no longer in the status the action needs (someone
 * else acted, or a removed reply's topic is down). 404: the post is gone.
 */
export function visibilityProblem(
  status: number | null,
  action: VisibilityAction,
): string | null {
  if (status === 404) return "forum.toast.already_moderated";
  if (status === 409)
    return action === "republish"
      ? "forum.toast.republish_conflict"
      : "forum.toast.unpublish_conflict";
  return null;
}
