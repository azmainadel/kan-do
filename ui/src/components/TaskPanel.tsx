import { useEffect, useState, type DragEvent, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { Switch } from "radix-ui";
import { ArrowUp, Trash2, X } from "lucide-react";
import { ApiError, api } from "../api";
import { projectColor } from "../lib/colors";
import { cleanTags, plural, taskKey } from "../lib/text";
import type { Image, Project, Task, TaskPatch } from "../types";
import { Dot, Time, copyText, fail } from "./bits";
import { useConfirm } from "./Confirm";
import { AutoTextarea, InlineField, hasFiles, imagesIn } from "./fields";
import { Markdown } from "./Markdown";
import { toast } from "sonner";

interface Props {
  id: number;
  statuses: string[];
  projects: Project[];
  /** The task was just added and nothing in it saved yet: Esc and closing drop it. */
  fresh: boolean;
  /** Something in the task was saved, so it is no longer fresh. */
  onTouched: () => void;
  onChanged: () => void;
  onClose: () => void;
}

/** The task panel: every field is edited in place and saves when it loses focus. */
export function TaskPanel({ id, statuses, projects, fresh, onTouched, onChanged, onClose }: Props) {
  const confirm = useConfirm();
  const { data, error } = useQuery({ queryKey: ["task", id], queryFn: () => api.task(id) });

  useEffect(() => {
    if (error instanceof ApiError && error.status === 404) {
      toast("That task was deleted");
      onClose();
    }
  }, [error, onClose]);

  if (!data) return null;
  const { task, comments, images } = data;

  const patch = (fields: TaskPatch) => {
    onTouched();
    api.updateTask(id, fields).then(onChanged, (e) => {
      fail(e);
      onChanged();
    });
  };
  const attach = async (files: File[]) => {
    try {
      for (const f of files) {
        await api.attachImage(id, await api.uploadImage(f));
        onTouched();
      }
    } catch (e) {
      fail(e);
    }
    onChanged();
  };
  const remove = async () => {
    const extra = task.comments ? ` and its ${plural(task.comments, "comment")}` : "";
    const choice = await confirm({
      title: `Delete ${taskKey(task)}?`,
      body: `This deletes "${task.title}"${extra}. It cannot be undone.`,
      action: "Delete task",
      danger: true,
    });
    if (choice !== "action") return;
    onTouched();
    api.deleteTask(id).then(() => {
      onClose();
      onChanged();
    }, fail);
  };
  // Esc in a field of a task just added drops the task.
  const escape = () => {
    if (!fresh) return false;
    onClose();
    return true;
  };
  const closeOnCmdEnter = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      onClose();
    }
  };
  const pasteImages = (e: React.ClipboardEvent) => {
    const files = imagesIn(e.clipboardData);
    if (files.length) {
      e.preventDefault();
      attach(files);
    }
  };

  return (
    <aside
      id="task-panel"
      aria-label={`Task ${taskKey(task)}`}
      className="fixed inset-y-0 right-0 z-30 flex w-[min(500px,100vw)] animate-[slide-in_.18s_ease-out] flex-col border-l border-line bg-bg shadow-pop"
    >
      <div className="flex items-center gap-2 px-4 pt-3.5 pb-1.5 pl-5">
        <button
          className="min-w-0 cursor-copy truncate font-mono text-xs whitespace-nowrap text-muted hover:text-text"
          title={`Copy ${taskKey(task)}`}
          onClick={() => copyText(taskKey(task))}
        >
          {taskKey(task)}
        </button>
        <ProjectPick task={task} projects={projects} onPick={(project) => patch({ project })} />
        <span className="flex-1" />
        <button
          className="icon-btn hover:!bg-danger-bg hover:!text-danger"
          title="Delete task"
          aria-label="Delete task"
          onClick={remove}
        >
          <Trash2 className="size-[18px]" />
        </button>
        <button className="icon-btn" title="Close (Esc)" aria-label="Close" onClick={onClose}>
          <X className="size-[18px]" />
        </button>
      </div>

      <div className="flex flex-1 flex-col gap-5 overflow-y-auto px-5 pt-2 pb-6 [&>*]:shrink-0">
        <InlineField
          key={`title-${id}`}
          id="task-title"
          className="inline-edit font-display text-xl font-semibold tracking-tight"
          ariaLabel="Title"
          placeholder="Task title"
          value={task.title}
          autoFocus={fresh}
          same={(a, b) => !a.trim() || a.trim().replace(/\s+/g, " ") === b}
          onSave={(title) => patch({ title })}
          onEscape={escape}
          onPaste={pasteImages}
          onKeyDown={(e) => {
            closeOnCmdEnter(e);
            // The title is one line: Enter, like Tab, goes on to the description.
            if ((e.key === "Enter" || e.key === "Tab") && !e.shiftKey && !e.metaKey && !e.ctrlKey) {
              e.preventDefault();
              document.getElementById("task-description")?.focus();
            }
          }}
        />

        <div className="grid grid-cols-[96px_1fr] items-center gap-x-3 gap-y-2 text-muted">
          Status
          <span>
            <select
              className="field w-auto cursor-pointer rounded-full py-1 pr-8 pl-3 text-text capitalize"
              aria-label="Status"
              value={task.status}
              onChange={(e) => patch({ status: e.target.value })}
            >
              {(statuses.includes(task.status) ? statuses : [...statuses, task.status]).map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </span>
          Tags
          <InlineField
            key={`tags-${id}`}
            className="inline-edit text-text"
            ariaLabel="Tags"
            placeholder="Add tags, like bug, ui"
            value={task.tags.join(", ")}
            enterSaves
            same={(a, b) => cleanTags(a).join(",") === cleanTags(b).join(",")}
            onSave={(tags) => patch({ tags })}
            onEscape={escape}
          />
          Human
          <label className="flex items-center gap-2.5">
            <Switch.Root
              checked={task.human}
              onCheckedChange={(human) => patch({ human })}
              aria-label="Human task"
              className="relative h-5 w-[34px] shrink-0 rounded-full bg-active transition-colors data-[state=checked]:bg-accent"
            >
              <Switch.Thumb className="block size-3.5 translate-x-[3px] rounded-full bg-white shadow transition-transform data-[state=checked]:translate-x-[17px]" />
            </Switch.Root>
            <span className="text-xs text-faint">Agents skip it</span>
          </label>
          Created
          <span className="text-text">
            <Time ts={task.created_at} />
          </span>
          Updated
          <span className="text-text">
            <Time ts={task.updated_at} />
          </span>
        </div>

        <Description
          key={`desc-${id}`}
          task={task}
          images={images.filter((i) => i.comment_id === null)}
          onSave={(description) => patch({ description })}
          onEscape={escape}
          onKeyDown={closeOnCmdEnter}
          onImages={attach}
          onRemoveImage={(img) => api.deleteImage(img.id).then(onChanged, fail)}
        />

        <section>
          <h3 className="mb-2 flex items-center gap-2 text-[13px] font-semibold text-muted">
            Comments <span className="font-normal text-faint">{comments.length || ""}</span>
          </h3>
          {comments.map((c) => (
            <div key={c.id} className="group py-2.5 first:pt-0">
              <div className="mb-0.5 flex items-center gap-1 text-xs text-faint">
                <b className="font-semibold text-text">{c.author}</b> · <Time ts={c.created_at} />
                <button
                  className="invisible ml-auto rounded-md p-1 group-hover:visible hover:bg-danger-bg hover:text-danger focus-visible:visible [@media(hover:none)]:visible"
                  title="Delete comment"
                  aria-label="Delete comment"
                  onClick={async () => {
                    const choice = await confirm({
                      title: "Delete this comment?",
                      body: "It cannot be undone.",
                      action: "Delete",
                      danger: true,
                    });
                    if (choice === "action") api.deleteComment(c.id).then(onChanged, fail);
                  }}
                >
                  <Trash2 className="size-3.5" />
                </button>
              </div>
              {c.body && <Markdown>{c.body}</Markdown>}
              <Thumbs
                images={images.filter((i) => i.comment_id === c.id).map((i) => ({ file: i.file }))}
                className="mt-1.5"
              />
            </div>
          ))}
          <Composer
            key={`composer-${id}`}
            taskId={id}
            onSent={() => {
              onTouched();
              onChanged();
            }}
          />
        </section>
      </div>
    </aside>
  );
}

