package fizzy

import (
	"encoding/json"
	"testing"

	"github.com/gmurray/fizel/internal/config"
)

func TestFetchCandidateItemsNormalizesCard(t *testing.T) {
	runner := func(args []string, env []string) ([]byte, error) {
		payload := map[string]any{
			"ok": true,
			"data": []map[string]any{{
				"number":      42,
				"title":       "Fix login",
				"description": "Broken form",
				"column":      map[string]any{"name": "In Progress"},
				"board":       map[string]any{"id": "board-1", "name": "Platform"},
				"assignees":   []map[string]any{{"id": "usr_123"}},
				"tags":        []map[string]any{{"title": "Bug"}},
			}},
		}
		return json.Marshal(payload)
	}
	tracker := NewWithRunner(config.TrackerSettings{
		Kind:    "fizzy",
		APIKey:  "token",
		APIURL:  "https://app.fizzy.do",
		BoardID: "board-1",
	}, runner)

	items, err := tracker.FetchCandidateItems()
	if err != nil {
		t.Fatalf("FetchCandidateItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item.ID != "board-1:42" {
		t.Fatalf("unexpected item ID %q", item.ID)
	}
	if item.State != "In Progress" {
		t.Fatalf("unexpected state %q", item.State)
	}
	if item.AssigneeID != "usr_123" {
		t.Fatalf("unexpected assignee %q", item.AssigneeID)
	}
}
