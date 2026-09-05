import { useState } from "react";
import { serviceAction, type ServiceState } from "../api/client";
import { Button } from "./ui";

const LABELS: Record<string, string> = {
  "clamav-daemon": "Scanner daemon (clamd)",
  "clamav-freshclam": "Signature updater (freshclam)",
  "clamav-clamonacc": "On-access scanner (clamonacc)",
};

function badge(active: string) {
  const map: Record<string, string> = {
    active: "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200",
    inactive: "bg-zinc-200 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-300",
    failed: "bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-200",
    activating: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-200",
  };
  return map[active] ?? map.inactive;
}

export default function ServiceCard({ svc, onChange }: { svc: ServiceState; onChange: () => void }) {
  const [busy, setBusy] = useState<string>();
  const [err, setErr] = useState<string>();

  const act = async (action: string) => {
    setBusy(action);
    setErr(undefined);
    try {
      await serviceAction(svc.unit, action);
      onChange();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(undefined);
    }
  };

  return (
    <div className="rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900">
      <div className="flex items-start justify-between gap-2">
        <div>
          <div className="text-sm font-medium">{LABELS[svc.unit] ?? svc.unit}</div>
          <div className="mt-0.5 font-mono text-xs text-zinc-500">{svc.unit}</div>
        </div>
        <span className={`rounded px-1.5 py-0.5 text-xs font-medium ${badge(svc.active)}`}>
          {svc.installed ? svc.active : "not installed"}
        </span>
      </div>

      {svc.installed && (
        <>
          <div className="mt-1 text-xs text-zinc-500">
            {svc.sub} · {svc.enabled}
          </div>
          <div className="mt-3 flex flex-wrap gap-2">
            {svc.active === "active" ? (
              <Button variant="secondary" disabled={!!busy} onClick={() => act("restart")}>
                {busy === "restart" ? "…" : "Restart"}
              </Button>
            ) : (
              <Button variant="secondary" disabled={!!busy} onClick={() => act("start")}>
                {busy === "start" ? "…" : "Start"}
              </Button>
            )}
            {svc.active === "active" && (
              <Button variant="secondary" disabled={!!busy} onClick={() => act("stop")}>
                {busy === "stop" ? "…" : "Stop"}
              </Button>
            )}
            {svc.enabled === "enabled" ? (
              <Button variant="secondary" disabled={!!busy} onClick={() => act("disable")}>
                {busy === "disable" ? "…" : "Disable"}
              </Button>
            ) : (
              <Button variant="secondary" disabled={!!busy} onClick={() => act("enable")}>
                {busy === "enable" ? "…" : "Enable at boot"}
              </Button>
            )}
          </div>
        </>
      )}
      {err && <div className="mt-2 text-xs text-red-600">{err}</div>}
    </div>
  );
}
