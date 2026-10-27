import { describe, expect, it } from "vitest";
import { direction, navigate, shift } from "./keys";

describe("keyboard", () => {
  const statuses = ["todo", "doing", "review", "done"];
  const cols = { todo: [1, 2, 3], doing: [], review: [4], done: [5, 6] };

  it("focuses the first card first", () => {
    expect(navigate(cols, statuses, null, "down")).toBe(1);
    expect(navigate({ todo: [], doing: [7] }, ["todo", "doing"], null, "left")).toBe(7);
  });
  it("moves within a column and stops at its ends", () => {
    expect(navigate(cols, statuses, 2, "down")).toBe(3);
    expect(navigate(cols, statuses, 3, "down")).toBe(3);
    expect(navigate(cols, statuses, 1, "up")).toBe(1);
  });
  it("skips empty columns and keeps about the same row", () => {
    expect(navigate(cols, statuses, 3, "right")).toBe(4);
    expect(navigate(cols, statuses, 4, "left")).toBe(1);
    expect(navigate(cols, statuses, 6, "right")).toBe(6);
  });
  it("moves cards with shift", () => {
    expect(shift(cols, statuses, 2, "up")).toEqual({ status: "todo", before: 1 });
    expect(shift(cols, statuses, 1, "up")).toBeNull();
    expect(shift(cols, statuses, 1, "down")).toEqual({ status: "todo", before: 3 });
    expect(shift(cols, statuses, 2, "down")).toEqual({ status: "todo", before: null });
    expect(shift(cols, statuses, 3, "down")).toBeNull();
    expect(shift(cols, statuses, 2, "right")).toEqual({ status: "doing", before: null });
    expect(shift(cols, statuses, 6, "left")).toEqual({ status: "review", before: null });
    expect(shift(cols, statuses, 5, "right")).toBeNull();
  });
  it("knows arrows and vim keys", () => {
    expect(direction("ArrowLeft")).toBe("left");
    expect(direction("j")).toBe("down");
    expect(direction("x")).toBeUndefined();
  });
});
