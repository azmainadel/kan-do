# kan-do spec

kan-do is a kanban board for one person and their coding agents. One program serves the board in the browser and an MCP server for agents, and both read and write the same SQLite file. A private git repo keeps the board in step across machines. There are no accounts and no hosted server.

This spec covers v0.1: the full feature set below, a Go backend, and a React frontend built into the binary.

## Goals

- One static binary for macOS and Linux (amd64, arm64, and armv7 for older Raspberry Pis), with the UI embedded.
- No setup. The first run creates the data folder and opens the board.
- Agents and the person share one board and see each other's changes within a second.
- Agent replies stay short plain text, to save tokens.

Out of scope for v0.1: accounts and logins, Windows, hosted or multi-user mode, merging boards that diverged.

## Stack

| Part | Choice |
| --- | --- |
| Language | Go 1.26 |
| HTTP | `net/http` with Go 1.22+ pattern routing |
| Database | SQLite through `modernc.org/sqlite` (pure Go, no cgo) |
| MCP | `github.com/modelcontextprotocol/go-sdk`, stdio and streamable HTTP |
| CLI | `github.com/spf13/cobra` |
| Config | `github.com/BurntSushi/toml` |
| UI | React 19, TypeScript, Vite, Tailwind CSS 4 |
| UI libraries | TanStack Query, dnd-kit, Radix UI primitives, cmdk, react-markdown with remark-gfm, lucide-react, sonner |
| Release | GoReleaser, plus an `install.sh` for `curl \| sh` |

## Repository layout

```
cmd/kan-do/          main package: CLI wiring only
internal/home/       data folder, config.toml, file lock
internal/store/      schema, migrations, all SQL
internal/web/        HTTP API, SSE, static files, host guard
internal/mcp/        MCP tools, stdio and HTTP
internal/sync/       git sync
internal/service/    launchd and systemd units
internal/update/     release check and self-update
internal/export/     Markdown and JSON export
ui/                  React app (Vite); builds into internal/web/dist
docs/                this spec
```

The UI builds into `internal/web/dist/app`, which is git-ignored. `go build` works without Node: with no build there, the server serves an embedded `placeholder.html` that says to run `make ui`. Release builds run the UI build first. `make dev` runs Vite with hot reload and proxies the API to a running board.

## Data folder

The data folder is chosen in this order: `--home <dir>`, then `$KANDO_HOME`, then `~/kan-do`. It holds:

| File | Purpose |
| --- | --- |
| `kan-do.db` | The board |
| `config.toml` | Settings, written with comments on first run |
| `images/` | Attached images, named `<16 hex>.<png\|jpg\|gif\|webp>` by a hash of their content |
| `kan-do.lock` | Lock file. Every operation takes a shared lock and sync takes an exclusive one. |
| `.gitignore` | Leaves out the journal, lock and `*.bak` files |
| `kan-do.db.bak` | The old board, kept when sync takes the remote one |

### config.toml

```toml
port = 4242
listen = "127.0.0.1"                         # "0.0.0.0" opens the board to the network, which has no logins
statuses = ["todo", "doing", "review", "done"]  # new tasks go in the first, the last counts as done
mcp_http = true                              # also serve MCP at /mcp
```

Unknown keys are an error. `statuses` needs at least two entries, each made of lowercase letters, digits, `-` or `_`, with no duplicates.

## Database

SQLite in rollback-journal mode (`journal_mode=TRUNCATE`) with `foreign_keys=ON` and a 5 second busy timeout. Each operation opens a connection and closes it afterwards. This means a database file replaced by sync takes effect on the next request. Writes run in `BEGIN IMMEDIATE` transactions.

Tables:

- `meta(key, value)` holds `schema_version`, `compatible` (the oldest schema version that can still read this file) and `changes`.
- `projects(id, slug UNIQUE, name, repo, color, created_at)`. The `repo` column is legacy and always NULL.
- `project_repos(repo PRIMARY KEY, project_id)`. A project can have several repos, and each repo belongs to one project.
- `tasks(id, project_id, title, description, status, position REAL, tags, human, created_at, updated_at)`. `tags` is comma-separated.
- `comments(id, task_id, author, body, created_at)`
- `images(id, task_id, comment_id NULL, file, created_at)`

