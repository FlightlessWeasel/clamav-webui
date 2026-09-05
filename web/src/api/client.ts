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

export type Install = {
  installed: boolean;
  engine_version: string;
  db_version: string;
  db_date: string;
  has_daemon: boolean;
  has_freshclam: boolean;
  has_clamonacc: boolean;
  apt_installed: string;
  apt_candidate: string;
  upgrade_available: boolean;
};

export type ServiceState = {
  unit: string;
  load: string;
  active: string;
  sub: string;
  enabled: string;
  installed: boolean;
  since_unix: number;
};

export type Dashboard = {
  install: Install;
  services: ServiceState[];
  signatures: Signatures | null;
  quarantine_held: number;
  last_scan: unknown;
};

export type Job = {
  id: number;
  kind: string;
  status: "queued" | "running" | "done" | "error";
  log: string;
  error?: string;
  started_at?: string;
  finished_at?: string;
  created_at: string;
};

export type SignatureDB = {
  name: string;
  file: string;
  present: boolean;
  version: number;
  sigs: number;
  build_time: string;
  mtime_unix: number;
};

export type Signatures = {
  db_dir: string;
  databases: SignatureDB[];
  total_sigs: number;
  newest_unix: number;
  age_seconds: number;
  freshclam_service: ServiceState;
  checks: number;
  has_freshclam: boolean;
  has_sigtool: boolean;
};

export const getSignatures = () => api<Signatures>("/signatures");
export const signaturesUpdate = () => api<{ job_id: number }>("/signatures/update", { method: "POST" });

export const getDashboard = () => api<Dashboard>("/dashboard");
export const getServices = () => api<{ services: ServiceState[] }>("/services");
export const serviceAction = (unit: string, action: string) =>
  api<ServiceState>(`/services/${unit}/${action}`, { method: "POST" });
export const serviceLogs = (unit: string, lines = 200) =>
  api<{ unit: string; logs: string }>(`/services/${unit}/logs?lines=${lines}`);
export const clamavInstall = () => api<{ job_id: number }>("/clamav/install", { method: "POST" });
export const clamavUpgrade = () => api<{ job_id: number }>("/clamav/upgrade", { method: "POST" });
export const getJob = (id: number) => api<Job>(`/jobs/${id}`);
