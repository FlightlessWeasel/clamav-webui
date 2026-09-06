import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { createScan, getImageScan, getScan, type Scan as ScanRow, type ScanOptions } from "../api/client";
import { Alert, Button, Card } from "../components/ui";
import PathPicker from "../components/PathPicker";
import JobConsole from "../components/JobConsole";
import FindingsTable from "../components/FindingsTable";
import { useAsync } from "../lib/useAsync";
import { humanDuration } from "../lib/format";

// elapsed renders the wall-clock scan duration; "under a second" when the
// timestamps are missing or the scan was near-instant.
function elapsed(s: ScanRow): string {
  if (!s.started_at || !s.finished_at) return "under a second";
  const ms = Date.parse(s.finished_at + "Z") - Date.parse(s.started_at + "Z");
  if (!Number.isFinite(ms) || ms < 1000) return "under a second";
  return humanDuration(ms / 1000);
}

// scanSummary is the one-line result banner shown when a scan finishes.
function scanSummary(s: ScanRow): string {
  const files = `${s.scanned} file${s.scanned === 1 ? "" : "s"}`;
  const verdict = s.infected ? `${s.infected} infected` : "no threats found";
  return `Scanned ${files} in ${elapsed(s)} — ${verdict}.`;
}

export default function Scan() {
  const navigate = useNavigate();
  const [paths, setPaths] = useState<string[]>([]);
  const [opts, setOpts] = useState<ScanOptions>({ recursive: true });
  const [jobId, setJobId] = useState<number>();
  const [scanId, setScanId] = useState<number>();
  const [done, setDone] = useState(false);
  const [result, setResult] = useState<ScanRow>();
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string>();

  // Disk-image mount config, so we can flag .iso-style targets before launch.
  const { data: imageScan } = useAsync(() => getImageScan(), []);
  const imageExts = imageScan?.extensions ?? [];
  const looksLikeImage = (p: string) => imageExts.some((e) => p.toLowerCase().endsWith(e.toLowerCase()));

  // When the job finishes, pull the final scan row so we can show a summary —
  // a quick scan can complete before any progress is visible on screen.
  useEffect(() => {
    if (!done || !scanId) return;
    getScan(scanId)
      .then(setResult)
      .catch(() => {});
  }, [done, scanId]);

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
                  <li key={p} className="flex flex-wrap items-center gap-2">
                    <span className="font-mono">{p}</span>
                    <button
                      className="text-zinc-500 hover:text-red-600"
                      onClick={() => setPaths(paths.filter((x) => x !== p))}
                    >
                      remove
                    </button>
                    {looksLikeImage(p) &&
                      (imageScan?.enabled ? (
                        <span className="text-emerald-600 dark:text-emerald-400">
                          disk image — its contents will be scanned (mounted, or extracted)
                        </span>
                      ) : (
                        <span className="text-amber-600 dark:text-amber-500">
                          disk image — a raw scan skips its contents;{" "}
                          <Link to="/settings" className="underline">
                            enable disk-image scanning
                          </Link>
                        </span>
                      ))}
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
        <>
          <Card
            title={!done ? "Running" : result?.status === "error" ? "Failed" : "Completed"}
            actions={
              <Button variant="secondary" onClick={() => navigate(`/scans/${scanId}`)}>
                Open scan detail
              </Button>
            }
          >
            <JobConsole jobId={jobId} onDone={() => setDone(true)} />
          </Card>
          {done && result && result.status !== "error" && (
            <Alert kind={result.infected ? "error" : "success"}>{scanSummary(result)}</Alert>
          )}
          <Card title="Findings">
            <FindingsTable scanId={scanId} running={!done} />
          </Card>
        </>
      )}
    </>
  );
}
