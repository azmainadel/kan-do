import { useRef, useState, type FormEvent, type ReactNode } from "react";
import { Popover } from "radix-ui";
import {
  Check,
  CloudUpload,
  Folder,
  FolderPlus,
  LayoutGrid,
  MoreHorizontal,
  Pencil,
  Search,
  Trash2,
  X,
  ArrowDownCircle,
} from "lucide-react";
import { api } from "../api";
import { PROJECT_COLORS, projectColor, projectColorName } from "../lib/colors";
import { plural, repoName } from "../lib/text";
import type { Project } from "../types";
import { Logo, fail } from "./bits";
import { useConfirm } from "./Confirm";

interface Props {
  projects: Project[];
  current: string;
  version: string;
  syncRemote: string | null;
  syncing: boolean;
  update: { current: string; available: string | null } | null;
  updating: boolean;
  onSelect: (slug: string) => void;
  onChanged: () => void;
  onDeleted: (p: Project) => void;
  onSearch: () => void;
  onSync: () => void;
  onSyncSettings: () => void;
  onCheckUpdate: () => void;
  onInstallUpdate: () => void;
}

export function Sidebar(props: Props) {
  const { projects, current } = props;
  const open = projects.reduce((n, p) => n + p.open, 0);

  return (
    <aside className="flex min-h-0 flex-col border-line bg-sidebar p-2 max-[700px]:border-b min-[701px]:border-r min-[701px]:px-2 min-[701px]:py-3">
      <div className="flex items-center gap-2.5 px-2.5 pb-4 max-[700px]:pb-1.5">
        <Logo className="size-7" />
        <span className="font-display text-[19px] font-bold tracking-tight">kan-do</span>
        <button
          className="icon-btn ml-auto"
          title="Sync with your other machines"
          aria-label="Sync"
          disabled={props.syncing}
          onClick={props.onSync}
        >
          <CloudUpload className={`size-[18px] ${props.syncing ? "animate-pulse" : ""}`} />
        </button>
        <button className="icon-btn" title="Search (/ or ⌘K)" aria-label="Search tasks" onClick={props.onSearch}>
          <Search className="size-[18px]" />
        </button>
      </div>

      <NewProject
        onCreated={(p) => {
          props.onChanged();
          props.onSelect(p.slug);
        }}
      />

      <nav className="flex flex-1 flex-col gap-0.5 overflow-y-auto max-[700px]:flex-row max-[700px]:overflow-x-auto">
        <NavLink
          active={current === "all"}
          onClick={() => props.onSelect("all")}
          icon={<LayoutGrid className="size-[18px]" />}
          name="All projects"
          count={open}
        />
        {projects.length > 0 && <div className="px-2.5 pt-4 pb-1.5 text-faint max-[700px]:hidden">Projects</div>}
        {projects.map((p) => (
          <ProjectItem key={p.slug} project={p} active={current === p.slug} {...props} />
        ))}
      </nav>

      {props.update?.available && (
        <button
          className="mt-2 flex items-center gap-2.5 rounded-[10px] bg-hover px-2.5 py-2 text-left hover:bg-active disabled:opacity-60 max-[700px]:hidden"
          title={`You have ${props.update.current}`}
          disabled={props.updating}
          onClick={props.onInstallUpdate}
        >
          <ArrowDownCircle className="size-[18px] text-amber" />
          {props.updating ? "Updating…" : `Update to ${props.update.available}`}
        </button>
      )}
      <div className="flex items-center gap-2 px-2.5 pt-2.5 text-xs text-faint max-[700px]:hidden">
        <button
          className="-ml-1.5 rounded-md px-1.5 py-0.5 hover:text-text"
          title="Check for updates"
          onClick={props.onCheckUpdate}
        >
          v{props.version}
        </button>
        {props.syncRemote && (
          <button
            className="-mr-1.5 ml-auto inline-flex min-w-0 items-center gap-1 rounded-md px-1.5 py-0.5 hover:text-text"
            title={`Syncs with ${props.syncRemote}. Click to change.`}
            onClick={props.onSyncSettings}
          >
            <CloudUpload className="size-3.5 shrink-0" />
            <span className="truncate">{repoName(props.syncRemote)}</span>
          </button>
        )}
      </div>
    </aside>
  );
}