Triggers on every insert, update and delete increment `meta.changes`. Deleting a project cascades to its tasks, comments and images.

Migrations are a numbered list applied in order (currently six). A database whose `schema_version` is newer than this build still opens if its `compatible` value is at or below this build's version. Otherwise the error tells the person to run `kan-do update`. `migrate` writes nothing when the schema is already current, because every agent session starts a process that calls it.

Timestamps are UTC strings in the form `2026-10-25T09:30:00Z`.

## Domain rules

These match the existing behaviour exactly. Unit tests pin them down.

- A slug is 1 to 32 characters from `a-z`, `0-9` and `-`, starting with a letter or digit. Input is trimmed and lowercased. The name defaults to the slug.
- Repos are stored as lowercase `owner/repo`. The input can be `owner/repo` or any git remote or GitHub URL (`https://github.com/o/r.git`, `git@github.com:o/r`, `https://github.com/o/r/tree/main`).
- Project colors are one of: gray, red, orange, yellow, green, teal, blue, purple, pink. NULL means the UI picks one from a hash of the slug.
- Titles have their whitespace squashed to single spaces and must not be empty. Descriptions are trimmed.
- Tags are split on commas, whitespace-squashed, lowercased and deduplicated, keeping their order.
- A task key looks like `api#12`, `#12` or `12`. When a slug prefix is given it must match the task's project.
- New tasks go to the bottom of the first column. The position is the floor of the column's highest position, plus 1.
- Changing a task's status moves it to the bottom of the new column.
- Moving a task (`status`, `before`) puts it just above `before`, at the midpoint between neighbours, or at the bottom when `before` is null. When the gap between neighbours drops below 1e-9, the column is renumbered 1, 2, 3 and so on. A move only bumps `updated_at` when the status changes.
- `next_task` takes the top task of the first column that is not marked human and moves it to the bottom of the second column.
- Search matches every word (at most 10) against `slug#id title description tags`, case-insensitive with escaped LIKE patterns. Open tasks come first, then the most recently updated. It returns at most 30 results. An empty query returns the most recently changed tasks.
- The board returns only the newest 50 done tasks unless it is asked for all of them, and reports how many it left out (`done_hidden`).
- A comment needs text, at least one image, or both. The author defaults to `me` from the UI and to the MCP client name, then `agent`, from MCP.
- An image file is deleted once no task or comment references it.

## CLI

| Command | Does |
| --- | --- |
| `kan-do` | Starts the board on the configured port and opens the browser. `--port`, `--no-open`. If kan-do is already running on that port, it just opens the browser. |
| `kan-do mcp` | MCP server on stdio. Agents start it. |
| `kan-do init [dir]` | Creates the data folder |
| `kan-do sync` | Commits, then pulls or pushes. `--repo <url>`, `--keep-local`, `--keep-remote`. |
| `kan-do export` | `-f markdown\|json`, `-p <project>`, `-o <file>` |
| `kan-do service install\|uninstall` | LaunchAgent on macOS, systemd user unit on Linux |
| `kan-do update` | Installs the latest release and restarts the service if there is one |
| `kan-do --version` | Prints the version |

Every command accepts `--home`.

## HTTP API

All JSON. Errors are `{"error": "..."}` with status 400 (invalid input), 404 (not found), 503 (sync holds the lock) or 500.

