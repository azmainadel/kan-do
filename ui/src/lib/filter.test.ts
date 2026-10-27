import { describe, expect, it } from "vitest";
import { NO_FILTERS, filtersQuery, isFiltering, matches, parseFilters, tagCounts, type Filters } from "./filter";
import type { Task } from "../types";

const task = (over: Partial<Task>): Task => ({
  id: 1,
  project: "api",
  title: "Fix login",
  description: "Safari drops the cookie",
  tags: [],
  human: false,
  status: "todo",
  position: 1,
  created_at: "",
  updated_at: "",
  comments: 0,
  ...over,
});

const f = (over: Partial<Filters>): Filters => ({ ...NO_FILTERS, ...over });

describe("matches", () => {
  const t = task({ id: 12, tags: ["bug", "auth"] });
  it("matches every word in the key, title, description and tags", () => {
    expect(matches(t, f({ q: "LOGIN safari" }))).toBe(true);
    expect(matches(t, f({ q: "api#12" }))).toBe(true);
    expect(matches(t, f({ q: "auth" }))).toBe(true);
    expect(matches(t, f({ q: "login chrome" }))).toBe(false);
  });
  it("needs every selected tag", () => {
    expect(matches(t, f({ tags: ["bug"] }))).toBe(true);
    expect(matches(t, f({ tags: ["bug", "ui"] }))).toBe(false);
  });
  it("filters by owner and project", () => {
    expect(matches(t, f({ owner: "agent" }))).toBe(true);
    expect(matches(t, f({ owner: "human" }))).toBe(false);
    expect(matches(task({ human: true }), f({ owner: "human" }))).toBe(true);
    expect(matches(t, f({ projects: ["web"] }))).toBe(false);
    expect(matches(t, f({ projects: ["web", "api"] }))).toBe(true);
  });
});

describe("filters in the address", () => {
  it("round-trips", () => {
    const filters = f({ q: "login bug", tags: ["a b", "c"], owner: "human", projects: ["x", "y"] });
    expect(parseFilters(filtersQuery(filters))).toEqual(filters);
    expect(filtersQuery(NO_FILTERS)).toBe("");
    expect(isFiltering(parseFilters(""))).toBe(false);
  });
  it("ignores an unknown owner", () => {
    expect(parseFilters("owner=robots").owner).toBe("all");
  });
});

it("counts tags, most used first", () => {
  expect(tagCounts([task({ tags: ["ui", "bug"] }), task({ tags: ["bug"] })])).toEqual([
    ["bug", 2],
    ["ui", 1],
  ]);
});
