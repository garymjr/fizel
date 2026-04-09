package orchestrator

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gmurray/fizel/internal/config"
	"github.com/gmurray/fizel/internal/model"
	"github.com/gmurray/fizel/internal/observability"
	"github.com/gmurray/fizel/internal/tracker/memory"
	"github.com/gmurray/fizel/internal/workflow"
)

func TestTransitionCompletedItemMovesCardToPostRunState(t *testing.T) {
	tracker := memory.New()
	item := model.Item{
		ID:         "board-1:42",
		Identifier: "board-1:42",
		Title:      "Fix bug",
		State:      "In Progress",
	}
	tracker.Seed(item)

	service := New(
		config.Settings{
			Tracker: config.TrackerSettings{
				Kind:         "memory",
				PostRunState: "Human Review",
			},
			Agent: config.AgentSettings{MaxConcurrentAgents: 1},
		},
		workflow.Loaded{},
		nil,
		tracker,
		observability.NewTerminalForWriter(config.Settings{
			Agent: config.AgentSettings{MaxConcurrentAgents: 1},
		}, &bytes.Buffer{}),
		nil,
	)

	if err := service.transitionCompletedItem(item, config.Settings{
		Tracker: config.TrackerSettings{
			Kind:         "memory",
			PostRunState: "Human Review",
		},
	}); err != nil {
		t.Fatalf("transitionCompletedItem() error = %v", err)
	}

	items, err := tracker.FetchItemStatesByIDs([]string{item.ID})
	if err != nil {
		t.Fatalf("FetchItemStatesByIDs() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	if items[0].State != "Human Review" {
		t.Fatalf("expected state Human Review, got %q", items[0].State)
	}
}

func TestTransitionCompletedItemSkipsWhenPostRunStateBlank(t *testing.T) {
	tracker := memory.New()
	item := model.Item{
		ID:         "board-1:42",
		Identifier: "board-1:42",
		Title:      "Fix bug",
		State:      "In Progress",
	}
	tracker.Seed(item)

	service := New(
		config.Settings{
			Tracker: config.TrackerSettings{
				Kind:         "memory",
				PostRunState: "",
			},
			Agent: config.AgentSettings{MaxConcurrentAgents: 1},
		},
		workflow.Loaded{},
		nil,
		tracker,
		observability.NewTerminalForWriter(config.Settings{
			Agent: config.AgentSettings{MaxConcurrentAgents: 1},
		}, &bytes.Buffer{}),
		nil,
	)

	if err := service.transitionCompletedItem(item, config.Settings{
		Tracker: config.TrackerSettings{
			Kind:         "memory",
			PostRunState: "",
		},
	}); err != nil {
		t.Fatalf("transitionCompletedItem() error = %v", err)
	}

	items, err := tracker.FetchItemStatesByIDs([]string{item.ID})
	if err != nil {
		t.Fatalf("FetchItemStatesByIDs() error = %v", err)
	}
	if items[0].State != "In Progress" {
		t.Fatalf("expected unchanged state, got %q", items[0].State)
	}
}

func TestCleanupMergedWorkspacesRemovesDoneWorkspace(t *testing.T) {
	root := t.TempDir()
	tracker := memory.New()
	item := model.Item{
		ID:         "board-1:42",
		Identifier: "board-1:42",
		State:      "Done",
		Labels:     []string{"repo:api"},
	}
	tracker.Seed(item)

	repo := config.ResolvedRepo{
		Key: "api",
		Settings: config.Settings{
			Workspace: config.WorkspaceSettings{Root: root},
			Repo:      config.RepoSettings{Key: "api"},
			Agent:     config.AgentSettings{MaxConcurrentAgents: 1},
			Hooks:     config.HookSettings{TimeoutMS: 1000},
		},
	}
	path := filepath.Join(root, "board-1_42")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	service := New(
		config.Settings{Agent: config.AgentSettings{MaxConcurrentAgents: 1}},
		workflow.Loaded{},
		map[string]config.ResolvedRepo{"api": repo},
		tracker,
		observability.NewTerminalForWriter(config.Settings{
			Agent: config.AgentSettings{MaxConcurrentAgents: 1},
		}, &bytes.Buffer{}),
		nil,
	)

	service.cleanupMergedWorkspaces()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected workspace removal, stat err = %v", err)
	}
}

func TestCleanupMergedWorkspacesSkipsNonDoneWorkspace(t *testing.T) {
	root := t.TempDir()
	tracker := memory.New()
	item := model.Item{
		ID:         "board-1:42",
		Identifier: "board-1:42",
		State:      "Human Review",
		Labels:     []string{"repo:api"},
	}
	tracker.Seed(item)

	repo := config.ResolvedRepo{
		Key: "api",
		Settings: config.Settings{
			Workspace: config.WorkspaceSettings{Root: root},
			Repo:      config.RepoSettings{Key: "api"},
			Agent:     config.AgentSettings{MaxConcurrentAgents: 1},
			Hooks:     config.HookSettings{TimeoutMS: 1000},
		},
	}
	path := filepath.Join(root, "board-1_42")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	service := New(
		config.Settings{Agent: config.AgentSettings{MaxConcurrentAgents: 1}},
		workflow.Loaded{},
		map[string]config.ResolvedRepo{"api": repo},
		tracker,
		observability.NewTerminalForWriter(config.Settings{
			Agent: config.AgentSettings{MaxConcurrentAgents: 1},
		}, &bytes.Buffer{}),
		nil,
	)

	service.cleanupMergedWorkspaces()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected workspace to remain, stat err = %v", err)
	}
}

