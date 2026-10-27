import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { UserRound } from "lucide-react";
import { memo } from "react";
import { plainText, taskKey } from "../lib/text";
import type { Task } from "../types";
import { Dot, copyText } from "./bits";

interface Props {
  task: Task;
  /** The project's color, shown as a dot on All projects to tell projects apart. */
  projectDot: string | null;
  open: boolean;
  /** The keyboard is on this card. */
  focused?: boolean;
  onOpen: (id: number) => void;
}

/** A card on the board, draggable within and across columns. */
export const Card = memo(function Card({ task, projectDot, open, focused, onOpen }: Props) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: task.id });
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Translate.toString(transform), transition }}
      {...attributes}
      {...listeners}
      data-id={task.id}
      onClick={() => onOpen(task.id)}
      className={`touch-manipulation rounded-[14px] ${isDragging ? "opacity-40" : ""} ${focused ? "outline-2 -outline-offset-2 outline-amber" : ""}`}
    >
      <CardBody task={task} projectDot={projectDot} open={open} />
    </div>
  );
});

/** What a card shows; also drawn under the pointer while dragging. */
export function CardBody({ task: t, projectDot, open, lifted }: Omit<Props, "onOpen"> & { lifted?: boolean }) {
  const desc = t.description ? plainText(t.description) : "";
  return (
    <div
      className={`cursor-grab rounded-[14px] border px-3 py-2.5 transition-colors select-none ${
        open ? "border-amber bg-active" : "border-line bg-card hover:bg-card-hover dark:border-transparent"
      } ${lifted ? "rotate-1 shadow-pop" : ""}`}
    >
      <div className="flex items-start gap-2.5">
        <div className="min-w-0 flex-1 [overflow-wrap:anywhere]">{t.title}</div>
        {t.human && (
          <span className="mt-0.5 text-muted" title="Human task: agents skip it">
            <UserRound className="size-[15px]" />
          </span>
        )}
        <button
          className="mt-0.5 inline-flex shrink-0 cursor-copy items-center gap-1.5 text-xs text-faint tabular-nums hover:text-text"
          title={`Copy ${taskKey(t)}`}
          onClick={(e) => {
            e.stopPropagation();
            copyText(taskKey(t));
          }}
        >
          {projectDot && <Dot color={projectDot} className="size-1.5" />}#{t.id}
        </button>
      </div>
      {desc && <div className="mt-0.5 truncate text-[13px] text-muted">{desc}</div>}
      {t.tags.length > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-1">
          {t.tags.map((tag) => (
            <span key={tag} className="rounded-full bg-hover px-2 py-px text-xs text-muted">
              {tag}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
