import { useState } from "react";
import { browse } from "../api/client";
import { useAsync } from "../lib/useAsync";
import { Alert, Button, Spinner } from "./ui";

type Props = {
  selected: string[];
  onChange: (paths: string[]) => void;
};

// PathPicker is an inline server-directory browser with multi-select.
export default function PathPicker({ selected, onChange }: Props) {
  const [cwd, setCwd] = useState<string>("");
  const { data, error, loading } = useAsync(() => browse(cwd || "/"), [cwd]);
  const [manual, setManual] = useState("");

  const toggle = (p: string) => {
    onChange(selected.includes(p) ? selected.filter((x) => x !== p) : [...selected, p]);
  };

  return (
    <div className="rounded-md border border-zinc-200 dark:border-zinc-800">
      <div className="flex items-center gap-2 border-b border-zinc-200 p-2 text-xs dark:border-zinc-800">
        <Button
          variant="secondary"
          disabled={!data?.parent}
          onClick={() => data?.parent && setCwd(data.parent)}
        >
          ↑ Up
        </Button>
        <span className="truncate font-mono">{data?.path ?? cwd ?? "/"}</span>
      </div>

      {error && (
        <div className="p-2">
          <Alert>{error.message}</Alert>
        </div>
      )}
      {loading && !data && (
        <div className="p-3">
          <Spinner />
        </div>
      )}

      <ul className="max-h-64 overflow-auto text-sm">
        {data?.entries.map((e) => (
          <li
            key={e.path}
            className="flex items-center gap-2 border-b border-zinc-100 px-2 py-1 last:border-0 dark:border-zinc-800/60"
          >
            {!e.is_dir && (
              <input
                type="checkbox"
                checked={selected.includes(e.path)}
                onChange={() => toggle(e.path)}
              />
            )}
            {e.is_dir ? (
              <button className="text-left text-sky-600 hover:underline" onClick={() => setCwd(e.path)}>
                📁 {e.name}
              </button>
            ) : (
              <span>📄 {e.name}</span>
            )}
            {e.is_dir && (
              <button
                className="ml-auto text-xs text-zinc-500 hover:text-sky-600"
                onClick={() => toggle(e.path)}
              >
                {selected.includes(e.path) ? "remove" : "add folder"}
              </button>
            )}
          </li>
        ))}
        {data && data.entries.length === 0 && <li className="px-2 py-3 text-zinc-500">empty directory</li>}
      </ul>

      <div className="flex gap-2 border-t border-zinc-200 p-2 dark:border-zinc-800">
        <input
          className="flex-1 rounded border border-zinc-300 bg-white px-2 py-1 text-xs dark:border-zinc-700 dark:bg-zinc-900"
          placeholder="/path/to/scan"
          value={manual}
          onChange={(e) => setManual(e.target.value)}
        />
        <Button
          variant="secondary"
          onClick={() => {
            const p = manual.trim();
            if (p) {
              toggle(p);
              setManual("");
            }
          }}
        >
          Add path
        </Button>
      </div>
    </div>
  );
}
