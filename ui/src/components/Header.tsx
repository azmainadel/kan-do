import type { Project } from "../types";
import { plural } from "../lib/text";

/** The board's title: the project with its open tasks and repos, or every project. */
export function Header({ project, projects }: { project: Project | null; projects: Project[] }) {
  const openTasks = (n: number) => `${plural(n, "open task")}`;
  return (
    <header className="flex min-h-16 flex-wrap items-baseline gap-3 px-6 pt-4 pb-3.5 max-[700px]:p-3">
      {project ? (
        <>
          <h1 className="font-display text-xl font-semibold tracking-tight">{project.name}</h1>
          <span className="text-muted">{openTasks(project.open)}</span>
          {project.repos.map((r) => (
            <a
              key={r}
              href={`https://github.com/${r}`}
              target="_blank"
              rel="noopener"
              title={`Open ${r} on GitHub`}
              className="inline-flex items-center gap-1.5 self-center rounded-full bg-hover py-0.5 pr-2.5 pl-2 text-[12.5px] text-muted hover:bg-active hover:text-text"
            >
              <GitHub />
              {r}
            </a>
          ))}
        </>
      ) : (
        <>
          <h1 className="font-display text-xl font-semibold tracking-tight">All projects</h1>
          <span className="text-muted">
            {openTasks(projects.reduce((n, p) => n + p.open, 0))} in {plural(projects.length, "project")}
          </span>
        </>
      )}
    </header>
  );
}

// lucide-react dropped brand icons; this is the GitHub mark as a line icon.
function GitHub() {
  return (
    <svg
      viewBox="0 0 24 24"
      className="size-3.5"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M15 22v-4a4.8 4.8 0 0 0-1-3.5c3 0 6-2 6-5.5.1-1.3-.3-2.5-1-3.5.3-1.2.3-2.4 0-3.5 0 0-1 0-3 1.5a12 12 0 0 0-8 0C6 2 5 2 5 2c-.3 1.2-.3 2.4 0 3.5A5.4 5.4 0 0 0 4 9c0 3.5 3 5.5 6 5.5-.4.5-.7 1-.8 1.7-.2.6-.3 1.2-.2 1.8v4" />
      <path d="M9 18c-4.5 2-5-2-7-2" />
    </svg>
  );
}
