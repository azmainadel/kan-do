// Shapes of the board's JSON API (see docs/SPEC.md).

export interface Project {
  id: number;
  slug: string;
  name: string;
  repos: string[];
  color: string | null;
  created_at: string;
  open: number;
  total: number;
}

export interface Task {
  id: number;
  project: string;
  title: string;
  description: string;
  tags: string[];
  human: boolean;
  status: string;
  position: number;
  created_at: string;
  updated_at: string;
  comments: number;
}

export interface Comment {
  id: number;
  task_id: number;
  author: string;
  body: string;
  created_at: string;
}

export interface Image {
  id: number;
  task_id: number;
  comment_id: number | null;
  file: string;
}

export interface Board {
  changes: string;
  done_hidden: number;
  projects: Project[];
  statuses: string[];
  tasks: Task[];
  version: string;
}

export interface TaskDetail {
  task: Task;
  comments: Comment[];
  images: Image[];
}

export interface UpdateStatus {
  current: string;
  latest: string | null;
  available: string | null;
}

export interface TaskPatch {
  title?: string;
  description?: string;
  status?: string;
  project?: string;
  tags?: string;
  human?: boolean;
}
