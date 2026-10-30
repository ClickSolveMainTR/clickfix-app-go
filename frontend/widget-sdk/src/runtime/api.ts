import type { Envelope } from "./types";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

/** Anything the widget calls the visitor API through (FakeApi in preview). */
export interface Transport {
  token: string;
  call<T>(method: string, path: string, body?: unknown): Promise<T>;
}

export class Api implements Transport {
  token = "";
  constructor(private base: string) {}

  async call<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = { Accept: "application/json" };
    if (this.token) headers.Authorization = `Bearer ${this.token}`;
    let payload: BodyInit | undefined;
    if (body instanceof FormData) payload = body;
    else if (body !== undefined) {
      headers["Content-Type"] = "application/json";
      payload = JSON.stringify(body);
    }
    const res = await fetch(this.base + path, {
      method,
      headers,
      body: payload,
      credentials: "omit",
      mode: "cors",
    });
    let env: Envelope<T> | null = null;
    try {
      env = (await res.json()) as Envelope<T>;
    } catch {
      env = null;
    }
    if (!res.ok || !env || !env.success) {
      throw new ApiError(
        res.status,
        env?.error?.code ?? "HTTP_" + res.status,
        env?.error?.message ?? res.statusText,
      );
    }
    return env.data;
  }
}

/** A failed fetch (no response at all): the visitor is offline. */
export function isNetworkError(err: unknown): boolean {
  return err instanceof TypeError;
}
