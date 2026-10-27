import { useEffect, useMemo, useState } from "react";
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  closestCorners,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragOverEvent,
  type DragStartEvent,
  type UniqueIdentifier,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { ChevronsLeft, ChevronsRight, Plus } from "lucide-react";
import { columnsOf, dropTarget } from "../lib/board";
import { direction, navigate, shift } from "../lib/keys";
import { taskKey } from "../lib/text";
import { projectColor } from "../lib/colors";
import type { Board as BoardData, Task } from "../types";
import { StatusIcon, copyText } from "./bits";
import { Card, CardBody } from "./Card";

type Columns = Record<string, number[]>;

interface Props {
  board: BoardData;
  /** Every project is shown, so cards carry their project's dot. */
  all: boolean;
  openTask: number | null;
  doneCollapsed: boolean;
  onCollapseDone: (collapsed: boolean) => void;
  onOpen: (id: number) => void;
  onNew: () => void;
  onShowAllDone: () => void;
  onMove: (id: number, status: string, before: number | null) => void;
  /** Called with true when a drag starts and false when it ends, so refreshes wait. */
  onDragging: (dragging: boolean) => void;
  /** Whether a card passes the filter bar. */
  visible: (t: Task) => boolean;
  filtering: boolean;
}

const columnId = (status: string) => `column:${status}`;

export function Board({
  board,
  all,
  openTask,
  doneCollapsed,
  onCollapseDone,
  onOpen,
  onNew,
  onShowAllDone,
  onMove,
  onDragging,
  visible,
  filtering,
}: Props) {
  const { statuses, tasks, projects } = board;
  const derived = useMemo(() => columnsOf(tasks.filter(visible), statuses), [tasks, statuses, visible]);
  const totals = useMemo(() => columnsOf(tasks, statuses), [tasks, statuses]);
  const byId = useMemo(() => new Map(tasks.map((t) => [t.id, t])), [tasks]);
  // While dragging, the order shown is ours; otherwise the board's.
  const [order, setOrder] = useState<Columns | null>(null);
  const [active, setActive] = useState<Task | null>(null);
  const cols = order ?? derived;

  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 5 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 200, tolerance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  const containerOf = (id: UniqueIdentifier, c: Columns) =>
    typeof id === "string" && id.startsWith("column:")
      ? id.slice(7)
      : Object.keys(c).find((s) => c[s]!.includes(id as number));

  const start = ({ active }: DragStartEvent) => {
    setOrder(structuredClone(derived));
    setActive(byId.get(active.id as number) ?? null);
    onDragging(true);
  };

  // Moving over another column takes the card there at once, so the column opens a gap for it.
  const over = ({ active, over }: DragOverEvent) => {
    if (!over) return;
    setOrder((c) => {
      if (!c) return c;
      const from = containerOf(active.id, c);
      const to = containerOf(over.id, c);
      if (!from || !to || from === to) return c;
      const target = c[to]!;
      const at = target.indexOf(over.id as number);
      const next = { ...c, [from]: c[from]!.filter((id) => id !== active.id), [to]: [...target] };
      next[to]!.splice(at === -1 ? target.length : at, 0, active.id as number);
      return next;
    });
  };

  const end = ({ active, over }: DragEndEvent) => {
    const c = order;
    finish();
    if (!c || !over) return;
    const id = active.id as number;
    const to = containerOf(over.id, c);
    if (!to) return;
    let final = c;
    const list = c[to]!;
    const from = list.indexOf(id);
    const at = list.indexOf(over.id as number);
    if (from !== -1 && at !== -1 && from !== at) final = { ...c, [to]: arrayMove(list, from, at) };
    const target = dropTarget(final, id);
    const before = derived[to]?.[derived[to]!.indexOf(id) + 1] ?? null;
    const task = byId.get(id);
    if (!target || (task?.status === target.status && derived[to]!.includes(id) && before === target.before)) return;
    onMove(id, target.status, target.before);
  };

  const finish = () => {
    setOrder(null);
    setActive(null);
    onDragging(false);
  };

  const last = statuses.length - 1;

  // The card the keyboard is on: arrows move it, Enter opens it, Shift + arrows move the card.
  const [focused, setFocused] = useState<number | null>(null);
  const open = (id: number) => {
    setFocused(id);
    onOpen(id);
  };
  useEffect(() => {
    if (focused !== null)
      document.querySelector(`[data-id="${focused}"]`)?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }, [focused]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const typing = (e.target as HTMLElement).closest("input, textarea, select, [contenteditable]");
      if (typing || active || e.metaKey || e.ctrlKey || e.altKey) return;
      if (document.querySelector("[role=dialog], [role=alertdialog], [data-radix-popper-content-wrapper]")) return;
      const dir = direction(e.key);
      // Cards of a collapsed column can't be seen, so the keyboard passes them by.
      const seen = doneCollapsed ? { ...derived, [statuses[last]!]: [] } : derived;
      const current = focused !== null && byId.has(focused) && visible(byId.get(focused)!) ? focused : null;
      if (dir && e.shiftKey && current !== null) {
        e.preventDefault();
        const to = shift(derived, statuses, current, dir);
        if (to) onMove(current, to.status, to.before);
      } else if (dir && !e.shiftKey) {
        e.preventDefault();
        setFocused(navigate(seen, statuses, current, dir));
      } else if (e.key === "Enter" && current !== null) {
        e.preventDefault();
        onOpen(current);
      } else if (e.key === "c" && current !== null) {
        copyText(taskKey(byId.get(current)!));
      } else if (e.key === "Escape" && !openTask) {
        setFocused(null);
      }
    };
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  });

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCorners}
      onDragStart={start}
      onDragOver={over}
      onDragEnd={end}
      onDragCancel={finish}
    >
      <div
        className="grid min-h-0 flex-1 auto-cols-[minmax(260px,1fr)] grid-flow-col gap-3 overflow-x-auto px-6 pb-6 max-[700px]:auto-cols-[minmax(78vw,1fr)] max-[700px]:px-3 max-[700px]:pb-3"
        style={{
          gridTemplateColumns: statuses
            .map((_, i) => (i === last && doneCollapsed ? "48px" : "minmax(260px, 1fr)"))
            .join(" "),
        }}
      >
        {statuses.map((status, i) => {
          const ids = cols[status] ?? [];
          const hidden = i === last ? board.done_hidden : 0;
          const total = (totals[status]?.length ?? 0) + hidden;
          return (
            <Column
              key={status}
              status={status}
              statuses={statuses}
              count={filtering ? `${ids.length}/${total}` : total ? String(total) : ""}
              first={i === 0}
              collapsible={i === last}
              collapsed={i === last && doneCollapsed}
              onCollapse={onCollapseDone}
              onNew={onNew}
            >
              {hidden > 0 && (
                <button
                  className="rounded-[10px] px-2.5 py-2 text-left text-muted hover:bg-hover"
                  onClick={onShowAllDone}
                >
                  Show {hidden} older
                </button>
              )}
              <SortableContext items={ids} strategy={verticalListSortingStrategy}>
                {ids.map((id) => {
                  const t = byId.get(id);
                  return (
                    t && (
                      <Card
                        key={id}
                        task={t}
                        projectDot={all ? projectColor(t.project, projects) : null}
                        open={openTask === id}
                        focused={focused === id}
                        onOpen={open}
                      />
                    )
                  );
                })}
              </SortableContext>
            </Column>
          );
        })}
      </div>
      <DragOverlay dropAnimation={{ duration: 160, easing: "ease-out" }}>
        {active && (
          <CardBody
            task={active}
            projectDot={all ? projectColor(active.project, projects) : null}
            open={false}
            lifted
          />
        )}
      </DragOverlay>
    </DndContext>
  );
}

