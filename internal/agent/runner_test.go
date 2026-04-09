package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmurray/fizel/internal/config"
	"github.com/gmurray/fizel/internal/model"
	"github.com/gmurray/fizel/internal/workspace"
)

func TestRunnerExecutesSingleTurnPerRun(t *testing.T) {
	root := t.TempDir()
	traceFile := filepath.Join(root, "trace.jsonl")
	scriptPath := filepath.Join(root, "fake-codex.sh")
	script := `#!/bin/sh
trace_file="` + traceFile + `"
count=0
while IFS= read -r line; do
  count=$((count + 1))
  printf '%s\n' "$line" >> "$trace_file"
  case "$count" in
    1)
      printf '%s\n' '{"id":1,"result":{}}'
      ;;
    2)
      ;;
    3)
      printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-1"}}}'
      ;;
    4)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-1"}}}'
      printf '%s\n' '{"method":"turn/completed"}'
      exit 0
      ;;
  esac
done
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex script: %v", err)
	}

	settings := config.Settings{
		Workspace: config.WorkspaceSettings{Root: filepath.Join(root, "workspaces")},
		Hooks:     config.HookSettings{TimeoutMS: 1000},
		Agent:     config.AgentSettings{MaxTurns: 5},
		Tracker: config.TrackerSettings{
			Kind:    "fizzy",
			APIKey:  "token",
			BoardID: "board-1",
		},
		Codex: config.CodexSettings{
			Command:        scriptPath,
			ApprovalPolicy: "never",
			ThreadSandbox:  "workspace-write",
		},
	}

	runner := New(settings, workspace.New(settings))
	_, err := runner.Run(context.Background(), model.Item{
		ID:         "board-1:42",
		Identifier: "board-1:42",
		Title:      "Test item",
		State:      "In Progress",
	}, "Handle it", nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	raw, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	if strings.Count(string(raw), `"method":"turn/start"`) != 1 {
		t.Fatalf("expected exactly one turn/start, trace was:\n%s", string(raw))
	}
}
