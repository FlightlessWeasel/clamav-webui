// Thin fetch wrapper: same-origin, cookie auth, automatic CSRF header on
// mutations, and a typed error carrying the server's message.

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

function readCookie(name: string): string | null {
  const m = document.cookie.match(new RegExp("(?:^|; )" + name + "=([^;]*)"));
  return m ? decodeURIComponent(m[1]) : null;
}

type Options = {
  method?: string;
  body?: unknown;
  signal?: AbortSignal;
};

export async function api<T = unknown>(path: string, opts: Options = {}): Promise<T> {
  const method = opts.method ?? "GET";
  const headers: Record<string, string> = {};
  let body: BodyInit | undefined;

  if (opts.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(opts.body);
  }
  if (method !== "GET" && method !== "HEAD") {
    const token = readCookie("cw_csrf");
    if (token) headers["X-CSRF-Token"] = token;
  }

  const res = await fetch(`/api${path}`, {
    method,
    headers,
    body,
    credentials: "same-origin",
    signal: opts.signal,
  });

  if (res.status === 204) return undefined as T;

  const text = await res.text();
  const data = text ? JSON.parse(text) : undefined;

  if (!res.ok) {
    const msg = (data && typeof data === "object" && "error" in data && String(data.error)) || res.statusText;
    throw new ApiError(res.status, msg);
  }
  return data as T;
}

export type Status = {
  setup_complete: boolean;
  authenticated: boolean;
  version: string;
  tls: boolean;
};

export const getStatus = () => api<Status>("/status");
export const setupAdmin = (password: string) => api<void>("/setup", { method: "POST", body: { password } });
export const login = (password: string) => api<void>("/login", { method: "POST", body: { password } });
export const logout = () => api<void>("/logout", { method: "POST" });
