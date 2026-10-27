import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast, Toaster } from "sonner";
import { ApiError, api } from "./api";
import { Board } from "./components/Board";
import { Logo, fail } from "./components/bits";
import { useConfirm } from "./components/Confirm";
import { FilterBar } from "./components/FilterBar";
import { Header } from "./components/Header";
import { Shortcuts } from "./components/Shortcuts";
import { Search } from "./components/Search";
import { Sidebar, useDeleteProject } from "./components/Sidebar";
import { SyncDialog } from "./components/SyncDialog";
import { useEvents } from "./hooks/useEvents";
import { useRoute } from "./hooks/useRoute";
import { storage, useStoredFlag } from "./hooks/useStored";
import { applyMove } from "./lib/board";
import { filtersQuery, isFiltering, matches, parseFilters } from "./lib/filter";
import { repoName } from "./lib/text";
import type { Board as BoardData, Task } from "./types";

const NEW_TITLE = "New task";

// The panel brings the Markdown renderer, the biggest part of the page: load it on first open.
const TaskPanel = lazy(() => import("./components/TaskPanel").then((m) => ({ default: m.TaskPanel })));

export function App() {
  const qc = useQueryClient();
  const confirm = useConfirm();
  const route = useRoute();
  const { project, task: openTask, setProject, setTask, filters, setFilters } = route;
  // Keyed by the query text, so the board only refilters when a filter changes.
  const filterKey = filtersQuery(filters);
  const visible = useMemo(() => {
    const f = parseFilters(filterKey);
    return (t: Task) => matches(t, f);
  }, [filterKey]);

  // The server leaves out older done tasks unless asked: the board doesn't show them.
  const [allDone, setAllDone] = useState(false);
  const boardKey = ["board", project, allDone];
  const board = useQuery({
    queryKey: boardKey,
    queryFn: () => api.board(project, allDone),
    placeholderData: keepPreviousData,
  });
  const refresh = useCallback(() => qc.invalidateQueries(), [qc]);

  // Refreshes wait while a card is being dragged, so the board doesn't move under it.
  const dragging = useRef(false);
  const missed = useRef(false);
  const online = useEvents(() => {
    if (dragging.current) missed.current = true;
    else refresh();
  });
  const onDragging = useCallback(
    (d: boolean) => {
      dragging.current = d;
      if (!d && missed.current) {
        missed.current = false;
        refresh();
      }
    },
    [refresh],
  );

  // A project that no longer exists, like one deleted from another tab.
  useEffect(() => {
    if (board.error instanceof ApiError && board.error.status === 404 && project !== "all") setProject("all");
    else if (board.error) fail(board.error);
  }, [board.error, project, setProject]);

  const selectProject = (slug: string) => {
    setAllDone(false);
    setProject(slug);
  };

  // A task added with "+" is created at once and opened with its title selected to type
  // over. Closed untouched, it is deleted again.
  const [fresh, setFresh] = useState<number | null>(null);
  const freshRef = useRef(fresh);
  useEffect(() => {
    freshRef.current = fresh;
  }, [fresh]);
  const dropFresh = useCallback(() => {
    const id = freshRef.current;
    if (!id) return;
    setFresh(null);
    api.deleteTask(id).then(refresh, () => {});
  }, [refresh]);
  useEffect(() => {
    // Leaving the page also counts as closing the task.
    const onHide = () =>
      freshRef.current && fetch(`/api/tasks/${freshRef.current}`, { method: "DELETE", keepalive: true });
    addEventListener("pagehide", onHide);
    return () => removeEventListener("pagehide", onHide);
  }, []);

  const closeTask = useCallback(() => {
    // Blurring a field saves it, so do that before the panel lets go of the task.
    const panel = document.getElementById("task-panel");
    if (panel?.contains(document.activeElement)) (document.activeElement as HTMLElement).blur();
    dropFresh();
    setTask(null);
  }, [dropFresh, setTask]);

  const open = useCallback(
    (id: number) => {
      if (id === openTask) return;
      const panel = document.getElementById("task-panel");
      if (panel?.contains(document.activeElement)) (document.activeElement as HTMLElement).blur();
      if (freshRef.current !== id) dropFresh();
      setTask(id);
    },
    [openTask, dropFresh, setTask],
  );

  const newTask = async () => {
    const projects = board.data?.projects ?? [];
    if (!projects.length) return toast("Create a project first");
    const last = storage.get("kan-do.newTaskProject");
    const target = project !== "all" ? project : projects.some((p) => p.slug === last) ? last! : projects[0]!.slug;
    try {
      const t = await api.createTask(target, NEW_TITLE);
      dropFresh();
      setFresh(t.id);
      setTask(t.id);
      refresh();
      // Select the title to type over once the panel has it.
      const select = (tries: number) => {
        const title = document.getElementById("task-title") as HTMLTextAreaElement | null;
        if (title) {
          title.focus({ preventScroll: true });
          title.select();
        } else if (tries) requestAnimationFrame(() => select(tries - 1));
      };
      select(30);
    } catch (e) {
      fail(e);
    }
  };

  // Moves show at once; a failure puts the board back.
  const move = (id: number, status: string, before: number | null) => {
    if (fresh === id) setFresh(null);
    const prev = qc.getQueryData<BoardData>(boardKey);
    if (prev) qc.setQueryData(boardKey, applyMove(prev, id, status, before));
    api.moveTask(id, status, before).then(refresh, (e) => {
      if (prev) qc.setQueryData(boardKey, prev);
      fail(e);
    });
  };

  const deleteProject = useDeleteProject((p) => {
    if (openTask && board.data?.tasks.some((t) => t.id === openTask && t.project === p.slug)) {
      setFresh(null);
      setTask(null);
    }
    if (project === p.slug) selectProject("all");
    refresh();
  });

  const [searching, setSearching] = useState(false);
  const [shortcuts, setShortcuts] = useState(false);
  const pick = (t: Task) => {
    if (project !== "all" && project !== t.project) selectProject(t.project);
    open(t.id);
  };

  // Sync: commits the board and pulls or pushes it through the data folder's git remote.
  const sync = useQuery({ queryKey: ["sync"], queryFn: api.syncRemote, retry: false, staleTime: Infinity });
  const remote = sync.data?.remote ?? null;
  const [syncSetup, setSyncSetup] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const syncNow = async (keep?: "local" | "remote") => {
    setSyncing(true);
    try {
      toast((await api.sync(keep)).message);
      refresh();
    } catch (e) {
      if (e instanceof ApiError && e.data.diverged) {
        setSyncing(false);
        return chooseSide(!!e.data.first);
      }
      fail(e);
    }
    setSyncing(false);
  };
  // Both sides changed the board, which can't be merged: ask which one to keep.
  const chooseSide = async (first: boolean) => {
    const repo = repoName(remote ?? "");
    const choice = await confirm(
      first
        ? {
            title: `${repo} already has a board`,
            body: "Use it on this machine? This machine's current board is kept in kan-do.db.bak.",
            action: "Use the board in the repo",
            alternative: "Replace it with this board",
          }
        : {
            title: "Both machines changed the board",
            body: "This machine and another one both changed the board since the last sync, and boards can't be merged. Keep one side. The board you replace here is kept in kan-do.db.bak.",
            action: "Keep this machine's board",
            alternative: "Take the other machine's board",
          },
    );
    const remoteWins = first ? choice === "action" : choice === "alternative";
    if (choice) syncNow(remoteWins ? "remote" : "local");
  };
  const syncButton = async () => {
    const { data } = await sync.refetch();
    if (data?.remote) syncNow();
    else setSyncSetup(true);
  };

  // Updates: checked once a day, offered in the sidebar.
  const update = useQuery({
    queryKey: ["update"],
    queryFn: () => api.updateStatus(),
    retry: false,
    refetchInterval: 86_400_000,
    staleTime: Infinity,
  });
  const [updating, setUpdating] = useState(false);
  const installUpdate = async (available: string, current: string) => {
    const choice = await confirm({
      title: `Update to ${available}?`,
      body: `You have ${current}. The board restarts, which takes a few seconds.`,
      action: "Update",
    });
    if (choice !== "action") return;
    setUpdating(true);
    try {
      await api.installUpdate();
    } catch (e) {
      setUpdating(false);
      return fail(e);
    }
    // The old program answered, then gives way to the new one: wait for it, then reload.
    await new Promise((r) => setTimeout(r, 1500));
    for (let i = 0; i < 60; i++) {
      try {
        await api.updateStatus();
        break;
      } catch {
        await new Promise((r) => setTimeout(r, 1000));
      }
    }
    location.reload();
  };
  const checkUpdate = async () => {
    try {
      const status = await api.updateStatus(true);
      qc.setQueryData(["update"], status);
      if (status.available) installUpdate(status.available, status.current);
      else if (status.latest) toast(`kan-do is up to date (${status.current})`);
      else toast("Can't reach GitHub to check for updates");
    } catch (e) {
      fail(e);
    }
  };

  const [doneCollapsed, setDoneCollapsed] = useStoredFlag("kan-do.doneCollapsed", true);

  // Keys that work anywhere on the board, unless typing or in a dialog. The board itself
  // handles the arrows, Enter and c for the card the keyboard is on.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        return setSearching(true);
      }
      const typing = (e.target as HTMLElement).closest("input, textarea, select, [contenteditable]");
      const dialog = document.querySelector("[role=dialog], [role=alertdialog]");
      if (typing || dialog || e.metaKey || e.ctrlKey || e.altKey) return;
      const act = {
        "/": () => setSearching(true),
        "?": () => setShortcuts(true),
        n: newTask,
        f: () => document.getElementById("filter-input")?.focus(),
        Escape: () => openTask && closeTask(),
      }[e.key];
      if (act) {
        e.preventDefault();
        act();
      }
    };
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  });

  // A click outside the task panel closes it. Cards open their own task, and menus and dialogs do their job.
  useEffect(() => {
    if (!openTask) return;
    const onDown = (e: MouseEvent) => {
      const t = e.target as HTMLElement;
      if (
        e.button === 0 &&
        t.isConnected &&
        !t.closest(
          "#task-panel, [data-id], [role=dialog], [role=alertdialog], [data-radix-popper-content-wrapper], [data-sonner-toaster]",
        )
      )
        closeTask();
    };
    addEventListener("mousedown", onDown);
    return () => removeEventListener("mousedown", onDown);
  }, [openTask, closeTask]);

  const data = board.data;
  if (!data) {
    return (
      <div className="grid h-full place-items-center">
        <Logo className="size-12 animate-pulse" />
      </div>
    );
  }
  const current = data.projects.find((p) => p.slug === project) ?? null;

  return (
    <>
      <div className="grid h-full overflow-hidden min-[701px]:grid-cols-[260px_minmax(0,1fr)] min-[701px]:grid-rows-[minmax(0,1fr)] max-[700px]:grid-cols-[minmax(0,1fr)] max-[700px]:grid-rows-[auto_minmax(0,1fr)]">
        <Sidebar
          projects={data.projects}
          current={project}
          version={data.version}
          syncRemote={remote}
          syncing={syncing}
          update={update.data ?? null}
          updating={updating}
          onSelect={selectProject}
          onChanged={refresh}
          onDeleted={deleteProject}
          onSearch={() => setSearching(true)}
          onSync={syncButton}
          onSyncSettings={() => setSyncSetup(true)}
          onCheckUpdate={checkUpdate}
          onInstallUpdate={() => update.data?.available && installUpdate(update.data.available, update.data.current)}
        />
        <main className="flex min-h-0 min-w-0 flex-col">
          {data.projects.length ? (
            <>
              <Header project={current} projects={data.projects} />
              <FilterBar
                filters={filters}
                onChange={setFilters}
                tasks={data.tasks}
                projects={data.projects}
                all={project === "all"}
                onHelp={() => setShortcuts(true)}
              />
              <Board
                board={data}
                all={project === "all"}
                openTask={openTask}
                doneCollapsed={doneCollapsed}
                onCollapseDone={setDoneCollapsed}
                onOpen={open}
                onNew={newTask}
                onShowAllDone={() => setAllDone(true)}
                onMove={move}
                onDragging={onDragging}
                visible={visible}
                filtering={isFiltering(filters)}
              />
            </>
          ) : (
            <div className="m-auto max-w-[380px] px-5 py-10 text-center text-muted">
              <Logo className="mx-auto mb-4 size-14" />
              <h1 className="mb-2 font-display text-2xl font-semibold text-text">Welcome to kan-do</h1>
              <p>
                Create your first project in the sidebar. Link it to a GitHub repo and agents working in that repo will
                find it on their own.
              </p>
            </div>
          )}
        </main>
      </div>

      {openTask && (
        <Suspense fallback={null}>
          <TaskPanel
            key={openTask}
            id={openTask}
            statuses={data.statuses}
            projects={data.projects}
            fresh={fresh === openTask}
            onTouched={() => setFresh((f) => (f === openTask ? null : f))}
            onChanged={refresh}
            onClose={closeTask}
          />
        </Suspense>
      )}
      <Shortcuts open={shortcuts} onOpenChange={setShortcuts} />
      <Search open={searching} onOpenChange={setSearching} projects={data.projects} onPick={pick} />
      <SyncDialog
        open={syncSetup}
        remote={remote}
        onOpenChange={setSyncSetup}
        onSaved={() => {
          sync.refetch();
          syncNow();
        }}
      />
      {!online && (
        <div className="fixed top-3 left-1/2 z-50 -translate-x-1/2 rounded-full bg-danger px-3 py-1 text-xs text-white">
          Board not reachable. Is kan-do running?
        </div>
      )}
      <Toaster
        position="bottom-center"
        toastOptions={{
          className:
            "!rounded-full !border-0 !bg-text !text-bg !font-medium !shadow-pop !py-2.5 !px-4 !w-auto !min-h-0",
        }}
      />
    </>
  );
}
