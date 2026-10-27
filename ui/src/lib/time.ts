/** Stored timestamps look like 2026-10-25T09:30:00Z. */
export const parseTime = (ts: string) => new Date(ts.replace(" ", "T"));

/** "just now", "5m ago", "3h ago", "2d ago", or the date for anything older than a week. */
export function ago(ts: string, now = Date.now()): string {
  const s = (now - parseTime(ts).getTime()) / 1000;
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  if (s < 86400 * 7) return `${Math.floor(s / 86400)}d ago`;
  return parseTime(ts).toLocaleDateString();
}
