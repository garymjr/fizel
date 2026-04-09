package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmurray/fizel/internal/config"
	"github.com/gmurray/fizel/internal/model"
)

func TestCreateForItemPassesRepoMetadataToHooks(t *testing.T) {
	root := t.TempDir()
	traceFile := filepath.Join(root, "trace.txt")
	manager := New(config.Settings{
		Workspace: config.WorkspaceSettings{Root: filepath.Join(root, "workspaces")},
		Hooks: config.HookSettings{
			TimeoutMS: 1000,
			AfterCreate: "printf '%s|%s|%s|%s' " +
				`"$SOURCE_REPO_URL" "$SOURCE_REPO_PATH" "$SOURCE_REPO_KEY" "$SOURCE_WORKFLOW_PATH"` +
				" > " + traceFile,
		},
		Repo: config.RepoSettings{
			Key:          "api",
			Path:         "/tmp/api",
			WorkflowPath: "/tmp/api/WORKFLOW.md",
		},
	})

	if _, err := manager.CreateForItem(model.Item{Identifier: "board-1:42"}, ""); err != nil {
		t.Fatalf("CreateForItem() error = %v", err)
	}

	raw, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "/tmp/api|/tmp/api|api|/tmp/api/WORKFLOW.md") {
		t.Fatalf("unexpected hook env %q", got)
	}
}
