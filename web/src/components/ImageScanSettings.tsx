import { useEffect, useState } from "react";
import { getImageScan, putImageScan, type ImageScanConfig } from "../api/client";
import { useAsync } from "../lib/useAsync";
import { Alert, Button, Card, Spinner } from "./ui";

// ImageScanSettings toggles loop-mounting disk-image scan targets. clamd will
// not read a multi-GB image past its size limits, so a raw scan of one returns
// "clean" without inspecting anything; mounting exposes the real files.
export default function ImageScanSettings() {
  const { data, error, loading, reload } = useAsync(() => getImageScan(), []);
  const [cfg, setCfg] = useState<ImageScanConfig | null>(null);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ kind: "error" | "success"; text: string }>();

  useEffect(() => {
    if (data && !cfg) setCfg(data);
  }, [data, cfg]);

  if (loading && !data) return <Spinner />;
  if (error) return <Alert>{error.message}</Alert>;
  if (!cfg) return null;

  const save = async () => {
    setBusy(true);
    setMsg(undefined);
    try {
      const cleaned = {
        enabled: cfg.enabled,
        extensions: cfg.extensions.map((e) => e.trim()).filter(Boolean),
      };
      await putImageScan(cleaned);
      reload();
      setMsg({ kind: "success", text: "Disk-image scanning settings saved." });
    } catch (e) {
      setMsg({ kind: "error", text: (e as Error).message });
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card
      title="Disk-image scanning"
      actions={
        <Button disabled={busy} onClick={save}>
          {busy ? "Saving…" : "Save"}
        </Button>
      }
    >
      {msg && <Alert kind={msg.kind}>{msg.text}</Alert>}

      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={cfg.enabled}
          onChange={(e) => setCfg({ ...cfg, enabled: e.target.checked })}
        />
        Loop-mount disk images and scan their contents
      </label>

      <label className="mt-3 block text-sm">
        <span className="text-zinc-500">Extensions (comma-separated)</span>
        <input
          className="mt-1 w-full rounded border border-zinc-300 bg-white px-2 py-1 font-mono text-xs dark:border-zinc-700 dark:bg-zinc-900"
          value={cfg.extensions.join(", ")}
          onChange={(e) => setCfg({ ...cfg, extensions: e.target.value.split(",") })}
          placeholder=".iso, .udf, .img"
        />
      </label>

      <p className="mt-2 text-xs text-zinc-500">
        A scan target with one of these extensions is mounted read-only (<code>nodev,nosuid,noexec</code>) and its
        files are scanned individually; a failed mount falls back to scanning the raw image. Mounting runs the
        kernel filesystem drivers as root — only enable it for images from sources you trust. Large files inside an
        image are still subject to clamd's <code>MaxFileSize</code> / <code>MaxScanSize</code>.
      </p>
    </Card>
  );
}
