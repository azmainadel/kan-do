import { useCallback, useEffect, useState } from "react";
import { storage } from "./useStored";

/** What the address holds: #project or #project/task-id, so a task can be linked to. */
export interface Route {
  project: string;
  task: number | null;
}

function read(): Route {
  let hash = location.hash.slice(1);
  try {
    hash = decodeURIComponent(hash);
  } catch {
    // A malformed address keeps its raw text.
  }
  const [slug, task] = hash.split("?")[0]!.split("/");
  return {
    project: slug || storage.get("kan-do.project") || "all",
    task: Number(task) || null,
  };
}

function write(r: Route) {
  const hash = "#" + r.project + (r.task ? "/" + r.task : "");
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

  const setProject = useCallback((project: string) => {
    storage.set("kan-do.project", project);
    setRoute((r) => ({ ...r, project }));
  }, []);
  const setTask = useCallback((task: number | null) => setRoute((r) => ({ ...r, task })), []);

  return { ...route, setProject, setTask };
}