func TestRepoForItemResolvesFromLabel(t *testing.T) {
	service := New(
		config.Settings{},
		workflow.Loaded{},
		map[string]config.ResolvedRepo{
			"api": {
				Key: "api",
				Settings: config.Settings{
					Repo: config.RepoSettings{Key: "api"},
				},
			},
		},
		memory.New(),
		observability.NewTerminalForWriter(config.Settings{
			Agent: config.AgentSettings{MaxConcurrentAgents: 1},
		}, &bytes.Buffer{}),
		nil,
	)

	repo, err := service.repoForItem(model.Item{
		ID:         "board-1:42",
		Identifier: "board-1:42",
		Labels:     []string{"repo:api"},
	})
	if err != nil {
		t.Fatalf("repoForItem() error = %v", err)
	}
	if repo.Key != "api" {
		t.Fatalf("expected api repo, got %q", repo.Key)
	}
}

func TestRepoForItemRejectsMissingLabel(t *testing.T) {
	service := New(
		config.Settings{},
		workflow.Loaded{},
		map[string]config.ResolvedRepo{
			"api": {
				Key: "api",
				Settings: config.Settings{
					Tracker: config.TrackerSettings{BoardID: "board-1"},
				},
			},
		},
		memory.New(),
		observability.NewTerminalForWriter(config.Settings{
			Agent: config.AgentSettings{MaxConcurrentAgents: 1},
		}, &bytes.Buffer{}),
		nil,
	)

	repo, err := service.repoForItem(model.Item{ID: "board-1:42", Identifier: "board-1:42"})
	if err != nil {
		t.Fatalf("expected board fallback resolution, got error %v", err)
	}
	if repo.Key != "api" {
		t.Fatalf("expected api repo, got %q", repo.Key)
	}
}

func TestRepoForItemRejectsMultipleLabels(t *testing.T) {
	service := New(
		config.Settings{},
		workflow.Loaded{},
		map[string]config.ResolvedRepo{
			"api": {Key: "api"},
			"web": {Key: "web"},
		},
		memory.New(),
		observability.NewTerminalForWriter(config.Settings{
			Agent: config.AgentSettings{MaxConcurrentAgents: 1},
		}, &bytes.Buffer{}),
		nil,
	)

	if _, err := service.repoForItem(model.Item{
		ID:         "board-1:42",
		Identifier: "board-1:42",
		Labels:     []string{"repo:api", "repo:web"},
	}); err == nil {
		t.Fatalf("expected multiple repo label error")
	}
}

func TestRepoForItemRejectsMissingLabelWithoutBoardFallback(t *testing.T) {
	service := New(
		config.Settings{},
		workflow.Loaded{},
		map[string]config.ResolvedRepo{
			"api": {Key: "api"},
		},
		memory.New(),
		observability.NewTerminalForWriter(config.Settings{
			Agent: config.AgentSettings{MaxConcurrentAgents: 1},
		}, &bytes.Buffer{}),
		nil,
	)

	if _, err := service.repoForItem(model.Item{ID: "board-1:42", Identifier: "board-1:42"}); err == nil {
		t.Fatalf("expected missing repo label error")
	}
}

func TestRepoForItemRejectsAmbiguousBoardFallback(t *testing.T) {
	service := New(
		config.Settings{},
		workflow.Loaded{},
		map[string]config.ResolvedRepo{
			"api": {
				Key: "api",
				Settings: config.Settings{
					Tracker: config.TrackerSettings{BoardID: "board-1"},
				},
			},
			"web": {
				Key: "web",
				Settings: config.Settings{
					Tracker: config.TrackerSettings{BoardID: "board-1"},
				},
			},
		},
		memory.New(),
		observability.NewTerminalForWriter(config.Settings{
			Agent: config.AgentSettings{MaxConcurrentAgents: 1},
		}, &bytes.Buffer{}),
		nil,
	)

	if _, err := service.repoForItem(model.Item{ID: "board-1:42", Identifier: "board-1:42"}); err == nil {
		t.Fatalf("expected ambiguous board fallback error")
	}
}

func TestPromptForUsesRepoWorkflowPrompt(t *testing.T) {
	service := New(
		config.Settings{},
		workflow.Loaded{},
		nil,
		memory.New(),
		observability.NewTerminalForWriter(config.Settings{
			Agent: config.AgentSettings{MaxConcurrentAgents: 1},
		}, &bytes.Buffer{}),
		nil,
	)

	got := service.promptFor(model.Item{
		Identifier: "board-1:42",
		Title:      "Fix bug",
	}, workflow.Loaded{PromptTemplate: "Repo prompt for {{ issue.identifier }}: {{ issue.title }}"})
	if got != "Repo prompt for board-1:42: Fix bug" {
		t.Fatalf("unexpected prompt %q", got)
	}
}
