# Fizel

Fizel is a Go rewrite of Symphony with Fizzy as the primary tracker backend.

Current scope:

- Global config at `~/.config/fizel/config.yaml`
- Per-repo `WORKFLOW.md` loading for watched repos
- `WORKFLOW.md` front matter + Markdown prompt contract
- Fizzy and memory tracker adapters
- Git worktree-based workspaces
- Codex app-server integration
- Polling orchestrator
- Terminal observability

Web dashboard/API parity is not implemented yet.

Default startup reads `~/.config/fizel/config.yaml`.

Example:

```yaml
tracker_defaults:
  kind: fizzy
  api_key: $FIZZY_TOKEN
settings:
  workspace:
    root: ~/code/fizel-workspaces
watched_repos:
  - key: api
    path: ~/code/api
  - key: web
    path: ~/code/web
```

Each watched repo must have a root `WORKFLOW.md`. In watched-repo mode, `tracker:` and `hooks:` front matter are read from that workflow file. `tracker:` applies on top of `tracker_defaults`. New workspaces are created as git worktrees rooted at the watched repo, so `hooks.after_create` is no longer supported. Remaining hooks are for run lifecycle customization only and must be defined in each repo `WORKFLOW.md`; `config.settings.hooks` is not supported. Other runtime settings still come from global `settings`. Fizzy cards must include exactly one repo label in the form `repo:<key>`.

One-off single workflow runs still work with `fizel -workflow /path/to/WORKFLOW.md`.
