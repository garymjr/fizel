package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gmurray/fizel/internal/workflow"
)

func TestFromLoadedAppliesFizzyFallbacks(t *testing.T) {
	t.Setenv("FIZZY_TOKEN", "token-123")
	loaded := workflow.Loaded{
		Config: map[string]any{
			"tracker": map[string]any{
				"kind":     "fizzy",
				"api_key":  "$FIZZY_TOKEN",
				"board_id": "board-1",
			},
		},
	}
	settings, err := FromLoaded(loaded)
	if err != nil {
		t.Fatalf("FromLoaded() error = %v", err)
	}
	if settings.Tracker.APIKey != "token-123" {
		t.Fatalf("expected env-backed API key, got %q", settings.Tracker.APIKey)
	}
}

func TestResolvePathExpandsHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := resolvePath("~/tmp")
	if got != home+"/tmp" {
		t.Fatalf("resolvePath() = %q", got)
	}
}

func TestFromLoadedDefaultsPostRunState(t *testing.T) {
	settings, err := FromLoaded(workflow.Loaded{
		Config: map[string]any{
			"tracker": map[string]any{
				"kind":     "memory",
				"board_id": "ignored",
			},
		},
	})
	if err != nil {
		t.Fatalf("FromLoaded() error = %v", err)
	}
	if settings.Tracker.PostRunState != "Human Review" {
		t.Fatalf("expected default post-run state, got %q", settings.Tracker.PostRunState)
	}
}

func TestFromLoadedAllowsPostRunStateOverride(t *testing.T) {
	settings, err := FromLoaded(workflow.Loaded{
		Config: map[string]any{
			"tracker": map[string]any{
				"kind":           "memory",
				"post_run_state": "Ready for QA",
			},
		},
	})
	if err != nil {
		t.Fatalf("FromLoaded() error = %v", err)
	}
	if settings.Tracker.PostRunState != "Ready for QA" {
		t.Fatalf("expected overridden post-run state, got %q", settings.Tracker.PostRunState)
	}
}

func TestFromRawAllowsFizzyDefaultsWithoutBoardID(t *testing.T) {
	t.Setenv("FIZZY_TOKEN", "token-123")
	settings, err := FromRaw(map[string]any{
		"tracker": map[string]any{
			"kind":    "fizzy",
			"api_key": "$FIZZY_TOKEN",
		},
	})
	if err != nil {
		t.Fatalf("FromRaw() error = %v", err)
	}
	if settings.Tracker.BoardID != "" {
		t.Fatalf("expected empty board id in defaults, got %q", settings.Tracker.BoardID)
	}
}

func TestFromLoadedStillRequiresFizzyBoardID(t *testing.T) {
	t.Setenv("FIZZY_TOKEN", "token-123")
	if _, err := FromLoaded(workflow.Loaded{
		Config: map[string]any{
			"tracker": map[string]any{
				"kind":    "fizzy",
				"api_key": "$FIZZY_TOKEN",
			},
		},
	}); err == nil {
		t.Fatalf("expected missing board id error")
	}
}

func TestLoadRegistryMergesDefaultsAndRepoWorkflow(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "api")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "WORKFLOW.md"), []byte(`---
tracker:
  board_id: board-9
agent:
  max_turns: 7
hooks:
  before_run: |
    echo repo > repo.txt
---
Prompt for {{ issue.identifier }}
`), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`tracker_defaults:
  kind: memory
settings:
  polling:
    interval_ms: 5000
  workspace:
    root: ~/tmp/fizel
  agent:
    max_concurrent_agents: 2
    max_turns: 3
watched_repos:
  - key: API
    path: `+repoPath+`
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	registry, err := LoadRegistry(configPath)
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}
	repo, ok := registry.Repos["api"]
	if !ok {
		t.Fatalf("expected normalized repo key")
	}
	if repo.Settings.Tracker.BoardID != "board-9" {
		t.Fatalf("expected workflow override board id, got %q", repo.Settings.Tracker.BoardID)
	}
	if repo.Settings.Agent.MaxTurns != 3 {
		t.Fatalf("expected global max_turns to win, got %d", repo.Settings.Agent.MaxTurns)
	}
	if repo.Settings.Agent.MaxConcurrentAgents != 2 {
		t.Fatalf("expected default max_concurrent_agents, got %d", repo.Settings.Agent.MaxConcurrentAgents)
	}
	if repo.Settings.Hooks.BeforeRun != "echo repo > repo.txt" {
		t.Fatalf("expected repo hook from workflow, got %q", repo.Settings.Hooks.BeforeRun)
	}
	if repo.Settings.Repo.Path != repoPath {
		t.Fatalf("expected repo path %q, got %q", repoPath, repo.Settings.Repo.Path)
	}
	if repo.Loaded.Prompt != "Prompt for {{ issue.identifier }}" {
		t.Fatalf("unexpected prompt %q", repo.Loaded.Prompt)
	}
}

func TestLoadRegistryRejectsGlobalHooks(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "api")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "WORKFLOW.md"), []byte("---\ntracker:\n  board_id: board-1\n---\n"), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`tracker_defaults:
  kind: memory
