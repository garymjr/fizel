# Fizel

Fizel is a Go rewrite of Symphony with Fizzy as the primary tracker backend.

Current scope:

- Global config at `~/.config/fizel/config.yaml`
- Per-repo `WORKFLOW.md` loading for watched repos
- `WORKFLOW.md` front matter + Markdown prompt contract
- Fizzy and memory tracker adapters
- Workspace lifecycle hooks
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
  hooks:
    after_create: |
      git clone --depth 1 "$SOURCE_REPO_URL" .
watched_repos:
  - key: api
    path: ~/code/api
  - key: web
    path: ~/code/web
```

Each watched repo must have a root `WORKFLOW.md`. In watched-repo mode, only `tracker:` front matter is read from that workflow file and applied on top of `tracker_defaults`; everything in `settings` is global source of truth. Fizzy cards must include exactly one repo label in the form `repo:<key>`.

One-off single workflow runs still work with `fizel -workflow /path/to/WORKFLOW.md`.
