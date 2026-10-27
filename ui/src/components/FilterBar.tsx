import { useMemo, type ReactNode } from "react";
import { Popover } from "radix-ui";
import { Check, ChevronDown, Keyboard, ListFilter, X } from "lucide-react";
import { NO_FILTERS, isFiltering, tagCounts, type Filters, type Owner } from "../lib/filter";
import { projectColor } from "../lib/colors";
import type { Project, Task } from "../types";
import { Dot } from "./bits";

interface Props {
  filters: Filters;
  onChange: (f: Filters) => void;
  tasks: Task[];
  projects: Project[];
  /** On All projects, cards can be narrowed to some projects. */
  all: boolean;
  onHelp: () => void;
}

const OWNERS: [Owner, string][] = [
  ["all", "All"],
  ["agent", "Agents"],
  ["human", "Human"],
];

/** Narrows the cards shown on the board. */
export function FilterBar({ filters, onChange, tasks, projects, all, onHelp }: Props) {
  const tags = useMemo(() => tagCounts(tasks), [tasks]);
  const set = (over: Partial<Filters>) => onChange({ ...filters, ...over });
  const toggle = (list: string[], v: string) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v]);

  return (
    <div className="flex flex-wrap items-center gap-2 px-6 pb-3 max-[700px]:px-3">
      <label className="flex h-8 w-56 items-center gap-2 rounded-full bg-field px-3 focus-within:bg-active max-[700px]:w-full">
        <ListFilter className="size-4 shrink-0 text-faint" />
        <input
          id="filter-input"
          className="min-w-0 flex-1 bg-transparent placeholder:text-faint focus-visible:outline-none"
          placeholder="Filter cards"
          aria-label="Filter cards"
          autoComplete="off"
          spellCheck={false}
          value={filters.q}
          onChange={(e) => set({ q: e.target.value })}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.stopPropagation();
              set({ q: "" });
              e.currentTarget.blur();
            } else if (e.key === "Enter") e.currentTarget.blur();
          }}
        />
        {filters.q && (
          <button
            className="text-faint hover:text-text"
            aria-label="Clear the filter text"
            onClick={() => set({ q: "" })}
          >
            <X className="size-3.5" />
          </button>
        )}
      </label>

      <Pick
        label="Tags"
        chosen={filters.tags}
        empty="No tags on these cards yet."
        options={tags.map(([tag, n]) => ({ value: tag, label: tag, hint: String(n) }))}
        onToggle={(tag) => set({ tags: toggle(filters.tags, tag) })}
      />

      {all && projects.length > 1 && (
        <Pick
          label="Projects"
          chosen={filters.projects}
          options={projects.map((p) => ({
            value: p.slug,
            label: p.name,
            icon: <Dot color={projectColor(p.slug, projects)} />,
          }))}
          onToggle={(slug) => set({ projects: toggle(filters.projects, slug) })}
        />
      )}

      <div className="flex h-8 items-center rounded-full bg-field p-0.5" role="radiogroup" aria-label="Whose tasks">
        {OWNERS.map(([owner, label]) => (
          <button
            key={owner}
            role="radio"
            aria-checked={filters.owner === owner}
            onClick={() => set({ owner })}
            className={`h-7 rounded-full px-3 ${filters.owner === owner ? "bg-bg text-text shadow-sm" : "text-muted hover:text-text"}`}
          >
            {label}
          </button>
        ))}
      </div>

      {isFiltering(filters) && (
        <button
          className="h-8 rounded-full px-3 text-muted hover:bg-hover hover:text-text"
          onClick={() => onChange(NO_FILTERS)}
        >
          Clear
        </button>
      )}

      <button
        className="icon-btn ml-auto max-[700px]:hidden"
        title="Keyboard shortcuts (?)"
        aria-label="Keyboard shortcuts"
        onClick={onHelp}
      >
        <Keyboard className="size-[18px]" />
      </button>
    </div>
  );
}

interface PickProps {
  label: string;
  chosen: string[];
  options: { value: string; label: string; hint?: string; icon?: ReactNode }[];
  empty?: string;
  onToggle: (value: string) => void;
}

/** A button that opens a list of options to pick several of. */
function Pick({ label, chosen, options, empty, onToggle }: PickProps) {
  const names = chosen.map((v) => options.find((o) => o.value === v)?.label ?? v);
  return (
    <Popover.Root>
      <Popover.Trigger
        className={`flex h-8 max-w-64 items-center gap-1.5 rounded-full px-3 ${chosen.length ? "bg-accent text-accent-text" : "bg-field text-muted hover:text-text"}`}
      >
        <span className="truncate">{chosen.length ? `${label}: ${names.join(", ")}` : label}</span>
        <ChevronDown className="size-3.5 shrink-0" />
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          align="start"
          sideOffset={6}
          collisionPadding={8}
          className="z-40 max-h-80 w-56 overflow-y-auto rounded-2xl border border-line bg-menu p-1.5 shadow-pop"
        >
          {options.length === 0 && <p className="px-2.5 py-2 text-faint">{empty}</p>}
          {options.map((o) => {
            const on = chosen.includes(o.value);
            return (
              <button
                key={o.value}
                role="menuitemcheckbox"
                aria-checked={on}
                onClick={() => onToggle(o.value)}
                className="flex w-full items-center gap-2.5 rounded-[10px] px-2.5 py-1.5 text-left hover:bg-hover"
              >
                <span
                  className={`grid size-4 shrink-0 place-items-center rounded ${on ? "bg-accent text-accent-text" : "border border-faint"}`}
                >
                  {on && <Check className="size-3" strokeWidth={3} />}
                </span>
                {o.icon}
                <span className="flex-1 truncate">{o.label}</span>
                {o.hint && <span className="text-xs text-faint tabular-nums">{o.hint}</span>}
              </button>
            );
          })}
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