| Method and path | Body or query | Returns |
| --- | --- | --- |
| `GET /api/ping` | | `kan-do` as plain text, used to detect a running board |
| `GET /api/events` | | SSE stream, described below |
| `GET /api/board` | `?project=<slug\|all>&done=all` | `{changes, done_hidden, projects[], statuses[], tasks[], version}` |
| `GET /api/search` | `?q=` | `Task[]` |
| `POST /api/projects` | `{slug, name?, repo?}` | Project |
| `PATCH /api/projects/{slug}` | `{name?, color?}` | Project |
| `DELETE /api/projects/{slug}` | | Project as it was |
| `POST /api/projects/{slug}/repos` | `{repo}` | Project |
| `DELETE /api/projects/{slug}/repos` | `{repo}` | Project |
| `POST /api/tasks` | `{project, title, description?}` | Task |
| `GET /api/tasks/{id}` | | `{task, comments[], images[]}` |
| `PATCH /api/tasks/{id}` | `{title?, description?, status?, project?, tags?, human?}` | Task |
| `DELETE /api/tasks/{id}` | | Task as it was |
| `POST /api/tasks/{id}/move` | `{status, before?}` | Task |
| `POST /api/tasks/{id}/comments` | `{body, images[]}` | Comment |
| `DELETE /api/comments/{id}` | | Comment as it was |
| `POST /api/images` | raw image bytes with the image's Content-Type, up to 25 MB | `{file, path}` |
| `POST /api/tasks/{id}/images` | `{file}` | Image |
| `DELETE /api/images/{id}` | | Image as it was |
| `GET /images/{file}` | | The image, cached as immutable |
| `GET /api/update` | `?now=1` | `{current, latest, available}` |
| `POST /api/update` | | `{restarting: true}`, after which the process replaces itself |
| `GET /api/sync` | | `{remote}` |
| `PUT /api/sync` | `{url}` | `{remote}` |
| `POST /api/sync` | `{keep?: "local"\|"remote"}` | `{message}`, or 409 `{error, diverged: true, first}` |

Project: `{id, slug, name, repos[], color, created_at, open, total}`. Task: `{id, project, title, description, tags[], human, status, position, created_at, updated_at, comments}`. Image: `{id, task_id, comment_id, file}`.

### Live updates

`GET /api/events` is a Server-Sent Events stream. A single watcher goroutine runs while at least one client is connected. Every 500 ms it reads the change token, made of `meta.changes` and the database file's inode, and sends `event: change` with the token whenever the token differs from the last one. The inode is included so a database replaced by sync always counts as a change. Polling the token is the only way to see writes from `kan-do mcp` processes, and it costs one indexed read. The stream also sends a comment line every 25 s so proxies keep it open. The UI shows an offline banner while the stream is disconnected and reloads the board when it reconnects.

### Security

- Host check against DNS rebinding. On loopback, the board accepts only `localhost` and loopback IPs. When listening on the network it also accepts any IP and this machine's own hostnames (`name`, `name.local`). A Host header with a different port is refused.
- An `Origin` header, when present, must be `http://` plus the Host.
- Every response carries `Content-Security-Policy: default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`, plus `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, and `Cache-Control: no-cache` unless a handler sets its own.
- On loopback IPv4 the board also listens on `::1`, because some clients resolve `localhost` to IPv6 only.

## MCP

The server name is `kan-do`. It runs over stdio (`kan-do mcp`) and, when `mcp_http` is on, as streamable HTTP at `/mcp` behind the same host check. Replies are plain text.

Over stdio the server reads the git remotes of its working directory, with `origin` first. When an agent leaves out `project`, the server uses the project linked to one of those repos. Over HTTP the agent has to pass `project`. Error messages name the known projects and say how to fix the call.

| Tool | Arguments | Reply |
| --- | --- | --- |
| `list_projects` | | `slug "Name" (repos): N open of M`, with `<- current repo` on the matching line |
| `create_project` | `slug, name?, repo?` | `Created project api (API), linked to o/r.` |
| `link_repo` | `project, repo` | `Project api is linked to o/r, o/r2.` |
| `list_tasks` | `project?` (or `all`), `status?` (or `all`) | One `api#12 [todo] Title {tags} (2 comments)` per line. Human tasks are left out and counted. Done tasks are left out unless a status is given, and then only the newest 20 are listed. |
| `get_task` | `id` | Title, status, times, description, image paths and comments |
| `create_task` | `title, description?, project?, tags?` | `Created api#12 in todo: Title` |
| `update_task` | `id, title?, description?, status?, tags?` | `Updated api#12, todo -> review: Title` |
| `add_comment` | `id, body, author?` | `Comment added to api#12 as claude-code.` |
| `next_task` | `project?` (or `all`) | `Moved to doing:` followed by the full task |
| `delete_task` | `id` | `Deleted api#12 and its 2 comments: Title` |

`id` accepts a string or a number. Input schemas leave out `$schema` to keep the tool list small. The server instructions tell agents the task key format, the next_task, add_comment, update_task loop, and to leave human tasks alone.

## Sync

Runs `git` in the data folder.