/** The project, shown as a tag; picking another one moves the task. */
function ProjectPick({ task, projects, onPick }: { task: Task; projects: Project[]; onPick: (slug: string) => void }) {
  const slugs = projects.map((p) => p.slug);
  if (!slugs.includes(task.project)) slugs.push(task.project);
  return (
    <label className="inline-flex items-center gap-1.5 rounded-full bg-hover py-px pr-2.5 pl-2 text-xs text-muted focus-within:bg-active hover:bg-active hover:text-text">
      <Dot color={projectColor(task.project, projects)} className="size-1.5" />
      <select
        className="cursor-pointer appearance-none bg-transparent [field-sizing:content]"
        title="Project"
        aria-label="Project"
        value={task.project}
        onChange={(e) => onPick(e.target.value)}
      >
        {slugs.map((s) => (
          <option key={s}>{s}</option>
        ))}
      </select>
    </label>
  );
}

interface DescriptionProps {
  task: Task;
  images: Image[];
  onSave: (description: string) => void;
  onEscape: () => boolean;
  onKeyDown: (e: React.KeyboardEvent<HTMLTextAreaElement>) => void;
  onImages: (files: File[]) => void;
  onRemoveImage: (img: Image) => void;
}

/** The description: rendered Markdown, which turns into the editor on a click. */
function Description({ task, images, onSave, onEscape, onKeyDown, onImages, onRemoveImage }: DescriptionProps) {
  const [editing, setEditing] = useState(!task.description);
  const preview = !editing && !!task.description;
  const edit = () => {
    setEditing(true);
    requestAnimationFrame(() => document.getElementById("task-description")?.focus());
  };
  const drop = useDrop(onImages);

  return (
    <section className={`rounded-xl ${drop.over ? "bg-hover" : ""}`} {...drop.handlers}>
      <h3 className="mb-2 flex items-center gap-2 text-[13px] font-semibold text-muted">
        Description
        {task.description && (
          <span className="ml-auto flex gap-0.5 rounded-full bg-hover p-0.5 text-xs font-normal">
            <Tab active={!preview} onClick={edit}>
              Write
            </Tab>
            <Tab active={preview} onClick={() => setEditing(false)}>
              Preview
            </Tab>
          </span>
        )}
      </h3>
      {preview ? (
        <div
          className="-mx-2.5 -my-1.5 cursor-text rounded-[10px] px-2.5 py-1.5 hover:bg-hover"
          title="Click to edit"
          onClick={(e) => !(e.target as HTMLElement).closest("a, input") && edit()}
        >
          <Markdown>{task.description}</Markdown>
        </div>
      ) : (
        <InlineField
          id="task-description"
          className="inline-edit min-h-[120px] placeholder-shown:bg-field"
          ariaLabel="Description"
          placeholder="Add a description (Markdown)"
          value={task.description}
          multiline
          onSave={onSave}
          onEscape={onEscape}
          onKeyDown={onKeyDown}
          onBlurred={() => setEditing(false)}
          onPaste={(e) => {
            const files = imagesIn(e.clipboardData);
            if (files.length) {
              e.preventDefault();
              onImages(files);
            }
          }}
        />
      )}
      <Thumbs
        images={images.map((img) => ({ file: img.file, onRemove: () => onRemoveImage(img) }))}
        className="mt-2.5"
      />
    </section>
  );
}

