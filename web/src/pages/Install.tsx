import { useState } from "react";
import { clamavInstall, clamavUpgrade, getDashboard } from "../api/client";
import { useAsync } from "../lib/useAsync";
import { Alert, Button, Card, Spinner } from "../components/ui";
import JobConsole from "../components/JobConsole";

export default function Install() {
  const { data, error, loading, reload } = useAsync(() => getDashboard(), []);
  const [jobId, setJobId] = useState<number>();
  const [starting, setStarting] = useState(false);
  const [startErr, setStartErr] = useState<string>();

  const start = async (fn: () => Promise<{ job_id: number }>) => {
    setStarting(true);
    setStartErr(undefined);
    try {
      const { job_id } = await fn();
      setJobId(job_id);
    } catch (e) {
      setStartErr((e as Error).message);
    } finally {
      setStarting(false);
    }
  };

  if (loading && !data) return <Spinner />;
  if (error) return <Alert>{error.message}</Alert>;
  if (!data) return null;

  const { install } = data;

  return (
    <>
      <h1 className="text-lg font-semibold">Install &amp; update ClamAV</h1>

      <Card title="Status">
        <dl className="grid grid-cols-[10rem_1fr] gap-y-1 text-sm">
          <dt className="text-zinc-500">apt package</dt>
          <dd>{install.apt_installed || "not installed"}</dd>
          <dt className="text-zinc-500">apt candidate</dt>
          <dd>{install.apt_candidate || "unknown"}</dd>
          <dt className="text-zinc-500">clamscan</dt>
          <dd>{install.installed ? install.engine_version : "—"}</dd>
          <dt className="text-zinc-500">clamd / freshclam / clamonacc</dt>
          <dd>
            {[
              install.has_daemon && "clamd",
              install.has_freshclam && "freshclam",
              install.has_clamonacc && "clamonacc",
            ]
              .filter(Boolean)
              .join(", ") || "—"}
          </dd>
        </dl>
      </Card>

      <Card title="Action">
        {startErr && <Alert>{startErr}</Alert>}
        <div className="flex flex-wrap gap-2">
          {!install.installed && (
            <Button disabled={starting || !!jobId} onClick={() => start(clamavInstall)}>
              Install ClamAV
            </Button>
          )}
          {install.installed && (
            <Button
              disabled={starting || !!jobId || !install.upgrade_available}
              onClick={() => start(clamavUpgrade)}
            >
              {install.upgrade_available
                ? `Upgrade to ${install.apt_candidate}`
                : "Up to date"}
            </Button>
          )}
          {install.installed && !install.upgrade_available && (
            <Button variant="secondary" disabled={starting || !!jobId} onClick={() => start(clamavUpgrade)}>
              Re-run apt install
            </Button>
          )}
        </div>
        <p className="mt-2 text-xs text-zinc-500">
          Runs <code>apt-get update</code> then installs <code>clamav</code>, <code>clamav-daemon</code> and{" "}
          <code>clamav-freshclam</code>. This can take a few minutes.
        </p>
      </Card>

      {jobId && (
        <Card title="Progress">
          <JobConsole
            jobId={jobId}
            onDone={() => {
              reload();
            }}
          />
        </Card>
      )}
    </>
  );
}