- Setting a repo: try the URL. `owner/repo` expands to the GitHub SSH URL and then the HTTPS one, and the first that answers `git ls-remote` wins. On first setup, `git init` with the remote's default branch, then add or replace `origin`.
- Syncing: fetch, take the exclusive lock, settle the database (make it complete without its journal), `git add --all`, release the lock, then commit `kan-do sync from <hostname>`. If git has no user configured, the commit uses `kan-do <kan-do@localhost>`.
- If the local branch doesn't exist on the remote: push it when the remote has no branches, follow it when the remote has exactly one other branch, and fail when it has several.
- Ahead only: push. Behind only: fast-forward under the lock, then migrate. Both: return `Diverged{first}` unless `keep` is set. `keep=local` force-pushes with lease. `keep=remote` copies the database to `kan-do.db.bak`, then hard-resets under the lock and migrates.
- From the UI, git never prompts (`GIT_TERMINAL_PROMPT=0`, `ssh -o BatchMode=yes`), and failures are cut down to the one line that explains them.

## Update

- `GET https://api.github.com/repos/azmainadel/kan-do/releases/latest` through Go's HTTP client, cached for 24 hours unless `now` is set. `KANDO_RELEASES_URL` overrides the base URL for tests.
- Install: download the archive for this OS and architecture plus `checksums.txt`, check the SHA-256, write the new binary next to the current one, and rename it over the current one. On success, `exec` the new binary with the same arguments plus `--no-open`.
- This works wherever the binary's folder is writable, unlike an installer script that needs a `bin` folder.

## Service

- macOS: LaunchAgent `com.github.azmainadel.kan-do` running `kan-do --no-open --home <dir>`, with KeepAlive on failure and logs in `~/Library/Logs/kan-do.log`.
- Linux: systemd user unit `kan-do.service` with `Restart=on-failure`. After install it prints the `loginctl enable-linger` hint for headless machines.
- `kan-do update` restarts the service when one is installed.

## Export

Markdown and JSON with the same structure as the board: projects, then tasks in board order, each with its comments and image paths. Statuses that are no longer in the config come last.

## UI

A single-page app served from `/`. React Query owns all server state, and SSE change events invalidate the board query.

### Layout

- Sidebar with logo, sync and search buttons, a new-project form, "All projects" with its open count, and the project list. Each project shows a colored folder icon, its name, and its open count, which turns into a ⋯ button on hover.
- Header with the project name, the open count, and repo chips that link to GitHub.
- The board's filter bar (new, described below).
- Columns, one per status. The last column can collapse to a 48 px vertical strip, and each browser remembers that choice. The first column has the add button.
- The task panel slides in from the right at 500 px wide, full width on phones.
- Dialogs: search (⌘K), sync setup, and confirmations for destructive actions. These replace every `confirm()`.
- Below 700 px the sidebar becomes a horizontal project strip on top and columns are 78vw wide.
- Light and dark themes follow the system. Both use the brand palette from `assets/brand`: forest green `#1c462d`, sage `#849f7a`, amber `#f4ae1d` and cream `#faf7ed`. In light mode the cream is the page background, forest green is the primary accent, and amber marks highlights such as the focused card. Dark mode uses a deep green-black background with sage and amber as accents.
- The sidebar draws the logo inline, so its dark bar can turn light in dark mode. Favicons are `favicon.svg`, `favicon.ico` (16, 32 and 48 px) and `apple-touch-icon.png` (180 px), all in `ui/public`.

### Cards

Each card shows the title, `#id` (click copies `project#id` and shows a toast), a project dot in the All view, a human icon, a one-line plain-text preview of the description, and tags.

### Drag and drop (dnd-kit)

- Mouse, touch (long press) and keyboard sensors.
- A drag overlay with a lifted card, and animated reordering within and across columns.
- Dropping on the collapsed done strip works.
- Moves are optimistic. The card moves right away, the API call runs in the background, and on error the board rolls back and shows a toast.
- SSE refreshes are held back while a drag is in progress.

### Task panel

