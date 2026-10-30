/** Incremental SSE parser: handles split UTF-8, CRLF, comments and multiline data. */
export class SSEParser {
  private buffer = '';
  push(chunk: string, emit: (name: string, data: unknown) => void) {
    this.buffer += chunk;
    if (this.buffer.length > 1_000_000) throw new Error('SSE_EVENT_TOO_LARGE');
    for (;;) {
      const boundary = /\r?\n\r?\n/.exec(this.buffer);
      if (!boundary) break;
      const block = this.buffer.slice(0, boundary.index);
      this.buffer = this.buffer.slice(boundary.index + boundary[0].length);
      let name = 'message';
      const data: string[] = [];
      for (const line of block.split(/\r?\n/)) {
        if (line.startsWith('event:')) name = line.slice(6).trim();
        if (line.startsWith('data:')) data.push(line.slice(5).replace(/^ /, ''));
      }
      if (data.length) emit(name, JSON.parse(data.join('\n')));
    }
  }
}

export async function readSSE(body: ReadableStream<Uint8Array>, emit: (name: string, data: unknown) => void) {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  const parser = new SSEParser();
  let completed = false;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      parser.push(decoder.decode(value, { stream: true }), (name, data) => {
        if (name === 'done') completed = true;
        emit(name, data);
      });
    }
    parser.push(decoder.decode(), emit);
    if (!completed) throw new Error('ASSISTANT_STREAM_INTERRUPTED');
  } finally {
    await reader.cancel().catch(() => undefined);
    reader.releaseLock();
  }
}