function NavLink(p: {
  active: boolean;
  onClick: () => void;
  icon: ReactNode;
  name: string;
  count: number;
  hasMenu?: boolean;
}) {
  return (
    <a
      href="#"
      onClick={(e) => {
        e.preventDefault();
        p.onClick();
      }}
      className={`group/link flex items-center gap-2.5 rounded-[10px] px-2.5 py-2 whitespace-nowrap hover:bg-hover ${p.active ? "bg-active hover:bg-active" : ""}`}
    >
      {p.icon}
      <span className="flex-1 truncate">{p.name}</span>
      {/* A project's "more" button takes the count's place on hover, like most sidebars. */}
      <span
        className={`text-xs text-faint tabular-nums ${p.hasMenu ? "group-hover/item:invisible group-focus-within/item:invisible group-data-[menu=true]/item:invisible [@media(hover:none)]:invisible" : ""}`}
      >
        {p.count}
      </span>
    </a>
  );
}

function ProjectItem({
  project: p,
  projects,
  active,
  onSelect,
  onChanged,
  onDeleted,
}: { project: Project; projects: Project[]; active: boolean } & Props) {
  const [menu, setMenu] = useState(false);
  const [renaming, setRenaming] = useState(false);
  const color = projectColor(p.slug, projects);
  const icon = <Folder className="size-[18px] shrink-0" style={{ color }} />;

  if (renaming) {
    return (
      <div className="flex items-center gap-2.5 py-[3px] pr-1.5 pl-2.5">
        {icon}
        <RenameField
          project={p}
          onDone={() => {
            setRenaming(false);
            onChanged();
          }}
        />
      </div>
    );
  }
  return (
    <div className={`group/item relative ${menu ? "[&>a]:bg-hover" : ""}`} data-menu={menu}>
      <NavLink active={active} onClick={() => onSelect(p.slug)} icon={icon} name={p.name} count={p.open} hasMenu />
      <Popover.Root open={menu} onOpenChange={setMenu}>
        <Popover.Trigger
          className={`icon-btn absolute top-1/2 right-1.5 -translate-y-1/2 p-[5px] ${menu ? "block text-text" : "hidden group-hover/item:block group-focus-within/item:block [@media(hover:none)]:block"}`}
          title="More"
          aria-label={`More for ${p.name}`}
        >
          <MoreHorizontal className="size-[18px]" />
        </Popover.Trigger>
        <Popover.Portal>
          <Popover.Content
            align="start"
            sideOffset={4}
            collisionPadding={8}
            className="z-40 w-60 rounded-2xl border border-line bg-menu p-1.5 shadow-pop"
          >
            <ProjectMenu
              project={p}
              projects={projects}
              onChanged={onChanged}
              onRename={() => {
                setMenu(false);
                setRenaming(true);
              }}
              onDelete={() => {
                setMenu(false);
                onDeleted(p);
              }}
            />
          </Popover.Content>
        </Popover.Portal>
      </Popover.Root>
    </div>
  );
}

function ProjectMenu({
  project: p,
  projects,
  onChanged,
  onRename,
  onDelete,
}: {
  project: Project;
  projects: Project[];
  onChanged: () => void;
  onRename: () => void;
  onDelete: () => void;
}) {
  const [repo, setRepo] = useState("");
  const current = projectColorName(p.slug, projects);
  const run = (action: Promise<unknown>) => action.then(onChanged, fail);

  return (
    <>
      <div className="px-2.5 pt-1 text-xs text-faint">Color</div>
      <div className="flex gap-1.5 px-2.5 pt-2 pb-2.5" role="group" aria-label="Color">
        {Object.entries(PROJECT_COLORS).map(([name, hex]) => (
          <button
            key={name}
            className="grid size-5 place-items-center rounded-full text-white transition-transform hover:scale-115"
            style={{ background: hex }}
            title={name}
            aria-label={name}
            aria-pressed={current === name}
            onClick={() => run(api.updateProject(p.slug, { color: name }))}
          >
            {current === name && <Check className="size-3" strokeWidth={3} />}
          </button>
        ))}
      </div>
      <div className="px-2.5 text-xs text-faint">Repos</div>
      <div className="flex flex-col gap-0.5 px-1.5 pt-1.5 pb-2.5">
        {p.repos.map((r) => (
          <div key={r} className="flex items-center gap-1.5 py-0.5 pl-1">
            <span className="flex-1 truncate text-[13px]">{r}</span>
            <button
              className="rounded-md p-1 text-faint hover:bg-danger-bg hover:text-danger"
              title={`Unlink ${r}`}
              aria-label={`Unlink ${r}`}
              onClick={() => run(api.unlinkRepo(p.slug, r))}
            >
              <X className="size-3.5" />
            </button>
          </div>
        ))}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (repo.trim())
              api.linkRepo(p.slug, repo).then(() => {
                setRepo("");
                onChanged();
              }, fail);
          }}
        >
          <input
            className="field mt-1 rounded-[10px] px-2.5 py-1.5 text-[13px]"
            placeholder="Link a repo: owner/repo"
            aria-label="Link a GitHub repo"
            autoComplete="off"
            spellCheck={false}
            value={repo}
            onChange={(e) => setRepo(e.target.value)}
          />
        </form>
      </div>
      <div className="mx-1 mb-1.5 h-px bg-line" />
      <MenuButton onClick={onRename} icon={<Pencil className="size-4" />}>
        Rename
      </MenuButton>
      <MenuButton onClick={onDelete} icon={<Trash2 className="size-4" />} danger>
        Delete project
      </MenuButton>
    </>
  );
}

