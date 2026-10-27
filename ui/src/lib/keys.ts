// Moving around the board with the keyboard. Columns hold the ids of the cards shown, in order.

type Columns = Record<string, number[]>;
export type Direction = "up" | "down" | "left" | "right";

function find(cols: Columns, statuses: string[], id: number) {
  for (const [c, status] of statuses.entries()) {
    const row = cols[status]?.indexOf(id) ?? -1;
    if (row !== -1) return { c, row };
  }
  return null;
}

/**
 * The card to focus after an arrow key. With nothing focused, any arrow picks the first
 * card. Left and right skip empty columns and keep about the same row.
 */
export function navigate(cols: Columns, statuses: string[], focused: number | null, dir: Direction): number | null {
  const at = focused === null ? null : find(cols, statuses, focused);
  if (!at) {
    for (const s of statuses) if (cols[s]?.length) return cols[s]![0]!;
    return null;
  }
  const column = cols[statuses[at.c]!]!;
  if (dir === "up") return column[Math.max(at.row - 1, 0)]!;
  if (dir === "down") return column[Math.min(at.row + 1, column.length - 1)]!;
  const step = dir === "left" ? -1 : 1;
  for (let c = at.c + step; c >= 0 && c < statuses.length; c += step) {
    const next = cols[statuses[c]!];
    if (next?.length) return next[Math.min(at.row, next.length - 1)]!;
  }
  return focused;
}

/**
 * Where Shift + an arrow moves a card, as the move to send: up or down one place in its
 * column, or into the next column at about the same row. Null when it can't go further.
 */
export function shift(
  cols: Columns,
  statuses: string[],
  id: number,
  dir: Direction,
): { status: string; before: number | null } | null {
  const at = find(cols, statuses, id);
  if (!at) return null;
  const status = statuses[at.c]!;
  const column = cols[status]!;
  if (dir === "up") return at.row === 0 ? null : { status, before: column[at.row - 1]! };
  if (dir === "down") return at.row === column.length - 1 ? null : { status, before: column[at.row + 2] ?? null };
  const target = statuses[at.c + (dir === "left" ? -1 : 1)];
  if (!target) return null;
  return { status: target, before: cols[target]?.[at.row] ?? null };
}

const ARROWS: Record<string, Direction> = {
  ArrowUp: "up",
  ArrowDown: "down",
  ArrowLeft: "left",
  ArrowRight: "right",
  k: "up",
  j: "down",
  h: "left",
  l: "right",
  K: "up",
  J: "down",
  H: "left",
  L: "right",
};

/** The direction of an arrow or vim key, if it is one. */
export const direction = (key: string): Direction | undefined => ARROWS[key];
