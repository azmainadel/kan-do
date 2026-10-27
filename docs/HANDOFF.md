# Handoff

Notes for whoever picks up kan-do next. Read `docs/SPEC.md` first: it's the full description of what kan-do does and how it's built.

## Where things stand

All work is on the branch `feat/go-rewrite`. Nothing is merged to `main` yet, and no release has been made.

| Phase | What | State |
| --- | --- | --- |
| 1 | Go module, `store` (SQLite, migrations), `home` (data folder, config, lock) | Done |
| 2 | HTTP API, live updates over SSE, MCP over stdio and HTTP, CLI | Done |
| 3 | React UI built into the binary | Done |
| 4 | Filter bar and keyboard navigation | Done |
| 5 | Git sync, self-update, `service install`, `export` | Done |
| 6 | Releases, install script, CI, README | In progress |

### Phase 6: what's done

- `LICENSE`: AGPL-3.0.
- `.goreleaser.yaml`: builds `kando` for linux and darwin on amd64 and arm64, plus linux armv7. Archives are named `kando_<version>_<os>_<arch>.tar.gz` next to a `checksums.txt`, which is what `internal/update` and `install.sh` expect. `install.sh` is attached to each release.
- `install.sh`: downloads the right archive, checks its SHA-256, and installs to `~/.local/bin` (or `$KANDO_INSTALL_DIR`). `$KANDO_VERSION` picks a version and `$KANDO_DOWNLOAD_URL` points it at another server, for testing.

### Phase 6: what's left

1. **Check the release config.** Run `goreleaser check`, then `goreleaser release --snapshot --clean`, and confirm the archive names in `dist/` match `update.Archive()` in `internal/update/update.go`. GoReleaser v2.19 needs Go 1.27 to install: `GOTOOLCHAIN=auto go install github.com/goreleaser/goreleaser/v2@v2.19.0`. The user stopped this step once; ask before running it.
2. **Test `install.sh` against the snapshot.** Serve `dist/` locally, run it with `KANDO_DOWNLOAD_URL` and `KANDO_VERSION` set, and confirm it installs a working `kando`.
3. **Add CI** in `.github/workflows/`:
   - `ci.yml` on pushes and pull requests: `make lint`, `make test`, and `make e2e` (needs `npx playwright install --with-deps chromium`). Also cross-compile every release target with `CGO_ENABLED=0`.
   - `release.yml` on `v*` tags: GoReleaser with Go and Node set up, and `contents: write` permission.
4. **Rewrite the README** for people who just want to use kan-do. The user asked for it to be reader friendly and short: what it is, install, run, connect an agent (`claude mcp add kando --scope user -- kando mcp`), sync between machines, a few commands, and the `config.toml` settings. Leave out internals. Keep the banner (`docs/img/banner.webp`) at the top. Don't mention where the design came from.
5. **Pin staticcheck** in the `Makefile` (it uses `@latest`) to `honnef.co/go/tools/cmd/staticcheck@v0.8.2`.
6. **Rerun every check after the rename to `kando`:** `make lint`, `make test`, `make e2e`. The Go tests passed after the rename. The UI's end-to-end tests have not been rerun since.

## Rules the user set

- **Naming.** `kan-do` is the product and repo name only, used in sentences and the UI wordmark. Everything a person types or the program uses is `kando`: the command, the binary, the data folder `~/kando`, `kando.db`, `kando.lock`, `kando.service`, the LaunchAgent label, localStorage keys, and the MCP server name. The Go module path stays `github.com/azmainadel/kan-do`, because that's the repo.
- **No reference to the project kan-do was ported from.** Not in code, the UI, the README or commit messages.
- **Commits.** Make small, logical commits. Author them as `Azmain Adel <azmainadel47@gmail.com>`, and add no AI co-author or session lines. The container's git identity is set to something else, so pass it per commit:
  `git -c user.name="Azmain Adel" -c user.email="azmainadel47@gmail.com" commit -m "..."`
  Message style: `area: what changed`, for example `web: add sync and update routes`.
- **Commit and push only when asked.** The user said when to commit and push at each step.
- **Writing.** Plain words, short sentences, sentence-case headings, no em dashes. This applies to docs, UI text and the README.
- **Code.** Match the surrounding style: comments only where the reason isn't obvious, written as plain sentences.

## Working in the repo

```sh
make            # build the UI, then the binary (./kando)
make dev        # UI with hot reload; run `./kando --no-open` alongside
make lint       # go vet, staticcheck, gofmt, eslint, tsc
make test       # go test -race, vitest
make e2e        # Playwright against the real binary on a fresh data folder
```

- **Go 1.26.** `go.mod` pins the `go1.26.10` toolchain and the `Makefile` exports `GOTOOLCHAIN`, so Go downloads it as needed. Without the Makefile, run `export GOTOOLCHAIN=go1.26.10` first.
- **The UI** builds into `internal/web/dist/app`, which is git-ignored and embedded with `go:embed`. Without a UI build the server serves `internal/web/placeholder.html`.
- **`ui/go.mod`** only keeps Go tools out of `node_modules`.
- **`staticcheck.conf`** turns off ST1005, because kan-do's errors are sentences shown to people.

## Layout

```
cmd/kando/          CLI (cobra)
internal/store/     SQLite schema, migrations, every query
internal/home/      data folder, config.toml, file lock
internal/web/       JSON API, SSE, host guard, sync and update routes, embedded UI
internal/mcp/       MCP tools (stdio and HTTP)
internal/gitsync/   git sync
internal/update/    release check and self-update
internal/service/   launchd and systemd
internal/export/    Markdown and JSON export
ui/                 React 19, TypeScript, Vite, Tailwind 4
assets/brand/       logo SVGs (the masters for every icon)
docs/               SPEC.md, this file, README images
```

## Open questions for the user

- **Attribution.** kan-do is AGPL-3.0, and so is the project its design and code were ported from. AGPL section 5(a) requires a notice that this is a modified version of that work. The user asked for no mention of that project anywhere. These two conflict, and the user hasn't decided yet. Raise it again before the first public release.

## Not tested on real systems

- `kando service install` and `uninstall` need a real systemd user session or launchd. Only the generated unit and plist files are tested.
- A full `kando update` needs a published release. The download, checksum check, extraction and binary swap are each tested against a fake release server.
