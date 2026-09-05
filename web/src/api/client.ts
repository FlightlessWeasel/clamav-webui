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

export type ScanOptions = {
  recursive?: boolean;
  follow_symlinks?: boolean;
  max_file_size_mb?: number;
  max_scan_size_mb?: number;
  force_clamscan?: boolean;
};

export type Scan = {
  id: number;
  source: string;
  status: "queued" | "running" | "done" | "error" | "canceled";
  paths: string[];
  engine: string;
  db_version: string;
  scanned: number;
  infected: number;
  error?: string;
  started_at?: string;
  finished_at?: string;
  created_at: string;
};

export type ScanFinding = {
  id: number;
  scan_id: number;
  path: string;
  signature: string;
  action: "none" | "quarantined" | "failed";
  created_at: string;
};

export type Dashboard = {
  install: Install;
  services: ServiceState[];
  signatures: Signatures | null;
  quarantine_held: number;
  last_scan: Scan | null;
};

export type BrowseResult = {
  path: string;
  parent: string;
  root: string;
  entries: { name: string; path: string; is_dir: boolean }[];
};

export const createScan = (paths: string[], options: ScanOptions) =>
  api<{ scan_id: number; job_id: number }>("/scans", { method: "POST", body: { paths, options } });
export const getScans = () => api<{ scans: Scan[] }>("/scans");
export const getScan = (id: number) => api<Scan>(`/scans/${id}`);
export const getScanFindings = (id: number) => api<{ findings: ScanFinding[] }>(`/scans/${id}/findings`);
export const cancelScan = (id: number) => api<{ result: string }>(`/scans/${id}/cancel`, { method: "POST" });
export const browse = (path: string) => api<BrowseResult>(`/browse?path=${encodeURIComponent(path)}`);

export type QuarantineItem = {
  id: number;
  store_name: string;
  orig_path: string;
  signature: string;
  sha256: string;
  scan_id?: number;
  status: "held" | "restored" | "deleted";
  orig_mode: number;
  created_at: string;
  updated_at: string;
};

export type Schedule = {
  id: number;
  name: string;
  cron_expr: string;
  paths: string[];
  options: ScanOptions;
  enabled: boolean;
  last_run_id?: number;
  last_run_at?: string;
  created_at: string;
  updated_at: string;
};

export type ScheduleInput = {
  name: string;
  cron_expr: string;
  paths: string[];
  options: ScanOptions;
  enabled: boolean;
};

export const getSchedules = () => api<{ schedules: Schedule[] }>("/schedules");
export const createSchedule = (body: ScheduleInput) => api<Schedule>("/schedules", { method: "POST", body });
export const updateSchedule = (id: number, body: ScheduleInput) =>
  api<Schedule>(`/schedules/${id}`, { method: "PUT", body });
export const deleteSchedule = (id: number) => api<void>(`/schedules/${id}`, { method: "DELETE" });
export const runSchedule = (id: number) => api<{ result: string }>(`/schedules/${id}/run`, { method: "POST" });

export type ConfEntry = {
  name: string;
  kind: "string" | "bool" | "int" | "path" | "size";
  repeatable: boolean;
  help: string;
  values: string[];
};
export type ConfView = { which: string; path: string; unit: string; entries: ConfEntry[] };

export const getConfig = (which: "clamd" | "freshclam") => api<ConfView>(`/config/${which}`);
export const putConfig = (which: "clamd" | "freshclam", updates: Record<string, string[]>, restart: boolean) =>
  api<{ config: ConfView; restarted: boolean }>(`/config/${which}`, { method: "PUT", body: { updates, restart } });

export type OnAccessStatus = {
  supported: boolean;
  daemon_active: boolean;
  clamonacc: ServiceState;
  enabled: boolean;
  watch_paths: string[];
  exclude_paths: string[];
  exclude_unames: string[];
  prevention: boolean;
};
export type OnAccessConfig = {
  enabled: boolean;
  paths: string[];
  exclude_paths?: string[];
  exclude_unames?: string[];
  prevention: boolean;
};
export const getOnAccess = () => api<OnAccessStatus>("/onaccess");
export const putOnAccess = (cfg: OnAccessConfig) => api<OnAccessStatus>("/onaccess", { method: "PUT", body: cfg });

export type ActivityEvent = { id: number; kind: string; severity: string; message: string; ts: string };
export type Activity = { jobs: Job[]; events: ActivityEvent[] };
export const getActivity = () => api<Activity>("/activity");

export const changePassword = (current: string, next: string) =>
  api<void>("/password", { method: "POST", body: { current, new: next } });

export type NotifyConfig = {
  enabled: boolean;
  events: string[];
  stale_days?: number;
  smtp?: { host: string; port: number; username?: string; password?: string; from: string; to: string; starttls?: boolean };
  webhook?: { url: string };
  ntfy?: { base_url?: string; topic: string; token?: string };
};
export const getNotifications = () => api<NotifyConfig>("/notifications");
export const putNotifications = (cfg: NotifyConfig) => api<NotifyConfig>("/notifications", { method: "PUT", body: cfg });
export const testNotification = () => api<{ result: string }>("/notifications/test", { method: "POST" });

export const getQuarantine = () => api<{ items: QuarantineItem[] }>("/quarantine");
export const quarantineFile = (body: { path: string; signature: string; scan_id?: number; finding_id?: number }) =>
  api<QuarantineItem>("/quarantine", { method: "POST", body });
export const restoreQuarantine = (id: number) => api<QuarantineItem>(`/quarantine/${id}/restore`, { method: "POST" });
export const deleteQuarantine = (id: number) => api<QuarantineItem>(`/quarantine/${id}/delete`, { method: "POST" });

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
