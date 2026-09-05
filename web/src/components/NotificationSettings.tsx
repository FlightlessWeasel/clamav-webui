import { useEffect, useState } from "react";
import { getNotifications, putNotifications, testNotification, type NotifyConfig } from "../api/client";
import { useAsync } from "../lib/useAsync";
import { Alert, Button, Card, Field, Spinner } from "./ui";

const EVENT_KINDS = ["scan-detection", "scan-failure", "onaccess-detection", "signatures-stale"];

export default function NotificationSettings() {
  const { data, error, loading, reload } = useAsync(() => getNotifications(), []);
  const [cfg, setCfg] = useState<NotifyConfig | null>(null);
  const [busy, setBusy] = useState<"save" | "test">();
  const [msg, setMsg] = useState<{ kind: "error" | "success"; text: string }>();

  useEffect(() => {
    if (data && !cfg) setCfg(data);
  }, [data, cfg]);

  if (loading && !data) return <Spinner />;
  if (error) return <Alert>{error.message}</Alert>;
  if (!cfg) return null;

  const patch = (p: Partial<NotifyConfig>) => setCfg({ ...cfg, ...p });

  const save = async () => {
    setBusy("save");
    setMsg(undefined);
    try {
      await putNotifications(cfg);
      reload();
      setMsg({ kind: "success", text: "Notification settings saved." });
    } catch (e) {
      setMsg({ kind: "error", text: (e as Error).message });
    } finally {
      setBusy(undefined);
    }
  };

  const test = async () => {
    setBusy("test");
    setMsg(undefined);
    try {
      await testNotification();
      setMsg({ kind: "success", text: "Test notification sent." });
    } catch (e) {
      setMsg({ kind: "error", text: (e as Error).message });
    } finally {
      setBusy(undefined);
    }
  };

  const toggleEvent = (k: string) =>
    patch({ events: cfg.events.includes(k) ? cfg.events.filter((x) => x !== k) : [...cfg.events, k] });

  return (
    <Card
      title="Notifications"
      actions={
        <div className="flex gap-2">
          <Button variant="secondary" disabled={!!busy} onClick={test}>
            {busy === "test" ? "Sending…" : "Send test"}
          </Button>
          <Button disabled={!!busy} onClick={save}>
            {busy === "save" ? "Saving…" : "Save"}
          </Button>
        </div>
      }
    >
      {msg && <Alert kind={msg.kind}>{msg.text}</Alert>}

      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={cfg.enabled} onChange={(e) => patch({ enabled: e.target.checked })} />
        Enable notifications
      </label>

      <div className="mt-3">
        <div className="text-sm font-medium">Send for</div>
        <div className="mt-1 flex flex-wrap gap-3 text-xs">
          {EVENT_KINDS.map((k) => (
            <label key={k} className="flex items-center gap-1">
              <input type="checkbox" checked={cfg.events.includes(k)} onChange={() => toggleEvent(k)} />
              {k}
            </label>
          ))}
        </div>
        <p className="mt-1 text-xs text-zinc-500">None checked = send for every event.</p>
      </div>

      <div className="mt-3">
        <Field
          label="Signatures stale after (days)"
          type="number"
          value={String(cfg.stale_days ?? 7)}
          onChange={(e) => patch({ stale_days: Number(e.target.value) || 7 })}
        />
      </div>

      <fieldset className="mt-4 space-y-2 border-t border-zinc-200 pt-3 dark:border-zinc-800">
        <legend className="text-sm font-medium">ntfy</legend>
        <Field
          label="Topic"
          value={cfg.ntfy?.topic ?? ""}
          onChange={(e) => patch({ ntfy: { ...cfg.ntfy, topic: e.target.value } })}
        />
        <Field
          label="Server (blank = ntfy.sh)"
          value={cfg.ntfy?.base_url ?? ""}
          onChange={(e) => patch({ ntfy: { ...(cfg.ntfy ?? { topic: "" }), base_url: e.target.value } })}
        />
        <Field
          label="Access token (optional)"
          type="password"
          value={cfg.ntfy?.token ?? ""}
          onChange={(e) => patch({ ntfy: { ...(cfg.ntfy ?? { topic: "" }), token: e.target.value } })}
        />
      </fieldset>

      <fieldset className="mt-4 space-y-2 border-t border-zinc-200 pt-3 dark:border-zinc-800">
        <legend className="text-sm font-medium">Webhook</legend>
        <Field
          label="POST URL"
          value={cfg.webhook?.url ?? ""}
          onChange={(e) => patch({ webhook: { url: e.target.value } })}
        />
      </fieldset>

      <fieldset className="mt-4 space-y-2 border-t border-zinc-200 pt-3 dark:border-zinc-800">
        <legend className="text-sm font-medium">Email (SMTP)</legend>
        <div className="grid grid-cols-2 gap-2">
          <Field
            label="Host"
            value={cfg.smtp?.host ?? ""}
            onChange={(e) => patch({ smtp: { ...smtpBase(cfg), host: e.target.value } })}
          />
          <Field
            label="Port"
            type="number"
            value={String(cfg.smtp?.port ?? 587)}
            onChange={(e) => patch({ smtp: { ...smtpBase(cfg), port: Number(e.target.value) || 587 } })}
          />
          <Field
            label="From"
            value={cfg.smtp?.from ?? ""}
            onChange={(e) => patch({ smtp: { ...smtpBase(cfg), from: e.target.value } })}
          />
          <Field
            label="To"
            value={cfg.smtp?.to ?? ""}
            onChange={(e) => patch({ smtp: { ...smtpBase(cfg), to: e.target.value } })}
          />
          <Field
            label="Username"
            value={cfg.smtp?.username ?? ""}
            onChange={(e) => patch({ smtp: { ...smtpBase(cfg), username: e.target.value } })}
          />
          <Field
            label="Password"
            type="password"
            placeholder={cfg.smtp?.password === "********" ? "unchanged" : ""}
            value={cfg.smtp?.password === "********" ? "" : (cfg.smtp?.password ?? "")}
            onChange={(e) => patch({ smtp: { ...smtpBase(cfg), password: e.target.value } })}
          />
        </div>
      </fieldset>
    </Card>
  );
}

function smtpBase(cfg: NotifyConfig) {
  return cfg.smtp ?? { host: "", port: 587, from: "", to: "" };
}
