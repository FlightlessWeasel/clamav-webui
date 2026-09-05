import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { createScan, type ScanOptions } from "../api/client";
import { Alert, Button, Card } from "../components/ui";
import PathPicker from "../components/PathPicker";
import JobConsole from "../components/JobConsole";

export default function Scan() {
  const navigate = useNavigate();
  const [paths, setPaths] = useState<string[]>([]);
  const [opts, setOpts] = useState<ScanOptions>({ recursive: true });
  const [jobId, setJobId] = useState<number>();
  const [scanId, setScanId] = useState<number>();
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string>();

  const launch = async () => {
    setBusy(true);
    setErr(undefined);
    try {
      const { job_id, scan_id } = await createScan(paths, opts);
      setJobId(job_id);
      setScanId(scan_id);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const set = (patch: Partial<ScanOptions>) => setOpts((o) => ({ ...o, ...patch }));

  return (
    <>
      <h1 className="text-lg font-semibold">Scan</h1>

      {!jobId && (
        <>
          <Card title="Targets">
            <PathPicker selected={paths} onChange={setPaths} />
            {paths.length > 0 && (
              <ul className="mt-2 space-y-1 text-xs">
                {paths.map((p) => (
                  <li key={p} className="flex items-center gap-2">
                    <span className="font-mono">{p}</span>
                    <button
                      className="text-zinc-500 hover:text-red-600"
                      onClick={() => setPaths(paths.filter((x) => x !== p))}
                    >
                      remove
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </Card>

          <Card title="Options">
            <div className="space-y-2 text-sm">
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={!!opts.recursive}
                  onChange={(e) => set({ recursive: e.target.checked })}
                />
                Recurse into subdirectories
              </label>
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={!!opts.follow_symlinks}
                  onChange={(e) => set({ follow_symlinks: e.target.checked })}
                />
                Follow symlinks
              </label>
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={!!opts.force_clamscan}
                  onChange={(e) => set({ force_clamscan: e.target.checked })}
                />
                Force <code>clamscan</code> (skip the daemon)
              </label>
              <div className="flex gap-4">
                <label className="flex items-center gap-2">
                  Max file MB
                  <input
                    type="number"
                    min={0}
                    className="w-20 rounded border border-zinc-300 bg-white px-2 py-1 dark:border-zinc-700 dark:bg-zinc-900"
                    value={opts.max_file_size_mb ?? ""}
                    onChange={(e) => set({ max_file_size_mb: Number(e.target.value) || 0 })}
                  />
                </label>
                <label className="flex items-center gap-2">
                  Max scan MB
                  <input
                    type="number"
                    min={0}
                    className="w-20 rounded border border-zinc-300 bg-white px-2 py-1 dark:border-zinc-700 dark:bg-zinc-900"
                    value={opts.max_scan_size_mb ?? ""}
                    onChange={(e) => set({ max_scan_size_mb: Number(e.target.value) || 0 })}
                  />
                </label>
              </div>
            </div>
          </Card>

          {err && <Alert>{err}</Alert>}
          <Button disabled={busy || paths.length === 0} onClick={launch}>
            {busy ? "Starting…" : `Scan ${paths.length || "…"} path${paths.length === 1 ? "" : "s"}`}
          </Button>
        </>
      )}

      {jobId && scanId && (
        <Card
          title="Running"
          actions={
            <Button variant="secondary" onClick={() => navigate(`/scans/${scanId}`)}>
              Open scan detail
            </Button>
          }
        >
          <JobConsole jobId={jobId} onDone={() => navigate(`/scans/${scanId}`)} />
        </Card>
      )}
    </>
  );
}