settings:
  hooks:
    before_run: echo nope
watched_repos:
  - key: api
    path: `+repoPath+`
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadRegistry(configPath); err == nil {
		t.Fatalf("expected global hooks error")
	}
}

func TestFromRawIgnoresAfterCreateHook(t *testing.T) {
	s, err := FromRaw(map[string]any{
		"tracker": map[string]any{
			"kind": "memory",
		},
		"hooks": map[string]any{
			"after_create": "echo nope",
		},
	})
	if err != nil {
		t.Fatalf("expected after_create to be ignored, got %v", err)
	}
	if s.Hooks.BeforeRun != "" || s.Hooks.AfterRun != "" || s.Hooks.BeforeRemove != "" {
		t.Fatalf("expected deprecated after_create to be ignored, got hooks %+v", s.Hooks)
	}
}

func TestLoadRegistryRejectsLegacyDefaultsKey(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "api")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "WORKFLOW.md"), []byte("---\ntracker:\n  board_id: board-1\n---\n"), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`defaults:
  tracker:
    kind: memory
watched_repos:
  - key: api
    path: `+repoPath+`
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadRegistry(configPath); err == nil {
		t.Fatalf("expected legacy defaults key error")
	}
}

func TestLoadRegistryRejectsDuplicateKeys(t *testing.T) {
	root := t.TempDir()
	repoA := filepath.Join(root, "api-a")
	repoB := filepath.Join(root, "api-b")
	for _, repoPath := range []string{repoA, repoB} {
		if err := os.MkdirAll(repoPath, 0o755); err != nil {
			t.Fatalf("mkdir repo: %v", err)
		}
		if err := os.WriteFile(filepath.Join(repoPath, "WORKFLOW.md"), []byte("---\ntracker:\n  kind: memory\n---\n"), 0o644); err != nil {
			t.Fatalf("write workflow: %v", err)
		}
	}
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`tracker_defaults:
  kind: memory
watched_repos:
  - key: api
    path: `+repoA+`
  - key: API
    path: `+repoB+`
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadRegistry(configPath); err == nil {
		t.Fatalf("expected duplicate key error")
	}
}

func TestLoadRegistryRejectsMissingWorkflow(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "api")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`tracker_defaults:
  kind: memory
watched_repos:
  - key: api
    path: `+repoPath+`
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadRegistry(configPath); err == nil {
		t.Fatalf("expected missing workflow error")
	}
}

func TestLoadRegistryRejectsInvalidWorkflowFrontMatter(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "api")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "WORKFLOW.md"), []byte(`---
tracker: [invalid
---
`), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`tracker_defaults:
  kind: memory
watched_repos:
  - key: api
    path: `+repoPath+`
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadRegistry(configPath); err == nil {
		t.Fatalf("expected invalid workflow error")
	}
}

func TestLoadRegistryRejectsTrackerInsideSettings(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "api")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "WORKFLOW.md"), []byte("---\ntracker:\n  board_id: board-1\n---\n"), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`settings:
  tracker:
    kind: memory
watched_repos:
  - key: api
    path: `+repoPath+`
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadRegistry(configPath); err == nil {
		t.Fatalf("expected settings.tracker error")
	}
}
