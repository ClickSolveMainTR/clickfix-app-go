import { generateId, type AttachmentAdapter } from "@assistant-ui/react";

import { documentErrorKey } from "../lib/errors";
import { assistantService } from "../services/assistant.service";

export const DOCUMENT_ACCEPT =
  ".pdf,application/pdf,.txt,.md,.markdown,.csv,.json,.yaml,.yml,.html,.htm,text/plain,text/markdown,text/csv,application/json,application/yaml,text/html";
export const DOCUMENT_MAX_BYTES = 2 * 1024 * 1024;

export type AttachmentDeps = {
  slug: string;
  ensureThread: () => Promise<string>;
  /** Called after a document reached the thread (the document strip refetches). */
  onUploaded: () => void;
  /** Translates a document error key for the attachment's status line. */
  translate: (key: string) => string;
};

/**
 * assistant-ui `AttachmentAdapter` over `POST threads/{uuid}/documents`
 * (CF-191). A file picked in the composer waits as a pending attachment and
 * is uploaded when the message is sent, so a new chat gets its thread first.
 * Documents belong to the thread, not the message: the sent message carries
 * no file content and the document strip lists what the assistant can read.
 */
export function createAttachmentAdapter(
  deps: AttachmentDeps,
): AttachmentAdapter {
  return {
    accept: DOCUMENT_ACCEPT,
    async add({ file }) {
      if (file.size > DOCUMENT_MAX_BYTES)
        throw new Error(deps.translate("assistant.documents.errors.size"));
      return {
        id: generateId(),
        type: "document",
        name: file.name,
        contentType: file.type,
        file,
        status: { type: "requires-action", reason: "composer-send" },
      };
    },
    async send(attachment) {
      const thread = await deps.ensureThread();
      try {
        await assistantService.uploadDocument(
          deps.slug,
          thread,
          attachment.file,
        );
      } catch (err) {
        throw new Error(deps.translate(documentErrorKey(err)));
      }
      deps.onUploaded();
      return {
        id: attachment.id,
        type: attachment.type,
        name: attachment.name,
        contentType: attachment.contentType,
        status: { type: "complete" },
        content: [],
      };
    },
    async remove() {},
  };
}
