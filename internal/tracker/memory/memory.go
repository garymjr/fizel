package memory

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gmurray/fizel/internal/model"
)

type Tracker struct {
	mu    sync.Mutex
	items map[string]model.Item
}

func New() *Tracker {
	return &Tracker{items: map[string]model.Item{}}
}

func (t *Tracker) Seed(items ...model.Item) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, item := range items {
		t.items[item.ID] = item
	}
}

func (t *Tracker) FetchCandidateItems() ([]model.Item, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []model.Item
	for _, item := range t.items {
		out = append(out, item)
	}
	return out, nil
}

func (t *Tracker) FetchItemsByStates(states []string) ([]model.Item, error) {
	want := map[string]struct{}{}
	for _, state := range states {
		want[strings.ToLower(strings.TrimSpace(state))] = struct{}{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []model.Item
	for _, item := range t.items {
		if _, ok := want[strings.ToLower(item.State)]; ok {
			out = append(out, item)
		}
	}
	return out, nil
}

func (t *Tracker) FetchItemStatesByIDs(ids []string) ([]model.Item, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []model.Item
	for _, id := range ids {
		if item, ok := t.items[id]; ok {
			out = append(out, item)
		}
	}
	return out, nil
}

func (t *Tracker) CreateComment(id, body string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	item, ok := t.items[id]
	if !ok {
		return fmt.Errorf("item %s not found", id)
	}
	item.Description = strings.TrimSpace(item.Description + "\n\n" + body)
	t.items[id] = item
	return nil
}

func (t *Tracker) UpdateItemState(id, state string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	item, ok := t.items[id]
	if !ok {
		return fmt.Errorf("item %s not found", id)
	}
	item.State = state
	t.items[id] = item
	return nil
}
