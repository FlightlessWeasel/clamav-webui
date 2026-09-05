import { useEffect, useRef, useState } from "react";
import { getJob } from "../api/client";
import { useEvents } from "../lib/useEvents";
import { Alert } from "./ui";

type Props = {
  jobId: number;
  onDone?: (status: "done" | "error") => void;
};

// JobConsole shows a worker job's streamed output and final status.
export default function JobConsole({ jobId, onDone }: Props) {
  const [lines, setLines] = useState<string[]>([]);
  const [status, setStatus] = useState<string>("running");
  const [error, setError] = useState<string>();
  const preRef = useRef<HTMLPreElement>(null);
  const doneFired = useRef(false);

  useEffect(() => {
    let cancelled = false;
    getJob(jobId)
      .then((j) => {
        if (cancelled) return;
        if (j.log) setLines(j.log.replace(/\n$/, "").split("\n"));
        setStatus(j.status);
        if (j.error) setError(j.error);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [jobId]);

  useEvents(
    (ev) => {
      const d = ev.data as Record<string, unknown>;
      if (Number(d.job_id) !== jobId) return;
      if (ev.type === "job-log") {
        setLines((ls) => [...ls, String(d.line)]);
      } else if (ev.type === "job") {
        const st = String(d.status);
        setStatus(st);
        if (d.error) setError(String(d.error));
        if ((st === "done" || st === "error") && !doneFired.current) {
          doneFired.current = true;
          onDone?.(st as "done" | "error");
        }
      }
    },
    ["job", "job-log"],
  );

  useEffect(() => {
    const el = preRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [lines]);

  return (
    <div className="space-y-2">
      <div className="text-xs text-zinc-500">
        Job #{jobId} — <span className="font-medium">{status}</span>
      </div>
      {error && <Alert>{error}</Alert>}
      <pre
        ref={preRef}
        className="max-h-80 overflow-auto rounded-md bg-zinc-950 p-3 text-xs leading-relaxed text-zinc-100"
      >
        {lines.length ? lines.join("\n") : "waiting for output…"}
      </pre>
    </div>
  );
}
