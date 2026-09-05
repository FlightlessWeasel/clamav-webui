import { useState } from "react";
import {
  createSchedule,
  deleteSchedule,
  getSchedules,
  runSchedule,
  updateSchedule,
  type Schedule,
  type ScheduleInput,
} from "../api/client";
import { useAsync } from "../lib/useAsync";
import { useEvents } from "../lib/useEvents";
import { describeCron } from "../lib/format";
import { Alert, Button, Card, Field, Spinner } from "../components/ui";

const BLANK: ScheduleInput = {
  name: "",
  cron_expr: "0 3 * * *",
  paths: [],
  options: { recursive: true },
  enabled: true,
};

export default function Schedules() {
  const { data, error, loading, reload } = useAsync(() => getSchedules(), []);
  const [editing, setEditing] = useState<{ id?: number; form: ScheduleInput } | null>(null);
  const [saveErr, setSaveErr] = useState<string>();
  const [busy, setBusy] = useState<number | "new">();
  useEvents(() => reload(), ["schedule", "job"]);

  const startNew = () => setEditing({ form: { ...BLANK } });
  const startEdit = (s: Schedule) =>
    setEditing({
      id: s.id,
      form: { name: s.name, cron_expr: s.cron_expr, paths: s.paths, options: s.options ?? {}, enabled: s.enabled },
    });

  const save = async () => {
    if (!editing) return;
    setSaveErr(undefined);
    setBusy(editing.id ?? "new");
    try {
      if (editing.id) await updateSchedule(editing.id, editing.form);
      else await createSchedule(editing.form);
      setEditing(null);
      reload();
    } catch (e) {
      setSaveErr((e as Error).message);
    } finally {
      setBusy(undefined);
    }
  };

  const act = async (fn: () => Promise<unknown>, id: number, confirmMsg?: string) => {
    if (confirmMsg && !window.confirm(confirmMsg)) return;
    setBusy(id);
    try {
      await fn();
      reload();
    } finally {
      setBusy(undefined);
    }
  };

  if (loading && !data) return <Spinner />;
  if (error) return <Alert>{error.message}</Alert>;

  const schedules = data?.schedules ?? [];

  return (
    <>
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">Schedules</h1>
        {!editing && <Button onClick={startNew}>New schedule</Button>}
      </div>

      {editing && (
        <Card title={editing.id ? "Edit schedule" : "New schedule"}>
          {saveErr && <Alert>{saveErr}</Alert>}
          <div className="space-y-3">
            <Field
              label="Name"
              value={editing.form.name}
              onChange={(e) => setEditing({ ...editing, form: { ...editing.form, name: e.target.value } })}
            />
            <Field
              label="Cron expression"
              hint={describeCron(editing.form.cron_expr)}
              value={editing.form.cron_expr}
              onChange={(e) => setEditing({ ...editing, form: { ...editing.form, cron_expr: e.target.value } })}
            />
            <Field
              label="Paths (one per line)"
              value={editing.form.paths.join("\n")}
              onChange={(e) =>
                setEditing({
                  ...editing,
                  form: { ...editing.form, paths: e.target.value.split("\n").map((x) => x.trim()).filter(Boolean) },
                })
              }
            />
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={editing.form.options.recursive ?? false}
                onChange={(e) =>
                  setEditing({
                    ...editing,
                    form: { ...editing.form, options: { ...editing.form.options, recursive: e.target.checked } },
                  })
                }
              />
              Recurse into subdirectories
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={editing.form.enabled}
                onChange={(e) => setEditing({ ...editing, form: { ...editing.form, enabled: e.target.checked } })}
              />
              Enabled
            </label>
            <div className="flex gap-2">
              <Button disabled={busy !== undefined} onClick={save}>
                Save
              </Button>
              <Button variant="secondary" onClick={() => setEditing(null)}>
                Cancel
              </Button>
            </div>
          </div>
        </Card>
      )}

      <Card>
        {schedules.length === 0 ? (
          <p className="text-sm text-zinc-500">No schedules.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="text-left text-xs text-zinc-500">
                <tr>
                  <th className="py-1 pr-4">Name</th>
                  <th className="py-1 pr-4">When</th>
                  <th className="py-1 pr-4">Paths</th>
                  <th className="py-1 pr-4">Last run</th>
                  <th className="py-1 pr-4">Enabled</th>
                  <th className="py-1"></th>
                </tr>
              </thead>
              <tbody>
                {schedules.map((s) => (
                  <tr key={s.id} className="border-t border-zinc-200 dark:border-zinc-800">
                    <td className="py-1.5 pr-4 font-medium">{s.name}</td>
                    <td className="py-1.5 pr-4">
                      <span className="font-mono text-xs">{s.cron_expr}</span>
                      <div className="text-xs text-zinc-500">{describeCron(s.cron_expr)}</div>
                    </td>
                    <td className="max-w-[14rem] truncate py-1.5 pr-4 font-mono text-xs">{s.paths.join(", ")}</td>
                    <td className="py-1.5 pr-4 text-xs text-zinc-500">
                      {s.last_run_at ? new Date(s.last_run_at + "Z").toLocaleString() : "—"}
                    </td>
                    <td className="py-1.5 pr-4">{s.enabled ? "yes" : "no"}</td>
                    <td className="space-x-2 py-1.5 text-right">
                      <Button variant="secondary" disabled={busy === s.id} onClick={() => act(() => runSchedule(s.id), s.id)}>
                        Run now
                      </Button>
                      <Button variant="secondary" onClick={() => startEdit(s)}>
                        Edit
                      </Button>
                      <Button
                        variant="danger"
                        disabled={busy === s.id}
                        onClick={() => act(() => deleteSchedule(s.id), s.id, `Delete schedule "${s.name}"?`)}
                      >
                        Delete
                      </Button>
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
