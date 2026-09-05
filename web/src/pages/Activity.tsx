import { getActivity } from "../api/client";
import { useAsync } from "../lib/useAsync";
import { useEvents } from "../lib/useEvents";
import { Alert, Card, Spinner } from "../components/ui";

const SEV: Record<string, string> = {
  info: "text-zinc-500",
  warning: "text-amber-600",
  critical: "text-red-600 font-semibold",
};

export default function Activity() {
  const { data, error, loading, reload } = useAsync(() => getActivity(), []);
  useEvents(() => reload(), ["job", "event", "quarantine", "onaccess"]);

  if (loading && !data) return <Spinner />;
  if (error) return <Alert>{error.message}</Alert>;

  const jobs = data?.jobs ?? [];
  const events = data?.events ?? [];

  return (
    <>
      <h1 className="text-lg font-semibold">Activity</h1>

      <Card title="Events">
        {events.length === 0 ? (
          <p className="text-sm text-zinc-500">No events yet.</p>
        ) : (
          <ul className="space-y-1 text-sm">
            {events.map((e) => (
              <li key={e.id} className="flex gap-3">
                <span className="w-40 shrink-0 text-xs text-zinc-500">
                  {new Date(e.ts + "Z").toLocaleString()}
                </span>
                <span className={SEV[e.severity] ?? ""}>{e.message}</span>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Card title="Jobs">
        {jobs.length === 0 ? (
          <p className="text-sm text-zinc-500">No jobs yet.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="text-left text-xs text-zinc-500">
                <tr>
                  <th className="py-1 pr-4">Created</th>
                  <th className="py-1 pr-4">Kind</th>
                  <th className="py-1 pr-4">Status</th>
                  <th className="py-1">Error</th>
                </tr>
              </thead>
              <tbody>
                {jobs.map((j) => (
                  <tr key={j.id} className="border-t border-zinc-200 dark:border-zinc-800">
                    <td className="py-1.5 pr-4 text-xs text-zinc-500">
                      {new Date(j.created_at + "Z").toLocaleString()}
                    </td>
                    <td className="py-1.5 pr-4 font-mono text-xs">{j.kind}</td>
                    <td className="py-1.5 pr-4">{j.status}</td>
                    <td className="py-1.5 text-xs text-red-600">{j.error}</td>
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
