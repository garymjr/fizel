package config

import (
	"os"
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
