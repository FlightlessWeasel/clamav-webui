import { useEffect, useState } from "react";
import { getOnAccess, putOnAccess, type OnAccessConfig } from "../api/client";
import { useAsync } from "../lib/useAsync";
import { useEvents } from "../lib/useEvents";
import { Alert, Button, Card, Spinner } from "../components/ui";
import ServiceCard from "../components/ServiceCard";

export default function Protection() {
  const { data, error, loading, reload } = useAsync(() => getOnAccess(), []);
  const [form, setForm] = useState<OnAccessConfig | null>(null);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ kind: "error" | "success"; text: string }>();
  useEvents(() => reload(), ["service", "onaccess"]);

  useEffect(() => {
    if (data && !form) {
      setForm({
        enabled: data.enabled,
        paths: data.watch_paths,
        exclude_unames: data.exclude_unames,
        prevention: data.prevention,
      });
    }
  }, [data, form]);

  if (loading && !data) return <Spinner />;
  if (error) return <Alert>{error.message}</Alert>;
  if (!data || !form) return null;

  const apply = async (enabled: boolean) => {
    setBusy(true);
    setMsg(undefined);
    try {
      await putOnAccess({ ...form, enabled });
      setForm({ ...form, enabled });
      reload();
      setMsg({ kind: "success", text: enabled ? "On-access scanning enabled." : "On-access scanning disabled." });
    } catch (e) {
      setMsg({ kind: "error", text: (e as Error).message });
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <h1 className="text-lg font-semibold">Real-time protection</h1>

      {!data.supported && (
        <Alert kind="info">
          The <code>clamonacc</code> on-access scanner is not installed. Install ClamAV first.
        </Alert>
      )}
      {msg && <Alert kind={msg.kind}>{msg.text}</Alert>}

      <Card
        title="On-access scanning"
        actions={
          data.enabled ? (
            <Button variant="danger" disabled={busy} onClick={() => apply(false)}>
              Disable
            </Button>
          ) : (
            <Button disabled={busy || !data.supported || form.paths.length === 0} onClick={() => apply(true)}>
              {busy ? "Applying…" : "Enable"}
            </Button>
          )
        }
      >
        <p className="mb-3 text-sm text-zinc-500">
          Watches the directories below with fanotify and scans files as they are opened. Requires the{" "}
          <code>clamav-daemon</code>; enabling restarts it.
        </p>

        <label className="block space-y-1">
          <span className="text-sm font-medium">Watched directories (one per line)</span>
          <textarea
            rows={Math.max(3, form.paths.length + 1)}
            className="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 font-mono text-xs dark:border-zinc-700 dark:bg-zinc-900"
            value={form.paths.join("\n")}
            onChange={(e) =>
              setForm({ ...form, paths: e.target.value.split("\n").map((x) => x.trim()).filter(Boolean) })
            }
          />
        </label>

        <label className="mt-3 flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={form.prevention}
            onChange={(e) => setForm({ ...form, prevention: e.target.checked })}
          />
          Prevention mode — block access to infected files (needs kernel fanotify permissions)
        </label>

        {data.enabled && (
          <div className="mt-3">
            <Button variant="secondary" disabled={busy} onClick={() => apply(true)}>
              {busy ? "Applying…" : "Apply changes & restart"}
            </Button>
          </div>
        )}
      </Card>

      <Card title="Scanner status">
        <div className="max-w-md">
          <ServiceCard svc={data.clamonacc} onChange={reload} />
        </div>
        <p className="mt-2 text-xs text-zinc-500">
          Real-time detections are auto-quarantined and appear on the Activity page.
        </p>
      </Card>
    </>
  );
}
