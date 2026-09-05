import { useState } from "react";
import { deleteQuarantine, getQuarantine, restoreQuarantine, type QuarantineItem } from "../api/client";
import { useAsync } from "../lib/useAsync";
import { useEvents } from "../lib/useEvents";
import { Alert, Button, Card, Spinner } from "../components/ui";

const STATUS_STYLE: Record<string, string> = {
  held: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-200",
  restored: "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200",
  deleted: "bg-zinc-200 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400",
};

export default function Quarantine() {
  const { data, error, loading, reload } = useAsync(() => getQuarantine(), []);
  const [busy, setBusy] = useState<number>();
  const [actErr, setActErr] = useState<string>();
  useEvents(() => reload(), ["quarantine"]);

  const act = async (fn: () => Promise<unknown>, id: number, confirmMsg?: string) => {
    if (confirmMsg && !window.confirm(confirmMsg)) return;
    setBusy(id);
    setActErr(undefined);
    try {
      await fn();
      reload();
    } catch (e) {
      setActErr((e as Error).message);
    } finally {
      setBusy(undefined);
    }
  };

  if (loading && !data) return <Spinner />;
  if (error) return <Alert>{error.message}</Alert>;

  const items = data?.items ?? [];

  return (
    <>
      <h1 className="text-lg font-semibold">Quarantine</h1>
      {actErr && <Alert>{actErr}</Alert>}

      <Card>
        {items.length === 0 ? (
          <p className="text-sm text-zinc-500">Nothing quarantined.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="text-left text-xs text-zinc-500">
                <tr>
                  <th className="py-1 pr-4">Original path</th>
                  <th className="py-1 pr-4">Signature</th>
                  <th className="py-1 pr-4">Held</th>
                  <th className="py-1 pr-4">Status</th>
                  <th className="py-1"></th>
                </tr>
              </thead>
              <tbody>
                {items.map((it: QuarantineItem) => (
                  <tr key={it.id} className="border-t border-zinc-200 dark:border-zinc-800">
                    <td className="max-w-[22rem] truncate py-1.5 pr-4 font-mono text-xs">{it.orig_path}</td>
                    <td className="py-1.5 pr-4">{it.signature}</td>
                    <td className="py-1.5 pr-4 text-zinc-500">
                      {new Date(it.created_at + "Z").toLocaleString()}
                    </td>
                    <td className="py-1.5 pr-4">
                      <span className={`rounded px-1.5 py-0.5 text-xs font-medium ${STATUS_STYLE[it.status]}`}>
                        {it.status}
                      </span>
                    </td>
                    <td className="space-x-2 py-1.5 text-right">
                      {it.status === "held" && (
                        <>
                          <Button
                            variant="secondary"
                            disabled={busy === it.id}
                            onClick={() => act(() => restoreQuarantine(it.id), it.id)}
                          >
                            Restore
                          </Button>
                          <Button
                            variant="danger"
                            disabled={busy === it.id}
                            onClick={() =>
                              act(
                                () => deleteQuarantine(it.id),
                                it.id,
                                `Permanently delete the quarantined copy of ${it.orig_path}? This cannot be undone.`,
                              )
                            }
                          >
                            Delete
                          </Button>
                        </>
                      )}
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
