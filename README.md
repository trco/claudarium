# claudarium

A tiny local web UI to view and edit your Claude Code config. Go + Fiber +
HTMX + Alpine + Tailwind (stack borrowed from Groopa). Runs on localhost, no
auth, no database — it reads and writes the real files in `~/.claude`.

## What it does (v1)

**Edit (global):**
- **Memory** — `~/.claude/CLAUDE.md`: editor with live markdown preview. Shows a
  **diff before saving** and writes a timestamped `.bak.<unix>` backup next to
  the file first — a failed backup aborts the write.

Settings (`settings.json`) editing was intentionally removed; the file is only
read (for the Plugins view's enabled/disabled state).

**Read-only inspection:**
- **Capabilities** — every agent / skill / command across global config,
  installed plugins, and repos. Repos come from `~/.claude.json` **plus** any
  sibling repo with a `.claude` dir found by scanning those repos' parent dirs
  (so repos you've never opened in Claude still show). Add more scan roots with
  `--scan ~/a,~/b`. Click a row for details + **Reveal in Finder**.
- **Plugins** — installed plugins + known marketplaces.
- **MCP** — configured servers (global + per-repo). Env **values are hidden**;
  only variable names are shown. Repos that no longer exist are flagged `stale`.
- **Health** — duplicate permission rules, missing paths, plugins
  enabled-but-not-installed, MCP commands not on `PATH`, stale repo references.

Every table has **per-column filters** (text + dropdowns), **click-to-sort**
columns, and a live count. Capabilities can **group by source**. There's a
**dark mode** toggle (◑, persisted) in the header.

## Install & run

The binary is self-contained (templates, CSS and JS are embedded), so once the
repo is public any of these is a single command:

```sh
# Run without installing (needs Go):
go run github.com/trco/claudarium/cmd/app@latest --open

# Install a binary onto your PATH (needs Go):
go install github.com/trco/claudarium/cmd/app@latest
claudarium --open

# No Go needed, via Homebrew (after the tap is set up):
brew install trco/tap/claudarium
claudarium --open
```

All of them serve on http://localhost:8787 and edit the real files in `~/.claude`.
Flags: `--addr` (listen address), `--open` (launch browser), `--scan a,b`
(extra dirs to scan for repos), `--dev` (read assets from disk for hot-reload).

> On recent macOS, `go run`/`go install` need **Go ≥ 1.23** (older versions hit
> the `LC_UUID` linker issue below). On Go 1.22.x, use Homebrew or `make` instead.

### From a checkout

```sh
make run     # build CSS, build+sign binary, run, open browser
make dev     # live-reload via air (uses --dev)
make test    # tests
make build   # release binary → ./claudarium
```

`make` targets need the `tailwindcss` standalone binary on PATH (and `air` for
`make dev`). `web/static/css/app.css` is committed so `go run`/`go install`
work without Tailwind.

## Releasing (prebuilt binaries + Homebrew)

Cross-platform binaries and a Homebrew formula are published by
[GoReleaser](https://goreleaser.com) via GitHub Actions on tag push:

```sh
git tag v0.1.0 && git push origin v0.1.0
```

One-time setup: push this repo to `github.com/trco/claudarium`, create a
`github.com/trco/homebrew-tap` repo, and add a `HOMEBREW_TAP_GITHUB_TOKEN`
secret (write access to the tap) for the formula publish step.

## Note on the Go version

Go 1.22.4 on macOS 26 (Darwin 25) needs two build fixups, both baked into the
Makefile (`make bin`/`make build`), so `make run` / `air` just work:

1. The internal linker omits `LC_UUID`, which dyld rejects at exec
   (`missing LC_UUID load command`) → link with the system linker
   (`CGO_ENABLED=1 -ldflags=-linkmode=external`).
2. The external linker then leaves an **invalid ad-hoc signature**, which Apple
   Silicon SIGKILLs the instant it runs — the process never binds, so you see
   *connection refused* → re-sign ad-hoc (`codesign -f -s -`).

Drop both once you're on a Go that links a valid `LC_UUID` internally (≥ 1.23).

## Not yet (deferred)

Editing project-level config, enabling/disabling plugins or MCPs, adding MCP
servers, and hooks / statusline / keybindings editors — all read-only or
global-only for now.
