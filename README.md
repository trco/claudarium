# claudarium

A tiny local web UI to view and edit your Claude Code config. Runs on
localhost — no auth, no database — reading and writing the real files in
`~/.claude`.

## What it does

**Edit**
- **Memory** (`~/.claude/CLAUDE.md`) — editor with live markdown preview, a
  diff before saving, and a timestamped backup written first (a failed backup
  aborts the write).

**Inspect (read-only)**
- **Capabilities** — every agent, skill and command across global config,
  plugins and repos. Repos come from `~/.claude.json` plus any sibling repo
  with a `.claude` dir found by scanning their parent dirs, so repos you've
  never opened in Claude still show. Add scan roots with `--scan ~/a,~/b`.
  Click a row for details + Reveal in Finder.
- **Plugins** — installed plugins and known marketplaces.
- **MCP** — configured servers (global + per-repo). Env values are hidden;
  only variable names show. Missing repos are flagged `stale`.
- **Health** — duplicate permission rules, missing paths, plugins
  enabled-but-not-installed, MCP commands not on `PATH`, stale repo references.

Every table has per-column filters, click-to-sort columns and a live count;
Capabilities can group by source. Dark-mode toggle in the header.

`settings.json` is read-only — used only for the Plugins view's
enabled/disabled state.

## Run

```sh
go run github.com/trco/claudarium/cmd/app@latest --open
```

Serves on http://localhost:8787 and edits the real files in `~/.claude`.
Flags: `--addr` listen address · `--open` launch browser · `--scan a,b` extra
dirs to scan for repos.

> Needs **Go ≥ 1.23** on recent macOS.

## Not yet

Project-level config editing, enabling/disabling plugins or MCPs, adding MCP
servers, and hooks / statusline / keybindings editors.
