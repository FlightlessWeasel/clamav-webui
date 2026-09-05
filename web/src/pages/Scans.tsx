import { Link } from "react-router-dom";
import { getScans } from "../api/client";
import { useAsync } from "../lib/useAsync";
import { useEvents } from "../lib/useEvents";
import { fromUnix } from "../lib/format";
import { Alert, Card, Spinner } from "../components/ui";
import ScanStatusBadge from "../components/ScanStatusBadge";

export default function Scans() {
  const { data, error, loading, reload } = useAsync(() => getScans(), []);
  useEvents(() => reload(), ["job"]);

  if (loading && !data) return <Spinner />;
  if (error) return <Alert>{error.message}</Alert>;

  const scans = data?.scans ?? [];

  return (
    <>
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">Scan history</h1>
        <Link className="text-sm text-sky-600 hover:underline" to="/scan">
          New scan
        </Link>
      </div>

      <Card>
        {scans.length === 0 ? (
          <p className="text-sm text-zinc-500">No scans yet.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="text-left text-xs text-zinc-500">
                <tr>
                  <th className="py-1 pr-4">Started</th>
                  <th className="py-1 pr-4">Status</th>
                  <th className="py-1 pr-4">Targets</th>
                  <th className="py-1 pr-4">Scanned</th>
                  <th className="py-1 pr-4">Infected</th>
                  <th className="py-1"></th>
                </tr>
              </thead>
              <tbody>
                {scans.map((s) => (
                  <tr key={s.id} className="border-t border-zinc-200 dark:border-zinc-800">
                    <td className="py-1.5 pr-4 text-zinc-500">
                      {s.started_at ? new Date(s.started_at + "Z").toLocaleString() : fromUnix(0)}
                    </td>
                    <td className="py-1.5 pr-4">
                      <ScanStatusBadge status={s.status} />
                    </td>
                    <td className="max-w-[16rem] truncate py-1.5 pr-4 font-mono text-xs">
                      {s.paths.join(", ")}
                    </td>
                    <td className="py-1.5 pr-4">{s.scanned}</td>
                    <td className={`py-1.5 pr-4 ${s.infected ? "font-semibold text-red-600" : ""}`}>
                      {s.infected}
                    </td>
                    <td className="py-1.5">
                      <Link className="text-sky-600 hover:underline" to={`/scans/${s.id}`}>
                        view
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </>
  );
}
