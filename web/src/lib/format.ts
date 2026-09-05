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
