<p align="center">
  <img alt="kan-do: a tiny board for you and your agents" src="docs/img/banner.webp" width="840">
</p>

kan-do is a kanban board for you and your coding agents. You plan work on the board in your browser. Your agents read the same board through MCP, pick up tasks, move cards and leave comments as they work. You watch it happen live.

It's one small program. The whole board is one folder on your machine, and a private git repo can keep it in step on every computer you use. There's no account, no cloud service and nothing to host.

## What you get

### A board that's quick to use

- Projects in a sidebar, each with its own color and open-task count, plus an "All projects" view.
- Columns you choose: `todo`, `doing`, `review` and `done` by default. The done column folds away when you don't need it.
- Drag and drop with mouse, touch or keyboard.
- Cards with a title, a Markdown description, tags and comments. Paste or drop images straight into a task or a comment.
- Search across every project with `/` or ⌘K.
- Filters by text, tag, project, and agent or human tasks. Filters stay in the address, so you can bookmark a view.
- Keyboard control for almost everything (see [shortcuts](#keyboard-shortcuts)).
- Light and dark themes that follow your system, and a layout that works on a phone.

### A shared to-do list for your agents

- Agents connect over MCP, the standard way coding agents use tools. Claude Code, Cursor, Codex and any other MCP client work.
- Link a project to one or more git repos. An agent working in that repo uses that project on its own, so you don't have to tell it where to look.
- Agents can list tasks, create them, take the next one, update them and comment on them. A typical loop: the agent takes the top task, moves it to `doing`, works on it, leaves a comment saying what it did, and moves it to `review` for you.
- Mark a task as human, and agents leave it alone.
- Every change an agent makes shows up on the board right away. You don't need to reload.

### Your data, on your machines

- Everything is in `~/kando`: one SQLite file, your images and a settings file.
- Sync between computers through any private git repo, like GitHub. If two machines changed the board, kan-do asks which one to keep and saves a backup of the other.
- Export the whole board as Markdown or JSON whenever you want.
- Runs on macOS and Linux, including a Raspberry Pi.
- `kando update` installs new releases for you.

## Install

On macOS or Linux:

```sh
curl -fsSL https://github.com/azmainadel/kan-do/releases/latest/download/install.sh | sh
```

This puts `kando` in `~/.local/bin`. Set `KANDO_INSTALL_DIR` to install it somewhere else, or `KANDO_VERSION` to pick a version.

To update later, run `kando update`.

## Run

```sh
kando
```

The board opens at http://localhost:4242. Your data is kept in `~/kando`.

To start the board every time you log in:

```sh
kando service install
```

## Connect an agent

For Claude Code:

```sh
claude mcp add kando --scope user -- kando mcp
```

Other agents work the same way: run `kando mcp` as a stdio MCP server. If your agent can only reach MCP over HTTP, use `http://localhost:4242/mcp`.

When an agent runs inside a git repo, kan-do picks the project linked to that repo. Link a repo from the project menu on the board, or ask your agent to do it. Then try telling your agent:

> Take the next task from kan-do and work on it.

## Keyboard shortcuts

| Key | What it does |
| --- | --- |
| `/` or ⌘K | Search |
| `n` | New task |
| `f` | Filter the board |
| Arrow keys or `h` `j` `k` `l` | Move between cards |
| `Enter` | Open the card |
| `Shift` + arrow keys | Move the card to another column, or up and down |
| `c` | Copy the card's key, like `api#12` |
| `Esc` | Close or clear |
| `?` | Show all shortcuts |

## Sync between machines

kan-do can keep your board in a private git repo, so every machine has the same board.

1. Create an empty private repo, for example `you/kando-data`.
2. Press the sync button on the board and paste the repo URL, or run:

   ```sh
   kando sync --repo you/kando-data
   ```

3. Do the same on your other machines.

After that, press sync or run `kando sync` to pull and push changes. If both machines changed the board since the last sync, kan-do asks which board to keep. The board it replaces is saved as `kando.db.bak`.

## Commands

| Command | What it does |
| --- | --- |
| `kando` | Start the board and open it |
| `kando --no-open` | Start the board without opening the browser |
| `kando mcp` | Run the MCP server for an agent |
| `kando sync` | Sync the board with its git repo |
| `kando export` | Write the board as Markdown, or JSON with `--format json` |
| `kando update` | Install the latest release |
| `kando service install` | Start the board at login |
| `kando service uninstall` | Stop starting it at login |

Run `kando --help` for every flag.

## Settings

Settings are in `~/kando/config.toml`:

```toml
# Port for the board.
port = 4242

# 127.0.0.1 keeps the board on this machine.
# "0.0.0.0" opens it to other devices on your network.
listen = "127.0.0.1"

# Board columns, left to right. New tasks go in the first one,
# and tasks in the last one count as done.
statuses = ["todo", "doing", "review", "done"]

# Also serve MCP over HTTP at /mcp.
mcp_http = true
```

The board has no logins. If you set `listen = "0.0.0.0"`, anyone who can reach the port can read and change your board.

## License

[AGPL-3.0](LICENSE)
