import assert from 'node:assert/strict';
import { test } from 'node:test';
import { SSEParser, readSSE } from './sse.ts';

test('SSE parser handles every byte boundary, CRLF, comments and Unicode', () => {
  const parser = new SSEParser();
  const events: unknown[] = [];
  const wire = ': heartbeat\r\n\r\nevent: delta\r\ndata: {"text":"Türkçe 👋"}\r\n\r\nevent: done\ndata: {}\n\n';
  const bytes = new TextEncoder().encode(wire);
  const decoder = new TextDecoder();
  for (const b of bytes)
    parser.push(decoder.decode(new Uint8Array([b]), { stream: true }), (name, data) => events.push([name, data]));
  assert.deepEqual(events, [
    ['delta', { text: 'Türkçe 👋' }],
    ['done', {}],
  ]);
});

test('unexpected EOF fails instead of silently accepting a partial answer', async () => {
  const body = new ReadableStream<Uint8Array>({
    start(c) {
      c.enqueue(new TextEncoder().encode('event: delta\ndata: {"text":"partial"}\n\n'));
      c.close();
    },
  });
  await assert.rejects(() => readSSE(body, () => {}), /INTERRUPTED/);
});
