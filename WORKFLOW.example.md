---
tracker:
  kind: fizzy
  api_key: $FIZZY_TOKEN
  board_id: board-1
  active_states:
    - Todo
    - In Progress
  terminal_states:
    - Done
    - Not Now
polling:
  interval_ms: 5000
workspace:
  root: ~/code/fizel-workspaces
hooks:
  before_run: |
    if [ -f package.json ]; then
      npm install
    fi
agent:
  max_concurrent_agents: 4
  max_turns: 5
codex:
  command: codex app-server
  approval_policy: never
  thread_sandbox: workspace-write
---

You are working on tracker item `{{ issue.identifier }}`.

Title: {{ issue.title }}
State: {{ issue.state }}
URL: {{ issue.url }}

Description:
{{ issue.description }}
