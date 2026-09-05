import { useParams } from "react-router-dom";
import { cancelScan, getScan } from "../api/client";
import { useAction, useAsync } from "../lib/useAsync";
import { useEvents } from "../lib/useEvents";
import { Alert, Button, Card, Spinner } from "../components/ui";
import ScanStatusBadge from "../components/ScanStatusBadge";
import FindingsTable from "../components/FindingsTable";

export default function ScanDetail() {
  const { id } = useParams();
  const scanId = Number(id);
  const scan = useAsync(() => getScan(scanId), [scanId]);

  useEvents(() => scan.reload(), ["job", "scan-progress"]);

  const cancel = useAction(() => cancelScan(scanId), () => scan.reload());

  if (scan.loading && !scan.data) return <Spinner />;
  if (scan.error) return <Alert>{scan.error.message}</Alert>;
  if (!scan.data) return null;

  const s = scan.data;
  const running = s.status === "running" || s.status === "queued";

  return (
    <>
      <div className="flex items-center gap-3">
        <h1 className="text-lg font-semibold">Scan #{s.id}</h1>
        <ScanStatusBadge status={s.status} />
        {running && (
          <Button variant="danger" disabled={cancel.running} onClick={cancel.run}>
            {cancel.running ? "Canceling…" : "Cancel"}
          </Button>
        )}
      </div>
      {cancel.error && <Alert>{cancel.error.message}</Alert>}
      {s.error && <Alert>{s.error}</Alert>}

      <Card title="Summary">
        <dl className="grid grid-cols-[8rem_1fr] gap-y-1 text-sm">
          <dt className="text-zinc-500">Targets</dt>
          <dd className="font-mono text-xs">{s.paths.join(", ")}</dd>
          <dt className="text-zinc-500">Engine</dt>
          <dd>
            {s.engine || "?"} {s.db_version && <>· sigs {s.db_version}</>}
          </dd>
          <dt className="text-zinc-500">Scanned</dt>
          <dd>{s.scanned}</dd>
          <dt className="text-zinc-500">Infected</dt>
          <dd className={s.infected ? "font-semibold text-red-600" : ""}>{s.infected}</dd>
          <dt className="text-zinc-500">Started</dt>
          <dd>{s.started_at ? new Date(s.started_at + "Z").toLocaleString() : "—"}</dd>
          <dt className="text-zinc-500">Finished</dt>
          <dd>{s.finished_at ? new Date(s.finished_at + "Z").toLocaleString() : "—"}</dd>
        </dl>
      </Card>

      <Card title="Findings">
        <FindingsTable scanId={scanId} running={running} />
      </Card>
    </>
  );
}