function Tab({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button
      className={`rounded-full px-2.5 py-0.5 ${active ? "bg-bg text-text shadow-sm" : "text-muted hover:text-text"}`}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

/** Dropping image files on an element; it reports when files are over it. */
function useDrop(onImages: (files: File[]) => void) {
  const [over, setOver] = useState(false);
  return {
    over,
    handlers: {
      onDragOver: (e: DragEvent) => {
        if (!hasFiles(e.dataTransfer)) return;
        e.preventDefault();
        setOver(true);
      },
      onDragLeave: (e: DragEvent) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node)) setOver(false);
      },
      onDrop: (e: DragEvent) => {
        setOver(false);
        const files = imagesIn(e.dataTransfer);
        if (!files.length) return;
        e.preventDefault();
        onImages(files);
      },
    },
  };
}

/** Thumbnails that open the full image; with onRemove, an × takes one off. */
function Thumbs({ images, className = "" }: { images: { file: string; onRemove?: () => void }[]; className?: string }) {
  if (!images.length) return null;
  return (
    <div className={`flex flex-wrap gap-2 ${className}`}>
      {images.map(({ file, onRemove }, i) => (
        <div key={file + i} className="group/thumb relative size-[72px] shrink-0 overflow-hidden rounded-xl bg-field">
          <a href={`/images/${file}`} target="_blank" rel="noopener">
            <img src={`/images/${file}`} alt="Attached image" className="size-full object-cover" />
          </a>
          {onRemove && (
            <button
              type="button"
              className="absolute top-1 right-1 hidden size-[22px] place-items-center rounded-full bg-black/65 text-white group-hover/thumb:grid focus-visible:grid [@media(hover:none)]:grid"
              title="Remove image"
              aria-label="Remove image"
              onClick={onRemove}
            >
              <X className="size-3.5" />
            </button>
          )}
        </div>
      ))}
    </div>
  );
}

