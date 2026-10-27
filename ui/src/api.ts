import type { Board, Comment, Image, Project, Task, TaskDetail, TaskPatch, UpdateStatus } from "./types";

/** An error reply from the board, with its JSON body for callers that need more than the message. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly data: Record<string, unknown>,
  ) {
    super(message);
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch("/api" + path, {
    method,
    headers: body === undefined ? {} : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(data.error || res.statusText, res.status, data);
  return data as T;
}

const enc = encodeURIComponent;

export const api = {
  board: (project: string, allDone: boolean) =>
    request<Board>("GET", `/board?project=${enc(project)}${allDone ? "&done=all" : ""}`),
  search: (q: string) => request<Task[]>("GET", `/search?q=${enc(q)}`),

  createProject: (slug: string, name: string, repo: string) =>
    request<Project>("POST", "/projects", { slug, name, repo: repo || null }),
  updateProject: (slug: string, patch: { name?: string; color?: string }) =>
    request<Project>("PATCH", `/projects/${enc(slug)}`, patch),
  deleteProject: (slug: string) => request<Project>("DELETE", `/projects/${enc(slug)}`),
  linkRepo: (slug: string, repo: string) => request<Project>("POST", `/projects/${enc(slug)}/repos`, { repo }),
  unlinkRepo: (slug: string, repo: string) => request<Project>("DELETE", `/projects/${enc(slug)}/repos`, { repo }),

  createTask: (project: string, title: string) => request<Task>("POST", "/tasks", { project, title }),
  task: (id: number) => request<TaskDetail>("GET", `/tasks/${id}`),
  updateTask: (id: number, patch: TaskPatch) => request<Task>("PATCH", `/tasks/${id}`, patch),
  deleteTask: (id: number) => request<Task>("DELETE", `/tasks/${id}`),
  moveTask: (id: number, status: string, before: number | null) =>
    request<Task>("POST", `/tasks/${id}/move`, { status, before }),

  addComment: (id: number, body: string, images: string[]) =>
    request<Comment>("POST", `/tasks/${id}/comments`, { body, images }),
  deleteComment: (id: number) => request<Comment>("DELETE", `/comments/${id}`),

  /** Saves an image file and returns its name, to attach to a task or a comment. */
  async uploadImage(image: Blob): Promise<string> {
    const res = await fetch("/api/images", { method: "POST", headers: { "Content-Type": image.type }, body: image });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new ApiError(data.error || res.statusText, res.status, data);
    return data.file;
  },
  attachImage: (id: number, file: string) => request<Image>("POST", `/tasks/${id}/images`, { file }),
  deleteImage: (id: number) => request<Image>("DELETE", `/images/${id}`),

  updateStatus: (now = false) => request<UpdateStatus>("GET", `/update${now ? "?now=1" : ""}`),
  installUpdate: () => request<{ restarting: boolean }>("POST", "/update"),
  syncRemote: () => request<{ remote: string | null }>("GET", "/sync"),
  setSyncRemote: (url: string) => request<{ remote: string }>("PUT", "/sync", { url }),
  sync: (keep?: "local" | "remote") => request<{ message: string }>("POST", "/sync", { keep }),
};
