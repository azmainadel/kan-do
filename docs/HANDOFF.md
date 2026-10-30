# Handoff

Notes for whoever picks up kan-do next. Read `docs/SPEC.md` first: it's the full description of what kan-do does and how it's built.

## Where things stand

Work happens on `develop`. `main` is the release branch: merging a pull request into it publishes a release. `v0.1.0` is out, and the repo is still private.

| Phase | What | State |
| --- | --- | --- |
| 1 | Go module, `store` (SQLite, migrations), `home` (data folder, config, lock) | Done |
| 2 | HTTP API, live updates over SSE, MCP over stdio and HTTP, CLI | Done |
| 3 | React UI built into the binary | Done |
| 4 | Filter bar and keyboard navigation | Done |
| 5 | Git sync, self-update, `service install`, `export` | Done |
| 6 | Releases, install script, CI, README | Done, `v0.1.0` released |

### Phase 6: what's done

- `LICENSE`: AGPL-3.0.
- `.goreleaser.yaml`: builds `kando` for linux and darwin on amd64 and arm64, plus linux armv7. Archives are named `kando_<version>_<os>_<arch>.tar.gz` next to a `checksums.txt`, which is what `internal/update` and `install.sh` expect. `install.sh` is attached to each release.
- `install.sh`: downloads the right archive, checks its SHA-256, and installs to `~/.local/bin` (or `$KANDO_INSTALL_DIR`). `$KANDO_VERSION` picks a version and `$KANDO_DOWNLOAD_URL` points it at another server, for testing.
- `goreleaser check` and `goreleaser release --snapshot --clean` pass. The archive names in `dist/` match `update.Archive()`, including `armv7`. GoReleaser v2.19 needs Go 1.27 to install, and on macOS it must be built without cgo: `CGO_ENABLED=0 GOTOOLCHAIN=auto go install github.com/goreleaser/goreleaser/v2@v2.19.0`.
- `install.sh` was tested against the snapshot served from `dist/`. It installs a working `kando`, and refuses an archive with a bad checksum. This found and fixed a bug: `$arch…` broke under `set -u`, because the shell read the ellipsis as part of the name.
- `.github/workflows/ci.yml` runs `make lint`, `make test` and `make e2e` on every push and pull request, and cross-compiles every release target with `CGO_ENABLED=0`.
- `.github/workflows/release.yml` releases on every pull request merged into `main`. The PR title picks the version: `feat:` bumps the minor version, `!` before the colon or `BREAKING CHANGE` bumps the major one, and anything else bumps the patch. `[skip release]` in the title skips the release. The workflow tags the merge commit, then runs GoReleaser v2.19.0. Pushing a `v*` tag by hand still releases that tag. `.github/pull_request_template.md` explains the title rules.
- The README is rewritten for people who just want to use kan-do.
- staticcheck is pinned to v0.8.2 in the `Makefile`.
- `make lint`, `make test` and `make e2e` all pass after the rename to `kando`.

### Phase 6: what's left

1. **Make the repo public.** Until then the `curl | sh` line and `kando update` get a 404, because they use GitHub without a login. To install while it's private:

   ```sh
   gh release download -R azmainadel/kan-do -p 'kando_*_darwin_arm64.tar.gz'
   tar -xzf kando_*_darwin_arm64.tar.gz kando && mv kando ~/.local/bin/
   ```

2. **Check the release-on-merge workflow** the first time a PR is merged into `main`.

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

## Decisions

- **Attribution.** kan-do is AGPL-3.0, and so is the project its design and code were ported from. AGPL section 5(a) asks for a notice that this is a modified version of that work. The user decided to ship without any notice.

## Not tested on real systems

- `kando service install` and `uninstall` need a real systemd user session or launchd. Only the generated unit and plist files are tested.
- A full `kando update` needs a public release. The download, checksum check, extraction and binary swap are each tested against a fake release server.
