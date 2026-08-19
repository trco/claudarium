# Claudarium

A tiny local web UI to view and edit your Claude Code setup. Runs on
localhost — no auth, no database — reading and writing the real files in
`~/.claude`.

## Quick start

```sh
GOPROXY=direct go run github.com/trco/claudarium/cmd/app@master --open
```

Opens http://localhost:8787. Needs Go installed; the module pins Go 1.23,
which the toolchain fetches automatically if your local Go is older.

## Tabs

| Tab | What it shows |
| --- | --- |
| Memory | Your global `~/.claude/CLAUDE.md`, edited with a live preview. Each save shows a diff and writes a timestamped `.bak` backup first. |
| Capabilities | Every agent, skill and command Claude can use, across global config, plugins and repos. |
| Plugins | Installed plugins — description, author and what each contributes. |
| Marketplaces | The plugin marketplaces Claude knows about, and how many plugins you've installed from each. |
| MCP | Configured MCP servers, global and per-repo. Env values are hidden — only variable names show. |
| Doctor | Config health checks: shadowed capabilities, plugins enabled but not installed or from an unknown marketplace, MCP commands off `PATH`, duplicate MCP names, stale repos, malformed settings. |
| Settings | Hooks, statusLine, model and a permissions summary. |

Every table has per-column filters, click-to-sort and a live count. The header
**Search** box looks across every tab at once. Click a row for full details,
then **Reveal in Finder** or **View raw**. A dark-mode toggle sits in the header.

## Editing

Most tabs are read-only. The few writable actions back the file up first:

| Action | Writes |
| --- | --- |
| Edit Memory (diff shown before saving) | `~/.claude/CLAUDE.md` |
| Enable / disable a plugin | `settings.json` |
| Turn an MCP server on / off | `~/.claude.json` (nothing is deleted — a disabled server is kept and restored on re-enable) |

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--addr` | `127.0.0.1:8787` | Listen address (keep it on localhost — it edits real config). |
| `--open` | off | Open the app in your browser on start. |
| `--scan a,b` | — | Extra dirs to scan one level deep for repos with a `.claude` dir. |

## Roadmap

Not yet: project-level config editing, adding MCP servers, and hooks /
statusLine / keybindings editors.
