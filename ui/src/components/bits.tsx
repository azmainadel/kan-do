// Small pieces used across the board.
import { useNow } from "../hooks/useNow";
import { ago, parseTime } from "../lib/time";
import { toast } from "sonner";

/** The kan-do mark. The bar follows the theme so it stays visible in dark mode. */
export function Logo({ className }: { className?: string }) {
  return (
    <svg viewBox="124 124 1006 1006" className={className} aria-hidden="true">
      <path
        fill="var(--logo-bar)"
        d="M245 252A68 68 0 0 1 313 184L441 184A68 68 0 0 1 509 252L509 1002A68 68 0 0 1 441 1070L313 1070A68 68 0 0 1 245 1002Z"
      />
      <path
        fill="#849f7a"
        d="M538 480A62 62 0 0 1 556 436L789 203A62 62 0 0 1 833 185L947 185A62 62 0 0 1 1009 247L1009 277A62 62 0 0 1 991 321L717 595A62 62 0 0 1 673 613L600 613A62 62 0 0 1 538 551Z"
      />
      <path
        fill="#f4ae1d"
        d="M537 702A62 62 0 0 1 599 640L674 640A62 62 0 0 1 718 658L991 931A62 62 0 0 1 1009 975L1009 1008A62 62 0 0 1 947 1070L822 1070A62 62 0 0 1 778 1052L555 829A62 62 0 0 1 537 785Z"
      />
    </svg>
  );
}

/**
 * A column's status as a small progress circle, as in Linear: empty for the first column,
 * filling up across the board, a check for the last. A shape, so it can't be taken for a
 * project's color dot.
 */
export function StatusIcon({ status, statuses }: { status: string; statuses: string[] }) {
  const i = Math.max(statuses.indexOf(status), 0);
  const f = statuses.length > 1 ? i / (statuses.length - 1) : 0;
  const color = f >= 1 ? "var(--accent)" : f > 0 ? "var(--amber)" : "var(--faint)";
  const r = 3.4;
  const a = f * 2 * Math.PI;
  const x = (7 + r * Math.sin(a)).toFixed(2);
  const y = (7 - r * Math.cos(a)).toFixed(2);
  return (
    <svg viewBox="0 0 14 14" className="size-3.5 shrink-0" aria-hidden="true">
      <circle cx="7" cy="7" r="6" fill="none" stroke={color} strokeWidth="1.5" />
      {f >= 1 ? (
        <>
          <circle cx="7" cy="7" r="6" fill={color} />
          <path
            d="m4.4 7.2 1.8 1.8 3.4-3.6"
            fill="none"
            stroke="var(--bg)"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </>
      ) : f > 0 ? (
        <path d={`M7 7L7 ${7 - r}A${r} ${r} 0 ${f > 0.5 ? 1 : 0} 1 ${x} ${y}Z`} fill={color} />
      ) : null}
    </svg>
  );
}

/** A relative time that stays current, with the full time on hover. */
export function Time({ ts }: { ts: string }) {
  const now = useNow();
  return (
    <time dateTime={ts} title={parseTime(ts).toLocaleString()}>
      {ago(ts, now)}
    </time>
  );
}

export function Dot({ color, className = "size-2" }: { color: string; className?: string }) {
  return <span className={`inline-block shrink-0 rounded-full ${className}`} style={{ background: color }} />;
}

/** Copies a task's key, like api#12, to paste to an agent. Plain HTTP from another device has no clipboard API. */
export async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    const area = document.createElement("textarea");
    area.value = text;
    area.style.cssText = "position:fixed;opacity:0";
    document.body.append(area);
    area.select();
    document.execCommand("copy");
    area.remove();
  }
  toast(`Copied ${text}`);
}

/** Shows an error from a failed action. */
export const fail = (e: unknown) => toast.error(e instanceof Error ? e.message : String(e));
