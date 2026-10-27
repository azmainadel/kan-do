import { useCallback, useEffect, useState } from "react";
import { filtersQuery, parseFilters, type Filters } from "../lib/filter";
import { storage } from "./useStored";

/** What the address holds: #project/task-id?filters, so a task or a filtered view can be linked to. */
export interface Route {
  project: string;
  task: number | null;
  filters: Filters;
}

function read(): Route {
  const [path = "", query = ""] = location.hash.slice(1).split("?");
  let decoded = path;
  try {
    decoded = decodeURIComponent(path);
  } catch {
    // A malformed address keeps its raw text.
  }
  const [slug, task] = decoded.split("/");
  return {
    project: slug || storage.get("kando.project") || "all",
    task: Number(task) || null,
    filters: parseFilters(query),
  };
}

function write(r: Route) {
  const query = filtersQuery(r.filters);
  const hash = "#" + encodeURIComponent(r.project) + (r.task ? "/" + r.task : "") + (query ? "?" + query : "");
  if (location.hash !== hash) history.replaceState(null, "", hash);
}

export function useRoute() {
  const [route, setRoute] = useState(read);

  useEffect(() => {
    const onHash = () => setRoute(read());
    addEventListener("hashchange", onHash);
    return () => removeEventListener("hashchange", onHash);
  }, []);

  useEffect(() => write(route), [route]);

  // Picking one project drops the project filter, which only applies to All projects.
  const setProject = useCallback((project: string) => {
    storage.set("kando.project", project);
    setRoute((r) => ({ ...r, project, filters: project === "all" ? r.filters : { ...r.filters, projects: [] } }));
  }, []);
  const setTask = useCallback((task: number | null) => setRoute((r) => ({ ...r, task })), []);
  const setFilters = useCallback((filters: Filters) => setRoute((r) => ({ ...r, filters })), []);

  return { ...route, setProject, setTask, setFilters };
}
