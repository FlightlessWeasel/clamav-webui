import { useState } from "react";
import { getSignatures, signaturesUpdate } from "../api/client";
import { useAsync } from "../lib/useAsync";
import { useEvents } from "../lib/useEvents";
import { fromUnix, humanCount, humanDuration } from "../lib/format";
import { Alert, Button, Card, Spinner } from "../components/ui";
import ServiceCard from "../components/ServiceCard";
import JobConsole from "../components/JobConsole";

const DAY = 86400;

function freshness(age: number): { kind: "success" | "info" | "error"; text: string } {
  if (age < 0) return { kind: "error", text: "No signature databases found" };
  if (age <= DAY) return { kind: "success", text: `Signatures updated ${humanDuration(age)} ago` };
  if (age <= 7 * DAY) return { kind: "info", text: `Signatures are ${humanDuration(age)} old` };
  return { kind: "error", text: `Signatures are ${humanDuration(age)} old — update now` };
}

export default function Signatures() {
  const { data, error, loading, reload } = useAsync(() => getSignatures(), []);
  const [jobId, setJobId] = useState<number>();
  const [starting, setStarting] = useState(false);
  const [startErr, setStartErr] = useState<string>();
  useEvents(() => reload(), ["service"]);

  const update = async () => {
    setStarting(true);
    setStartErr(undefined);
    try {
      const { job_id } = await signaturesUpdate();
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

  const f = freshness(data.age_seconds);

  return (
    <>
      <h1 className="text-lg font-semibold">Signatures</h1>

      <Alert kind={f.kind}>{f.text}</Alert>

      <Card
        title="Databases"
        actions={
          <Button disabled={starting || !!jobId || !data.has_freshclam} onClick={update}>
            {starting ? "Starting…" : "Update now"}
          </Button>
        }
      >
        {startErr && <Alert>{startErr}</Alert>}
        {!data.has_freshclam && <Alert kind="info">freshclam is not installed.</Alert>}
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="text-left text-xs text-zinc-500">
              <tr>
                <th className="py-1 pr-4">Database</th>
                <th className="py-1 pr-4">File</th>
                <th className="py-1 pr-4">Version</th>
                <th className="py-1 pr-4">Signatures</th>
                <th className="py-1 pr-4">Updated</th>
              </tr>
            </thead>
            <tbody>
              {data.databases.map((db) => (
                <tr key={db.name} className="border-t border-zinc-200 dark:border-zinc-800">
                  <td className="py-1.5 pr-4 font-medium">{db.name}</td>
                  <td className="py-1.5 pr-4 font-mono text-xs">{db.present ? db.file : "—"}</td>
                  <td className="py-1.5 pr-4">{db.version || "—"}</td>
                  <td className="py-1.5 pr-4">{db.sigs ? humanCount(db.sigs) : "—"}</td>
                  <td className="py-1.5 pr-4 text-zinc-500">{db.present ? fromUnix(db.mtime_unix) : "absent"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="mt-2 text-xs text-zinc-500">
          {humanCount(data.total_sigs)} signatures total · {data.db_dir}
        </p>
      </Card>

      {jobId && (
        <Card title="Update progress">
          <JobConsole jobId={jobId} onDone={() => reload()} />
        </Card>
      )}

      <Card title="Automatic updates">
        <p className="mb-3 text-sm text-zinc-500">
          The <code>clamav-freshclam</code> service checks for new signatures{" "}
          {data.checks ? <>{data.checks}× per day</> : "on its configured schedule"}. Enable it to keep
          signatures current automatically.
        </p>
        <div className="max-w-md">
          <ServiceCard svc={data.freshclam_service} onChange={reload} />
        </div>
      </Card>
    </>
  );
}
