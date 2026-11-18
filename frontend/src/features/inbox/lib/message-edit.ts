/**
 * Which messages the agent may edit (CF-248). Mirrors the backend rule
 * (conversations/usecase/edit.go): the author's own public text reply,
 * not being handed to the channel, within 15 minutes of sending. The
 * server checks again; this only decides whether "Düzenle" is offered.
 */

/** MessageEditWindow of the backend. */
export const EDIT_WINDOW_MS = 15 * 60_000;

export type EditableMessage = {
  direction: string;
  kind: string;
  private: boolean;
  status: string;
  content: string;
  created_at: string;
  sender: { type: string; uuid?: string | null };
  optimistic?: true;
};

export function canEditMessage(
  m: EditableMessage,
  userUuid: string | null | undefined,
  now: number = Date.now(),
): boolean {
  if (!userUuid || m.optimistic) return false;
  if (m.direction !== "out" || m.private || m.kind !== "text") return false;
  if (m.sender.type !== "agent" || m.sender.uuid !== userUuid) return false;
  if (!m.content || m.status === "pending") return false;
  return now - new Date(m.created_at).getTime() <= EDIT_WINDOW_MS;
}

/** Whether the contact already has the reply (an edit must reach the channel). */
export function deliveredToContact(m: { status: string }): boolean {
  return m.status === "sent" || m.status === "delivered" || m.status === "read";
}
