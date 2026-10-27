import type { Project } from "../types";

/** The colors a project can be given; the names match the server's list. */
export const PROJECT_COLORS: Record<string, string> = {
  gray: "#8b918a",
  red: "#d9534a",
  orange: "#e8803a",
  yellow: "#f4ae1d",
  green: "#4f9a5b",
  teal: "#2a9d8f",
  blue: "#4a78c2",
  purple: "#8a63c4",
  pink: "#cf5c9c",
};

const AUTO = Object.keys(PROJECT_COLORS).filter((n) => n !== "gray");

/** The color picked for a project, or until then one chosen from its slug. */
export function projectColorName(slug: string, projects: Project[] = []): string {
  const picked = projects.find((p) => p.slug === slug)?.color;
  if (picked && PROJECT_COLORS[picked]) return picked;
  let h = 0;
  for (const c of slug) h = (h * 31 + c.charCodeAt(0)) % 997;
  return AUTO[h % AUTO.length]!;
}

export const projectColor = (slug: string, projects?: Project[]) => PROJECT_COLORS[projectColorName(slug, projects)]!;
