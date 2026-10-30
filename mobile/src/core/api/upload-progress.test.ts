import assert from 'node:assert/strict';
import { test } from 'node:test';
import { fetchUpload } from './upload-progress.ts';

class UploadXHR {
  static last: UploadXHR;
  upload = {
    onprogress: null as ((event: { lengthComputable: boolean; loaded: number; total: number }) => void) | null,
  };
  headers: Record<string, string> = {};
  withCredentials = true;
  status = 201;
  responseText = '{"data":{"uuid":"attachment"}}';
  method = '';
  url = '';
  body?: FormData;
  aborted = false;
  onload = () => {};
  onerror = () => {};
  onabort = () => {};
  constructor() {
    UploadXHR.last = this;
  }
  open(method: string, url: string) {
    this.method = method;
    this.url = url;
  }
  setRequestHeader(name: string, value: string) {
    this.headers[name] = value;
  }
  getAllResponseHeaders() {
    return 'Content-Type: application/json\r\nX-Trace: request:123\r\n';
  }
  send(body: FormData) {
    this.body = body;
  }
  abort() {
    this.aborted = true;
    this.onabort();
  }
}

test('upload transport preserves auth and org, omits cookies/boundary, reports real bytes and response', async () => {
  const original = globalThis.XMLHttpRequest;
  globalThis.XMLHttpRequest = UploadXHR as unknown as typeof XMLHttpRequest;
  try {
    const form = new FormData();
    form.append('file', 'test');
    const request = new Request('https://example.test/v1/tenant/attachments', {
      method: 'POST',
      headers: {
        Authorization: 'Bearer test-token',
        'X-Organization': 'test-org',
        'Content-Type': 'multipart/form-data',
      },
    });
    const progress: number[] = [];
    const controller = new AbortController();
    const promise = fetchUpload(request, controller.signal, { body: form, onProgress: (v) => progress.push(v) });
    const xhr = UploadXHR.last;
    assert.equal(xhr.method, 'POST');
    assert.equal(xhr.url, request.url);
    assert.equal(xhr.headers.authorization, 'Bearer test-token');
    assert.equal(xhr.headers['x-organization'], 'test-org');
    assert.equal(xhr.headers['content-type'], undefined);
    assert.equal(xhr.withCredentials, false);
    assert.equal(xhr.body, form);
    xhr.upload.onprogress?.({ lengthComputable: false, loaded: 1, total: 0 });
    xhr.upload.onprogress?.({ lengthComputable: true, loaded: 25, total: 100 });
    xhr.upload.onprogress?.({ lengthComputable: true, loaded: 100, total: 100 });
    assert.deepEqual(progress, [0, 0.25, 1]);
    xhr.onload();
    const response = await promise;
    assert.equal(response.status, 201);
    assert.equal(response.headers.get('x-trace'), 'request:123');
    assert.deepEqual(await response.json(), { data: { uuid: 'attachment' } });
    controller.abort();
    assert.equal(xhr.aborted, false, 'settled uploads remove abort listeners');
  } finally {
    globalThis.XMLHttpRequest = original;
  }
});
test('upload transport rejects network failures and aborts without sending already-aborted requests', async () => {
  const original = globalThis.XMLHttpRequest;
  globalThis.XMLHttpRequest = UploadXHR as unknown as typeof XMLHttpRequest;
  try {
    const request = new Request('https://example.test/upload', { method: 'POST' });
    const body = new FormData();
    const onProgress = () => {};
    const controller = new AbortController();
    const pending = fetchUpload(request, controller.signal, { body, onProgress });
    controller.abort();
    await assert.rejects(pending, /aborted/);
    assert.equal(UploadXHR.last.aborted, true);
    const failed = fetchUpload(request, new AbortController().signal, { body, onProgress });
    UploadXHR.last.onerror();
    await assert.rejects(failed, /Network request failed/);
    const aborted = new AbortController();
    aborted.abort();
    await assert.rejects(fetchUpload(request, aborted.signal, { body, onProgress }), /aborted/);
    assert.equal(UploadXHR.last.body, undefined);
  } finally {
    globalThis.XMLHttpRequest = original;
  }
});
