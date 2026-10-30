/** XHR exposes native upload byte progress; authentication/retries remain in the shared client. */
export type UploadProgress = { body: FormData; onProgress: (fraction: number) => void };
export function fetchUpload(
  request: Request,
  signal: AbortSignal,
  upload: UploadProgress,
): Promise<Response> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const abort = () => xhr.abort();
    const cleanup = () => signal.removeEventListener('abort', abort);
    xhr.open(request.method, request.url);
    xhr.withCredentials = false;
    request.headers.forEach((value, name) => {
      // Native sets the multipart boundary when sending FormData.
      if (name.toLowerCase() !== 'content-type') xhr.setRequestHeader(name, value);
    });
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable && event.total > 0) upload.onProgress(event.loaded / event.total);
    };
    xhr.onload = () => {
      cleanup();
      if (!xhr.status) {
        reject(new TypeError('Network request failed'));
        return;
      }
      const headers = new Headers();
      for (const line of xhr
        .getAllResponseHeaders()
        .trim()
        .split(/[\r\n]+/)) {
        const index = line.indexOf(':');
        if (index > 0) headers.append(line.slice(0, index).trim(), line.slice(index + 1).trim());
      }
      resolve(new Response(xhr.responseText, { status: xhr.status, headers }));
    };
    xhr.onerror = () => {
      cleanup();
      reject(new TypeError('Network request failed'));
    };
    xhr.onabort = () => {
      cleanup();
      reject(new Error('Upload aborted'));
    };
    signal.addEventListener('abort', abort, { once: true });
    upload.onProgress(0);
    if (signal.aborted) {
      cleanup();
      reject(new Error('Upload aborted'));
      return;
    }
    xhr.send(upload.body);
  });
}
