import { describe, expect, it } from "vitest";
import { applyMove, columnsOf, dropTarget } from "./board";
import type { Board, Task } from "../types";

const task = (id: number, status: string, position: number): Task => ({
  id,
  project: "api",
  title: `t${id}`,
  description: "",
  tags: [],
  human: false,
  status,
  position,
  created_at: "",
  updated_at: "",
  comments: 0,
});

const board = (tasks: Task[]): Board => ({
  changes: "",
  done_hidden: 0,
  projects: [],
  statuses: ["todo", "doing", "done"],
  tasks,
  version: "",
});

const ids = (b: Board) => columnsOf(b.tasks, b.statuses);

describe("applyMove", () => {
  const b = board([task(1, "todo", 1), task(2, "todo", 2), task(3, "todo", 3), task(4, "doing", 1)]);

  it("moves above a task", () => {
    expect(ids(applyMove(b, 3, "todo", 1)).todo).toEqual([3, 1, 2]);
    expect(ids(applyMove(b, 3, "todo", 2)).todo).toEqual([1, 3, 2]);
  });

  it("moves to the bottom of another column", () => {
    const after = applyMove(b, 1, "doing", null);
    expect(ids(after)).toEqual({ todo: [2, 3], doing: [4, 1], done: [] });
    expect(after.tasks.find((t) => t.id === 1)?.status).toBe("doing");
  });

  it("moves into an empty column", () => {
    expect(ids(applyMove(b, 2, "done", null)).done).toEqual([2]);
  });

  it("treats an anchor in another column as the bottom", () => {
    expect(ids(applyMove(b, 1, "doing", 3)).doing).toEqual([4, 1]);
  });
});

describe("dropTarget", () => {
  it("names the column and the task below", () => {
    const cols = { todo: [1, 2], doing: [3] };
    expect(dropTarget(cols, 1)).toEqual({ status: "todo", before: 2 });
    expect(dropTarget(cols, 3)).toEqual({ status: "doing", before: null });
    expect(dropTarget(cols, 9)).toBeNull();
  });
});

describe("columnsOf", () => {
  it("leaves out statuses that are no longer columns", () => {
    expect(columnsOf([task(1, "todo", 1), task(2, "gone", 1)], ["todo", "done"])).toEqual({ todo: [1], done: [] });
  });
});
