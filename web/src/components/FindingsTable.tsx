import { useState } from "react";
import { getScanFindings, quarantineFile } from "../api/client";
import { useAsync } from "../lib/useAsync";
import { useEvents } from "../lib/useEvents";
import { Alert, Button } from "./ui";

// FindingsTable lists a scan's infected files and offers a per-row quarantine
// action. It refreshes on scan-finding / quarantine SSE events, so it works
// live during a scan and after it.
export default function FindingsTable({ scanId, running }: { scanId: number; running?: boolean }) {
  const { data, reload } = useAsync(() => getScanFindings(scanId), [scanId]);
  const [busy, setBusy] = useState<number>();
  const [err, setErr] = useState<string>();
  useEvents(() => reload(), ["scan-finding", "quarantine"]);

  const rows = data?.findings ?? [];

  const quarantine = async (findingId: number, path: string, signature: string) => {
    setBusy(findingId);
    setErr(undefined);
    try {
      await quarantineFile({ path, signature, scan_id: scanId, finding_id: findingId });
      reload();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(undefined);
    }
  };

  if (rows.length === 0) {
    return <p className="text-sm text-zinc-500">{running ? "Scanning…" : "No infected files."}</p>;
  }

  return (
    <div className="overflow-x-auto">
      {err && <Alert>{err}</Alert>}
      <table className="w-full text-sm">
        <thead className="text-left text-xs text-zinc-500">
          <tr>
            <th className="py-1 pr-4">Path</th>
            <th className="py-1 pr-4">Signature</th>
            <th className="py-1">Action</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((f) => (
            <tr key={f.id} className="border-t border-zinc-200 dark:border-zinc-800">
              <td className="py-1.5 pr-4 font-mono text-xs">{f.path}</td>
              <td className="py-1.5 pr-4">{f.signature}</td>
              <td className="py-1.5">
                {f.action === "none" ? (
                  <Button
                    variant="secondary"
                    disabled={busy === f.id}
                    onClick={() => quarantine(f.id, f.path, f.signature)}
                  >
                    {busy === f.id ? "Moving…" : "Quarantine"}
                  </Button>
                ) : (
                  <span className="text-zinc-500">{f.action}</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
