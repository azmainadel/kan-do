import type { Task } from "../types";
import { taskKey } from "./text";

export type Owner = "all" | "agent" | "human";

/** What the filter bar narrows the board to. They live in the address, so a filtered view can be shared. */
export interface Filters {
  /** Words that must all appear in the key, title, description or tags. */
  q: string;
  /** Tags a card must all have. */
  tags: string[];
  owner: Owner;
  /** Projects to show on All projects; empty shows every one. */
  projects: string[];
}

export const NO_FILTERS: Filters = { q: "", tags: [], owner: "all", projects: [] };

export const isFiltering = (f: Filters) =>
  !!f.q.trim() || f.tags.length > 0 || f.owner !== "all" || f.projects.length > 0;

export function matches(t: Task, f: Filters): boolean {
  if (f.owner === "human" && !t.human) return false;
  if (f.owner === "agent" && t.human) return false;
  if (f.projects.length && !f.projects.includes(t.project)) return false;
  if (f.tags.some((tag) => !t.tags.includes(tag))) return false;
  const words = f.q.toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return true;
  const text = `${taskKey(t)} ${t.title} ${t.description} ${t.tags.join(" ")}`.toLowerCase();
  return words.every((w) => text.includes(w));
}

const list = (v: string | null) => (v ? v.split(",").filter(Boolean) : []);

/** Reads filters from the query part of the address: q=…&tags=a,b&owner=human&projects=x,y. */
export function parseFilters(query: string): Filters {
  const p = new URLSearchParams(query);
  const owner = p.get("owner");
  return {
    q: p.get("q") ?? "",
    tags: list(p.get("tags")),
    owner: owner === "agent" || owner === "human" ? owner : "all",
    projects: list(p.get("projects")),
  };
}

/** The query part of the address for filters, without the "?"; empty when nothing is filtered. */
export function filtersQuery(f: Filters): string {
  const p = new URLSearchParams();
  if (f.q) p.set("q", f.q);
  if (f.tags.length) p.set("tags", f.tags.join(","));
  if (f.owner !== "all") p.set("owner", f.owner);
  if (f.projects.length) p.set("projects", f.projects.join(","));
  return p.toString();
}

/** Tags on the given tasks, most used first. */
export function tagCounts(tasks: Task[]): [string, number][] {
  const counts = new Map<string, number>();
  for (const t of tasks) for (const tag of t.tags) counts.set(tag, (counts.get(tag) ?? 0) + 1);
  return [...counts].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
}
