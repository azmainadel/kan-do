import type { Board, Task } from "../types";

/** Task ids per column, in board order. Tasks whose status is no longer a column are left out. */
export function columnsOf(tasks: Task[], statuses: string[]): Record<string, number[]> {
  const cols: Record<string, number[]> = Object.fromEntries(statuses.map((s) => [s, []]));
  for (const t of tasks) cols[t.status]?.push(t.id);
  return cols;
}

/**
 * The board after moving task id to status, just above task before, or to the bottom
 * when before is null: what the server will answer, so the card moves at once. The
 * position only needs to sort right; the next reload brings the server's own.
 */
export function applyMove(board: Board, id: number, status: string, before: number | null): Board {
  const moved = board.tasks.find((t) => t.id === id);
  if (!moved) return board;
  const column = board.tasks.filter((t) => t.status === status && t.id !== id);
  const at = before === null ? -1 : column.findIndex((t) => t.id === before);
  let position: number;
  if (at === -1) {
    const last = column.at(-1);
    position = last ? Math.floor(last.position) + 1 : 1;
  } else {
    const next = column[at]!.position;
    const prev = column[at - 1]?.position;
    position = prev === undefined ? next - 1 : (prev + next) / 2;
  }
  const order = (t: Task) => board.statuses.indexOf(t.status);
  const tasks = board.tasks
    .map((t) => (t.id === id ? { ...t, status, position } : t))
    .sort((a, b) => order(a) - order(b) || a.position - b.position);
  return { ...board, tasks };
}

/** Where a drag ended, as the move to send: the column, and the task now just below, if any. */
export function dropTarget(
  cols: Record<string, number[]>,
  id: number,
): { status: string; before: number | null } | null {
  for (const [status, ids] of Object.entries(cols)) {
    const i = ids.indexOf(id);
    if (i !== -1) return { status, before: ids[i + 1] ?? null };
  }
  return null;
}
