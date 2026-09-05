import { useMemo, useState } from "react";
import { getConfig, putConfig, type ConfEntry, type ConfView } from "../api/client";
import { useAsync } from "../lib/useAsync";
import { Alert, Button, Card, Spinner } from "./ui";

type Draft = Record<string, string[]>;

function draftFromView(v: ConfView): Draft {
  const d: Draft = {};
  for (const e of v.entries) d[e.name] = [...e.values];
  return d;
}

export default function ConfEditor({ which }: { which: "clamd" | "freshclam" }) {
  const { data, error, loading, reload } = useAsync(() => getConfig(which), [which]);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [restart, setRestart] = useState(false); // opt-in: "offer to restart"
  const [saving, setSaving] = useState(false);
  const [saveErr, setSaveErr] = useState<string>();
  const [saved, setSaved] = useState<string>();

  const view = data;
  const current = useMemo(() => (view ? draftFromView(view) : {}), [view]);
  const model = draft ?? current;

  if (loading && !view) return <Spinner />;
  if (error) return <Alert>{error.message}</Alert>;
  if (!view) return null;

  const set = (name: string, values: string[]) => setDraft({ ...model, [name]: values });

  const dirty = JSON.stringify(model) !== JSON.stringify(current);

  const save = async () => {
    setSaving(true);
    setSaveErr(undefined);
    setSaved(undefined);
    try {
      // Only send keys that changed.
      const updates: Draft = {};
      for (const k of Object.keys(model)) {
        if (JSON.stringify(model[k]) !== JSON.stringify(current[k])) updates[k] = model[k];
      }
      const res = await putConfig(which, updates, restart);
      setDraft(null);
      reload();
      setSaved(res.restarted ? `Saved and restarted ${view.unit}.` : "Saved.");
    } catch (e) {
      setSaveErr((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card
      title={<span className="font-mono">{view.path}</span>}
      actions={
        <div className="flex items-center gap-3 text-xs">
          <label className="flex items-center gap-1">
            <input type="checkbox" checked={restart} onChange={(e) => setRestart(e.target.checked)} />
            restart {view.unit}
          </label>
          <Button disabled={!dirty || saving} onClick={save}>
            {saving ? "Saving…" : "Save"}
          </Button>
        </div>
      }
    >
      {saveErr && <Alert>{saveErr}</Alert>}
      {saved && <Alert kind="success">{saved}</Alert>}
      <div className="space-y-4">
        {view.entries.map((e) => (
          <ConfField key={e.name} entry={e} values={model[e.name] ?? []} onChange={(v) => set(e.name, v)} />
        ))}
      </div>
    </Card>
  );
}

function ConfField({
  entry,
  values,
  onChange,
}: {
  entry: ConfEntry;
  values: string[];
  onChange: (v: string[]) => void;
}) {
  const label = (
    <div>
      <span className="text-sm font-medium">{entry.name}</span>
      {entry.help && <span className="ml-2 text-xs text-zinc-500">{entry.help}</span>}
    </div>
  );

  if (entry.kind === "bool") {
    const on = values.length > 0 && ["yes", "true", "1", "on"].includes(values[0].toLowerCase());
    return (
      <label className="flex items-center gap-2">
        <input type="checkbox" checked={on} onChange={(e) => onChange([e.target.checked ? "yes" : "no"])} />
        {label}
      </label>
    );
  }

  if (entry.repeatable) {
    return (
      <div className="space-y-1">
        {label}
        <textarea
          rows={Math.max(2, values.length + 1)}
          className="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 font-mono text-xs dark:border-zinc-700 dark:bg-zinc-900"
          value={values.join("\n")}
          onChange={(e) => onChange(e.target.value.split("\n").map((x) => x.trim()).filter(Boolean))}
        />
      </div>
    );
  }

  return (
    <div className="space-y-1">
      {label}
      <input
        className="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
        value={values[0] ?? ""}
        onChange={(e) => onChange(e.target.value ? [e.target.value] : [])}
      />
    </div>
  );
}