/** The comment box, with its send button inside like a chat composer. Images pasted or dropped here go with the comment. */
function Composer({ taskId, onSent }: { taskId: number; onSent: () => void }) {
  const [body, setBody] = useState("");
  const [pending, setPending] = useState<string[]>([]);
  const [sending, setSending] = useState(false);
  const add = async (files: File[]) => {
    try {
      for (const f of files) {
        const file = await api.uploadImage(f);
        setPending((p) => [...p, file]);
      }
    } catch (e) {
      fail(e);
    }
  };
  const drop = useDrop(add);
  const canSend = (!!body.trim() || pending.length > 0) && !sending;
  const send = async () => {
    if (!canSend) return;
    setSending(true);
    try {
      await api.addComment(taskId, body.trim(), pending);
      setBody("");
      setPending([]);
      onSent();
    } catch (e) {
      fail(e);
    }
    setSending(false);
  };

  return (
    <form
      className={`relative mt-1.5 rounded-[20px] ${drop.over ? "bg-active" : "bg-field"}`}
      onSubmit={(e) => {
        e.preventDefault();
        send();
      }}
      {...drop.handlers}
    >
      <Thumbs
        images={pending.map((file, i) => ({ file, onRemove: () => setPending((p) => p.filter((_, j) => j !== i)) }))}
        className="px-3 pt-3"
      />
      <AutoTextarea
        className="block max-h-[50vh] min-h-[84px] w-full resize-none bg-transparent py-3 pr-13 pl-4 placeholder:text-faint"
        placeholder="Add a comment"
        aria-label="Comment"
        value={body}
        onChange={(e) => setBody(e.target.value)}
        onPaste={(e) => {
          const files = imagesIn(e.clipboardData);
          if (files.length) {
            e.preventDefault();
            add(files);
          }
        }}
        onKeyDown={(e) => {
          // Esc leaves the box; the next one closes the panel.
          if (e.key === "Escape") e.currentTarget.blur();
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault();
            send();
          }
        }}
      />
      <button
        type="submit"
        className="absolute right-2 bottom-2 grid size-8 place-items-center rounded-full bg-accent text-accent-text disabled:opacity-25"
        title="Send (⌘↵)"
        aria-label="Send comment"
        disabled={!canSend}
      >
        <ArrowUp className="size-4" strokeWidth={2.2} />
      </button>
    </form>
  );
}
