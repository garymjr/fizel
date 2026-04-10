package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmurray/fizel/internal/config"
	"github.com/gmurray/fizel/internal/model"
)

func TestCreateForItemCreatesGitWorktreeFromRepo(t *testing.T) {
	root := t.TempDir()
	repoPath := initGitRepo(t, filepath.Join(root, "repo"))
	manager := New(config.Settings{
		Workspace: config.WorkspaceSettings{Root: filepath.Join(root, "workspaces")},
		Hooks:     config.HookSettings{TimeoutMS: 1000},
		Repo: config.RepoSettings{
			Key:          "api",
			Path:         repoPath,
			WorkflowPath: filepath.Join(repoPath, "WORKFLOW.md"),
		},
	})

	path, err := manager.CreateForItem(model.Item{Identifier: "board-1:42"}, "")
	if err != nil {
		t.Fatalf("CreateForItem() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		t.Fatalf("expected worktree git metadata: %v", err)
	}
	raw, err := exec.Command("git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("read worktree branch: %v: %s", err, strings.TrimSpace(string(raw)))
	}
	if got := strings.TrimSpace(string(raw)); got != "fizel/board-1_42" {
		t.Fatalf("expected worktree branch fizel/board-1_42, got %q", got)
	}
	tracked, err := os.ReadFile(filepath.Join(path, "tracked.txt"))
	if err != nil {
		t.Fatalf("read tracked file: %v", err)
	}
	if strings.TrimSpace(string(tracked)) != "base" {
		t.Fatalf("expected tracked file from repo, got %q", string(tracked))
	}
}

func TestCreateForItemPassesRepoMetadataToRunHooks(t *testing.T) {
	root := t.TempDir()
	repoPath := initGitRepo(t, filepath.Join(root, "repo"))
	traceFile := filepath.Join(root, "trace.txt")
	manager := New(config.Settings{
		Workspace: config.WorkspaceSettings{Root: filepath.Join(root, "workspaces")},
		Hooks: config.HookSettings{
			TimeoutMS: 1000,
			BeforeRun: "printf '%s|%s|%s|%s' " +
				`"$SOURCE_REPO_URL" "$SOURCE_REPO_PATH" "$SOURCE_REPO_KEY" "$SOURCE_WORKFLOW_PATH"` +
				" > " + traceFile,
		},
		Repo: config.RepoSettings{
			Key:          "api",
			Path:         repoPath,
			WorkflowPath: filepath.Join(repoPath, "WORKFLOW.md"),
		},
	})

	path, err := manager.CreateForItem(model.Item{Identifier: "board-1:42"}, "")
	if err != nil {
		t.Fatalf("CreateForItem() error = %v", err)
	}
	if err := manager.RunBefore(path, ""); err != nil {
		t.Fatalf("RunBefore() error = %v", err)
	}

	raw, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, repoPath+"|"+repoPath+"|api|"+filepath.Join(repoPath, "WORKFLOW.md")) {
		t.Fatalf("unexpected hook env %q", got)
	}
}

func TestRemoveDeletesGitWorktree(t *testing.T) {
	root := t.TempDir()
	repoPath := initGitRepo(t, filepath.Join(root, "repo"))
	manager := New(config.Settings{
		Workspace: config.WorkspaceSettings{Root: filepath.Join(root, "workspaces")},
		Hooks:     config.HookSettings{TimeoutMS: 1000},
		Repo: config.RepoSettings{
			Key:  "api",
			Path: repoPath,
		},
	})

	path, err := manager.CreateForItem(model.Item{Identifier: "board-1:42"}, "")
	if err != nil {
		t.Fatalf("CreateForItem() error = %v", err)
	}
	if err := manager.Remove(path, ""); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected worktree removal, stat err = %v", err)
	}
	raw, err := exec.Command("git", "-C", repoPath, "worktree", "list", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatalf("list worktrees: %v: %s", err, strings.TrimSpace(string(raw)))
	}
	if strings.Contains(string(raw), path) {
		t.Fatalf("expected worktree list to omit %q, got %q", path, string(raw))
	}
}

func initGitRepo(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runGit(t, path, "init")
	runGit(t, path, "config", "user.name", "Test User")
	runGit(t, path, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(path, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	runGit(t, path, "add", "tracked.txt")
	runGit(t, path, "commit", "-m", "init")
	return path
}

func runGit(t *testing.T, path string, args ...string) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", path}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
}
