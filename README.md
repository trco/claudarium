# Claudarium

A tiny local web UI to view and edit your Claude Code config. Runs on
localhost — no auth, no database — reading and writing the real files in
`~/.claude`.

## Quick start

```sh
go run github.com/trco/claudarium/cmd/app@latest --open
```

Opens http://localhost:8787. Needs Go installed; the module pins Go 1.23,
which the toolchain fetches automatically if your local Go is older (older
versions crash with `missing LC_UUID` on recent macOS).

## Features

| View | Mode | What it shows |
| --- | --- | --- |
| Memory | edit | Your global `~/.claude/CLAUDE.md`, with live markdown preview. Each save shows a diff and writes a timestamped `.bak` backup first. |
| Capabilities | read-only | Every agent, skill and command Claude can use, across global config, plugins and repos. Click a row for details + Reveal in Finder. |
| Plugins | read-only | Plugins installed from your marketplaces. |
| Marketplaces | read-only | The plugin marketplaces Claude knows about, and where each is sourced from. |
| MCP | read-only | Configured MCP servers, global and per-repo. Env values are hidden — only variable names show. |
| Doctor | read-only | Health checks: duplicate permission rules, missing paths, plugins enabled but not installed, MCP commands off `PATH`, and stale repos. |

Capabilities and MCP also pick up sibling repos with a `.claude` dir — even
ones you've never opened in Claude. Add more roots with `--scan ~/a,~/b`.

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--addr` | `127.0.0.1:8787` | Listen address (keep it on localhost — it edits real config). |
| `--open` | off | Open the app in your browser on start. |
| `--scan a,b` | — | Extra dirs to scan one level deep for repos with a `.claude` dir. |

## Notes

It writes to your real config. Only Memory is editable, and it always backs up
first. `settings.json` is read-only (used just for the Plugins view's
enabled/disabled state); everything else is inspection only.

## Roadmap

Not yet: project-level config editing, enabling/disabling plugins or MCPs,
adding MCP servers, and hooks / statusline / keybindings editors.
