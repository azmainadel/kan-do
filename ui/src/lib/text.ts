import type { Task } from "../types";

/** How tasks are shown everywhere: api#12. */
export const taskKey = (t: Pick<Task, "project" | "id">) => `${t.project}#${t.id}`;

/** A description as one line of plain text for a card: Markdown markers and code blocks dropped, lines joined. */
export function plainText(md: string): string {
  // Code blocks say little in a one-line preview: leave them out with their fences.
  let inCode = false;
  return md
    .split("\n")
    .filter((l) => {
      if (!/^\s*```/.test(l)) return !inCode;
      inCode = !inCode;
      return false;
    })
    .map((l) => l.replace(/^\s*(#{1,6}|>|[-*+]|\d+[.)])\s+/, "").replace(/^\[[ xX]\]\s+/, ""))
    .filter((l) => l.trim())
    .join(" ")
    .replace(/\*\*|__|`/g, "")
    .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1")
    .replace(/\s+/g, " ")
    .trim();
}

/** Tags typed comma-separated, tidied the way the server stores them. */
export function cleanTags(text: string): string[] {
  const tags = text.split(",").map((t) => t.trim().replace(/\s+/g, " ").toLowerCase());
  return [...new Set(tags.filter(Boolean))];
}

export const plural = (n: number, one: string, many = one + "s") => `${n} ${n === 1 ? one : many}`;

/** "you/kando-data" from any URL of a git repo. */
export const repoName = (url: string) =>
  url
    .replace(/\.git$/, "")
    .split(/[/:]/)
    .filter(Boolean)
    .slice(-2)
    .join("/");
