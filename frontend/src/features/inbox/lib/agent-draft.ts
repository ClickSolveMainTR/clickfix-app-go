/**
 * Which autonomous agent job the composer offers as a draft (CF-64,
 * CF-246). Pure so it can be tested without React.
 *
 * - `draft`: the agent's reply waits for review (draft mode).
 * - `handoff` with model text: the agent could not answer safely (low
 *   confidence, no checked source). The text is offered as an unsourced
 *   suggestion the agent must verify; nothing reaches the customer until a
 *   teammate edits and sends it.
 *
 * A teammate's public reply after the job hides it. Bot messages (the
 * handoff notice, auto replies) do not.
 */

export type AgentDraftJob = {
  uuid: string;
  task: string;
  status: string;
  result: Record<string, unknown>;
  created_at: string;
};

export type AgentDraftMessage = {
  direction: string;
  private: boolean;
  created_at: string;
  sender: { type: string };
};

export type AgentDraftConversation = {
  last_message_at: string;
  last_message_direction: string;
};

export type AgentDraft = {
  uuid: string;
  text: string;
  confidence: number | null;
  /** Handoff text without a checked source: label it and ask to verify. */
  unsourced: boolean;
};

export function pickAgentDraft(
  jobs: readonly AgentDraftJob[] | undefined,
  conversation: AgentDraftConversation,
  messages: readonly AgentDraftMessage[] = [],
): AgentDraft | null {
  // Jobs come newest first: only the latest reply job counts.
  const job = jobs?.find((j) => j.task === "reply");
  if (!job || (job.status !== "draft" && job.status !== "handoff")) return null;
  const reply = job.result.reply;
  if (typeof reply !== "string" || !reply.trim()) return null;
  const since = Date.parse(job.created_at);
  if (messages.length > 0) {
    const answered = messages.some(
      (m) =>
        m.direction === "out" &&
        !m.private &&
        m.sender.type === "agent" &&
        Date.parse(m.created_at) > since,
    );
    if (answered) return null;
  } else if (
    // Messages not loaded: fall back to the conversation summary.
    conversation.last_message_direction === "out" &&
    Date.parse(conversation.last_message_at) > since
  ) {
    return null;
  }
  const confidence = job.result.confidence;
  return {
    uuid: job.uuid,
    text: reply.trim(),
    confidence: typeof confidence === "number" ? confidence : null,
    unsourced: job.status === "handoff",
  };
}
