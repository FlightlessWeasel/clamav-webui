// Small formatting helpers shared across pages.

export function humanDuration(seconds: number): string {
  if (seconds < 0) return "never";
  if (seconds < 60) return `${Math.round(seconds)}s`;
  const m = Math.round(seconds / 60);
  if (m < 60) return `${m}m`;
  const h = Math.round(m / 60);
  if (h < 48) return `${h}h`;
  const d = Math.round(h / 24);
  return `${d}d`;
}

export function fromUnix(unix: number): string {
  if (!unix) return "—";
  return new Date(unix * 1000).toLocaleString();
}

export function humanCount(n: number): string {
  return n.toLocaleString();
}

const DOW = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

// describeCron gives a rough English gloss of common 5-field / @every specs.
// Falls back to the raw expression when the shape isn't recognised.
export function describeCron(expr: string): string {
  const e = expr.trim();
  const every = e.match(/^@every\s+(.+)$/i);
  if (every) return `every ${every[1]}`;
  if (e === "@hourly") return "every hour";
  if (e === "@daily" || e === "@midnight") return "every day at midnight";
  if (e === "@weekly") return "every Sunday at midnight";
  if (e === "@monthly") return "on the 1st of every month";

  const parts = e.split(/\s+/);
  if (parts.length !== 5) return e;
  const [min, hr, dom, mon, dow] = parts;

  const at =
    /^\d+$/.test(min) && /^\d+$/.test(hr)
      ? `at ${hr.padStart(2, "0")}:${min.padStart(2, "0")}`
      : min.startsWith("*/")
        ? `every ${min.slice(2)} min`
        : null;

  if (dom === "*" && mon === "*" && dow === "*") return at ? `every day, ${at}` : e;
  if (dom === "*" && mon === "*" && /^[0-6]$/.test(dow)) return `every ${DOW[+dow]}${at ? `, ${at}` : ""}`;
  if (/^\d+$/.test(dom) && mon === "*" && dow === "*") return `on day ${dom} of each month${at ? `, ${at}` : ""}`;
  return e;
}