interface ColumnProps {
  status: string;
  statuses: string[];
  /** The number of cards, or shown/total while filtering. */
  count: string;
  first: boolean;
  collapsible: boolean;
  collapsed: boolean;
  onCollapse: (collapsed: boolean) => void;
  onNew: () => void;
  children: React.ReactNode;
}

/** A column. The whole column takes drops, so a card can land in an empty or collapsed one. */
function Column({ status, statuses, count, first, collapsible, collapsed, onCollapse, onNew, children }: ColumnProps) {
  const { setNodeRef, isOver } = useDroppable({ id: columnId(status) });
  const toggle = collapsed ? "Show" : "Collapse";
  return (
    <section
      ref={setNodeRef}
      aria-label={status}
      onClick={collapsed ? () => onCollapse(false) : undefined}
      className={`flex min-h-0 flex-col rounded-[18px] bg-column transition-colors ${collapsed ? "cursor-pointer hover:bg-hover" : ""} ${isOver && collapsed ? "bg-active" : ""}`}
    >
      <h2
        className={`flex items-center gap-2 text-[13px] font-semibold text-muted capitalize ${collapsed ? "py-3 [writing-mode:vertical-rl]" : "px-4 pt-3.5 pb-2.5"}`}
      >
        {collapsible && collapsed && (
          <CollapseButton label={`${toggle} ${status}`} collapsed onClick={() => onCollapse(false)} />
        )}
        <StatusIcon status={status} statuses={statuses} />
        {status}
        <span className="font-normal text-faint tabular-nums">{count}</span>
        {first && (
          <button
            className="icon-btn -my-1.5 -mr-1.5 ml-auto"
            title="Add task (n)"
            aria-label="Add task"
            onClick={onNew}
          >
            <Plus className="size-[18px]" />
          </button>
        )}
        {collapsible && !collapsed && (
          <CollapseButton
            label={`${toggle} ${status}`}
            collapsed={false}
            onClick={() => onCollapse(true)}
            className={first ? "" : "ml-auto"}
          />
        )}
      </h2>
      {!collapsed && <div className="flex min-h-2 flex-col gap-1.5 overflow-y-auto px-2 pt-0.5 pb-2">{children}</div>}
    </section>
  );
}

function CollapseButton({
  label,
  collapsed,
  onClick,
  className = "",
}: {
  label: string;
  collapsed: boolean;
  onClick: () => void;
  className?: string;
}) {
  const Icon = collapsed ? ChevronsLeft : ChevronsRight;
  return (
    <button
      className={`icon-btn -my-1.5 -mr-1.5 text-faint ${collapsed ? "m-0 mb-1" : ""} ${className}`}
      title={label}
      aria-label={label}
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
    >
      <Icon className="size-[18px]" />
    </button>
  );
}
