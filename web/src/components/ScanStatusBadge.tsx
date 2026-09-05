const STYLES: Record<string, string> = {
  queued: "bg-zinc-200 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-300",
  running: "bg-sky-100 text-sky-800 dark:bg-sky-950 dark:text-sky-200",
  done: "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200",
  error: "bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-200",
  canceled: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-200",
};

export default function ScanStatusBadge({ status }: { status: string }) {
  return (
    <span className={`rounded px-1.5 py-0.5 text-xs font-medium ${STYLES[status] ?? STYLES.queued}`}>
      {status}
    </span>
  );
}
