package orchestrator

import (
	"bytes"
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
		tracker,
		observability.NewTerminalForWriter(config.Settings{
			Agent: config.AgentSettings{MaxConcurrentAgents: 1},
		}, &bytes.Buffer{}),
		nil,
	)

	if err := service.transitionCompletedItem(item); err != nil {
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
		tracker,
		observability.NewTerminalForWriter(config.Settings{
			Agent: config.AgentSettings{MaxConcurrentAgents: 1},
		}, &bytes.Buffer{}),
		nil,
	)

	if err := service.transitionCompletedItem(item); err != nil {
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