- The title is single-line. Enter or Tab moves to the description, ⌘↵ saves and closes, Esc reverts.
- Status select, tags input, human switch, and a project select that moves the task to another project. Created and updated times are relative, with the full time in a tooltip.
- The description has an Edit/Preview toggle. Preview renders GitHub-flavoured Markdown (task lists, tables, code, links) without raw HTML. Saving happens on blur.
- Images can be pasted or dropped onto the description (attached to the task) or the comment box (attached to the comment). Thumbnails open the full image, and × removes one.
- Comments are rendered as Markdown with their author and time, a delete button on hover, and a composer with a send button where ⌘↵ sends.
- Delete task, after a confirmation dialog.
- New task: "+" or `n` creates "New task" in the current project, or in the last one used when on All. The panel opens with the title selected. Closing it without changes deletes it, including when the page closes (`fetch` with `keepalive`).

### Search (cmdk)

Server search with a 120 ms debounce. The empty query shows "Recent". Rows show the project dot, title, key and status. Arrow keys and Enter open the task, switching project if needed.

### Filtering (new)

A filter bar above the columns, applied on the client to the loaded board:

- Text filter over title, description, tags and key.
- Tag filter: multi-select of the tags present on the board. A card must have all selected tags.
- Owner filter: All, Agent tasks, Human tasks.
- Project filter in the All view: multi-select.

Column counts show `shown/total` while a filter is on. A "Clear" button resets the filters. Esc in the text box clears it. Picking a single project drops the project filter, which only applies to All projects. Filters live in the URL so a filtered view can be shared: `#<project>/<task>?q=…&tags=a,b&owner=human&projects=x,y`. The done column filters only the done tasks it has loaded, and "Show N older" still loads the rest.

### Keyboard (new)

| Key | Action |
| --- | --- |
| `/` or ⌘K | Search |
| `n` | New task |
| `f` | Focus the filter text box |
| `←` `→` `↑` `↓` or `h` `l` `k` `j` | Move focus between cards and columns |
| `Enter` | Open the focused card |
| `Shift` + `←` `→` | Move the focused card to the previous or next column |
| `Shift` + `↑` `↓` | Move the focused card up or down within its column |
| `c` | Copy the focused card's key |
| `Esc` | Close the panel, dialog or menu, or clear the focused field |
| `?` | Shortcut help |

Focus rings are visible for keyboard users (`:focus-visible`). Dialogs trap focus and return it when they close.

### Other behaviour carried over

- The URL is `#project/taskId`. The last project lives in localStorage.
- Relative times refresh every minute.
- The sidebar shows the version, an update button when a newer release exists, and the sync repo.
- Update flow: confirm, `POST /api/update`, wait 1.5 s, poll `/api/update` until it answers, then reload.
- Sync conflicts: a dialog with two choices, "Use the remote board here" and "Overwrite the remote with this board", and different wording for a first sync.

## Tests

- `store`: every Rust store test ported (migrations idempotent, newer-schema compatibility, projects, repos, search, images, task lifecycle, cascade delete, ordering and renumbering, next_task, change counter, task keys, repo URLs).
- `home`: first open creates the files, bad statuses are rejected.
- `web`: host guard table, plus API handlers run through `httptest`.
- `mcp`: the reply format of each tool against a temporary data folder.
- `sync`: URL candidates and git error reasons, plus an end-to-end test against a bare local repo.
- `service`: the plist and unit file text, including path quoting.
- `ui`: Vitest for the move logic, filters and their address form, keyboard navigation and Shift moves, the plain-text preview, tags and times. A Playwright smoke test runs the real binary on a fresh data folder: create a project, add a task with `n`, drop an untouched new task, edit tags, comment, drag a card, see an agent's change arrive live, filter by text, tag and owner (and survive a reload), move around and move cards with the keyboard, search, delete.
- `make lint`, `make test` and `make e2e` run all of the above locally. CI runs the same, plus a cross-compile of every release target.

## Delivery

One reviewable change per phase:

1. Go module, `home`, `store` with migrations, tests.
2. HTTP API, SSE, MCP over stdio and HTTP, CLI, tests.
3. React UI with feature parity.
4. Filtering and keyboard navigation.
5. Sync, service, update, export.
6. GoReleaser, `install.sh`, CI, README.

## License

AGPL-3.0. See the open question below.

## Open questions

- Attribution. This is a port of an AGPL-3.0 project, so AGPL section 5(a) requires a notice that this is a modified version of that work, and the original copyright notices must stay. A `NOTICE` file is the least intrusive way to do it.
- Module path: `github.com/azmainadel/kan-do`.