function MenuButton({
  onClick,
  icon,
  danger,
  children,
}: {
  onClick: () => void;
  icon: ReactNode;
  danger?: boolean;
  children: ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      className={`flex w-full items-center gap-2.5 rounded-[10px] px-2.5 py-2 text-left ${danger ? "text-danger hover:bg-danger-bg" : "hover:bg-hover"}`}
    >
      {icon}
      {children}
    </button>
  );
}

/** Renaming a project in place: Enter or leaving saves, Esc cancels. The slug, and so the task keys, stay. */
function RenameField({ project, onDone }: { project: Project; onDone: () => void }) {
  const [name, setName] = useState(project.name);
  // Enter saves, and the blur that follows must not save again.
  const finished = useRef(false);
  const finish = async (save: boolean) => {
    if (finished.current) return;
    finished.current = true;
    const v = name.trim();
    if (save && v && v !== project.name) await api.updateProject(project.slug, { name: v }).catch(fail);
    onDone();
  };
  return (
    <input
      className="field rounded-lg px-2 py-[5px]"
      aria-label={`Rename ${project.name}`}
      autoFocus
      onFocus={(e) => e.target.select()}
      spellCheck={false}
      autoComplete="off"
      value={name}
      onChange={(e) => setName(e.target.value)}
      onBlur={() => finish(true)}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          finish(true);
        } else if (e.key === "Escape") {
          e.stopPropagation();
          finish(false);
        }
      }}
    />
  );
}

function NewProject({ onCreated }: { onCreated: (p: Project) => void }) {
  const [open, setOpen] = useState(false);
  if (!open) {
    return (
      <button
        className="flex items-center gap-2.5 rounded-[10px] px-2.5 py-2 text-left hover:bg-hover max-[700px]:hidden"
        onClick={() => setOpen(true)}
      >
        <FolderPlus className="size-[18px]" />
        New project
      </button>
    );
  }
  const submit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    api.createProject(String(f.get("slug")), String(f.get("name")), String(f.get("repo"))).then((p) => {
      setOpen(false);
      onCreated(p);
    }, fail);
  };
  return (
    <form
      className="flex flex-col gap-1.5 px-0.5 pt-1 pb-2.5 max-[700px]:hidden"
      onSubmit={submit}
      onKeyDown={(e) => e.key === "Escape" && setOpen(false)}
    >
      <input
        className="field"
        name="slug"
        placeholder="slug, e.g. api"
        required
        autoFocus
        autoComplete="off"
        pattern="[a-zA-Z0-9][a-zA-Z0-9\-]{0,31}"
        title="Letters, digits and -, up to 32"
      />
      <input className="field" name="name" placeholder="Name (optional)" autoComplete="off" />
      <input className="field" name="repo" placeholder="GitHub owner/repo (optional)" autoComplete="off" />
      <div className="mt-0.5 flex gap-1.5">
        <button className="btn btn-primary" type="submit">
          Create
        </button>
        <button className="btn" type="button" onClick={() => setOpen(false)}>
          Cancel
        </button>
      </div>
    </form>
  );
}

/** Asks before deleting a project with its tasks, then deletes it. */
export function useDeleteProject(after: (p: Project) => void) {
  const confirm = useConfirm();
  return async (p: Project) => {
    const tasks = p.total ? ` and its ${plural(p.total, "task")}, with their comments` : "";
    const choice = await confirm({
      title: `Delete ${p.name}?`,
      body: `This deletes project ${p.slug}${tasks}. It cannot be undone.`,
      action: "Delete project",
      danger: true,
    });
    if (choice === "action") api.deleteProject(p.slug).then(() => after(p), fail);
  };
}
