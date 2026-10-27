import { useEffect, useState } from "react";
import { Command } from "cmdk";
import { Dialog } from "radix-ui";
import { Search as SearchIcon } from "lucide-react";
import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { projectColor } from "../lib/colors";
import { taskKey } from "../lib/text";
import type { Project, Task } from "../types";
import { Dot, fail } from "./bits";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projects: Project[];
  onPick: (t: Task) => void;
}

/** The search window: tasks of every project, found by the server, open ones first. Without words it shows the latest changes. */
export function Search({ open, onOpenChange, projects, onPick }: Props) {
  const [q, setQ] = useState("");
  const [debounced, setDebounced] = useState("");
  useEffect(() => {
    const t = setTimeout(() => setDebounced(q.trim()), 120);
    return () => clearTimeout(t);
  }, [q]);
  const { data: results = [], isPlaceholderData } = useQuery({
    queryKey: ["search", debounced],
    queryFn: () => api.search(debounced),
    enabled: open,
    placeholderData: keepPreviousData,
  });
  // Results still from the last words: Enter fetches the ones for what's typed, then opens the first.
  const stale = q.trim() !== debounced || isPlaceholderData;
  const qc = useQueryClient();
  const choose = (t: Task) => {
    onOpenChange(false);
    setQ("");
    onPick(t);
  };
  const chooseFirstFresh = () => {
    const words = q.trim();
    qc.fetchQuery({ queryKey: ["search", words], queryFn: () => api.search(words) }).then((r) => {
      if (r[0]) choose(r[0]);
    }, fail);
  };

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o);
        if (!o) setQ("");
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-black/45" />
        <Dialog.Content
          aria-describedby={undefined}
          className="fixed top-[12vh] left-1/2 z-50 flex max-h-[70vh] w-[min(640px,calc(100vw-32px))] -translate-x-1/2 flex-col overflow-hidden rounded-[18px] border border-line bg-menu shadow-pop"
        >
          <Dialog.Title className="sr-only">Search tasks</Dialog.Title>
          {/* Results come ordered from the server, so cmdk only handles the keyboard. */}
          <Command
            shouldFilter={false}
            loop
            className="flex min-h-0 flex-col"
            onKeyDown={(e) => {
              if (e.key === "Enter" && stale) {
                e.preventDefault();
                chooseFirstFresh();
              }
            }}
          >
            <label className="flex items-center gap-2.5 border-b border-line px-[18px] py-3.5">
              <SearchIcon className="size-[18px] text-faint" />
              <Command.Input
                value={q}
                onValueChange={setQ}
                placeholder="Search tasks"
                className="min-w-0 flex-1 bg-transparent text-base placeholder:text-faint focus-visible:outline-none"
              />
            </label>
            <Command.List className="overflow-y-auto p-1.5">
              <Command.Empty className="px-3 py-3.5 text-faint">
                {debounced ? "No tasks match." : "No tasks yet."}
              </Command.Empty>
              {results.length > 0 && (
                <Command.Group
                  heading={debounced ? "Tasks" : "Recent"}
                  className="[&_[cmdk-group-heading]]:px-3 [&_[cmdk-group-heading]]:pt-2 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:text-faint"
                >
                  {results.map((t) => (
                    <Command.Item
                      key={t.id}
                      value={String(t.id)}
                      onSelect={() => choose(t)}
                      className="flex cursor-pointer items-center gap-2.5 rounded-[10px] px-3 py-2.5 data-[selected=true]:bg-hover"
                    >
                      <Dot color={projectColor(t.project, projects)} />
                      <span className="min-w-0 flex-1 truncate">{t.title}</span>
                      <span className="flex shrink-0 items-center gap-2 text-xs text-faint">
                        <span className="font-mono">{taskKey(t)}</span>
                        {t.status}
                      </span>
                    </Command.Item>
                  ))}
                </Command.Group>
              )}
            </Command.List>
          </Command>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
