/** Picker value meaning "send without a closing text" (CF-250). */
export const NO_CLOSING = "none";

type Options = {
  items: { uuid: string; text: string }[];
  selected?: string | null;
};

/**
 * The composer's closing text state: shown only for replies when the
 * conversation has texts; the agent's pick in this composer wins over the
 * server's starting pick (their last one, else the default). `send` is the
 * `closing_text` field of the post, undefined to leave it to the server.
 */
export function closingState(
  data: Options | undefined,
  pick: string | null,
  reply: boolean,
) {
  const items = data?.items ?? [];
  const show = reply && items.length > 0;
  let value = pick ?? data?.selected ?? NO_CLOSING;
  if (value !== NO_CLOSING && !items.some((o) => o.uuid === value)) {
    value = NO_CLOSING;
  }
  return {
    show,
    value,
    preview: show ? items.find((o) => o.uuid === value)?.text : undefined,
    send: show ? value : undefined,
  };
}
