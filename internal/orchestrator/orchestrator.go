package orchestrator

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gmurray/fizel/internal/agent"
	"github.com/gmurray/fizel/internal/codex"
	"github.com/gmurray/fizel/internal/config"
	"github.com/gmurray/fizel/internal/model"
	"github.com/gmurray/fizel/internal/observability"
	"github.com/gmurray/fizel/internal/tracker"
	"github.com/gmurray/fizel/internal/workflow"
	"github.com/gmurray/fizel/internal/workspace"
)

type Service struct {
	settings config.Settings
	loaded   workflow.Loaded
	tracker  tracker.Tracker
	term     *observability.Terminal
	logger   *slog.Logger

	mu       sync.Mutex
	running  map[string]runningItem
	retrying map[string]retryItem
}

type runningItem struct {
	item      model.Item
	startedAt time.Time
}

type retryItem struct {
	item    model.Item
	attempt int
	retryAt time.Time
}

func New(settings config.Settings, loaded workflow.Loaded, tracker tracker.Tracker, term *observability.Terminal, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		settings: settings,
		loaded:   loaded,
		tracker:  tracker,
		term:     term,
		logger:   logger,
		running:  map[string]runningItem{},
		retrying: map[string]retryItem{},
	}
}

func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(s.settings.Polling.IntervalMS) * time.Millisecond)
	defer ticker.Stop()
	if err := s.dispatch(ctx); err != nil {
		s.logger.Error("initial dispatch failed", "error", err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.dispatch(ctx); err != nil {
				s.logger.Error("dispatch failed", "error", err)
			}
		}
	}
}

func (s *Service) dispatch(ctx context.Context) error {
	s.render(true, "")
	s.dispatchDueRetries(ctx)
	items, err := s.tracker.FetchCandidateItems()
	if err != nil {
		return err
	}
	for _, item := range items {
		if !s.activeState(item.State) {
			continue
		}
		if s.isBusy(item.ID) {
			continue
		}
		if len(s.running) >= s.settings.Agent.MaxConcurrentAgents {
			break
		}
		s.startItem(ctx, item)
	}
	s.render(false, "")
	return nil
}

func (s *Service) startItem(ctx context.Context, item model.Item) {
	s.mu.Lock()
	s.running[item.ID] = runningItem{item: item, startedAt: time.Now()}
	s.mu.Unlock()

	go func() {
		manager := workspace.New(s.settings)
		runner := agent.New(s.settings, manager)
		lastEvent := ""
		path, err := runner.Run(ctx, item, s.promptFor(item), func(event codex.Event) {
			lastEvent = event.Event
			s.render(true, lastEvent)
		})
		s.mu.Lock()
		delete(s.running, item.ID)
		s.mu.Unlock()
		if err != nil {
			s.logger.Error("agent run failed", "item", item.Identifier, "error", err)
			s.scheduleRetry(item)
			return
		}
		s.logger.Info("agent run completed", "item", item.Identifier, "workspace", path)
		s.render(false, lastEvent)
	}()
}

func (s *Service) promptFor(item model.Item) string {
	prompt := strings.TrimSpace(s.loaded.PromptTemplate)
	if prompt == "" {
		prompt = "You are working on tracker item {{ issue.identifier }}."
	}
	replacer := strings.NewReplacer(
		"{{ issue.identifier }}", item.Identifier,
		"{{ issue.title }}", item.Title,
		"{{ issue.description }}", item.Description,
		"{{ issue.state }}", item.State,
		"{{ issue.url }}", item.URL,
	)
	return replacer.Replace(prompt)
}

func (s *Service) scheduleRetry(item model.Item) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.retrying[item.ID]
	entry.item = item
	entry.attempt++
	backoff := time.Duration(entry.attempt*10) * time.Second
	max := time.Duration(s.settings.Agent.MaxRetryBackoffMS) * time.Millisecond
	if backoff > max {
		backoff = max
	}
	entry.retryAt = time.Now().Add(backoff)
	s.retrying[item.ID] = entry
}

func (s *Service) dispatchDueRetries(ctx context.Context) {
	now := time.Now()
	var due []model.Item
	s.mu.Lock()
	for id, entry := range s.retrying {
		if entry.retryAt.IsZero() || entry.retryAt.After(now) {
			continue
		}
		delete(s.retrying, id)
		due = append(due, entry.item)
	}
	s.mu.Unlock()
	for _, item := range due {
		if s.isBusy(item.ID) || len(s.running) >= s.settings.Agent.MaxConcurrentAgents {
			s.scheduleRetry(item)
			continue
		}
		s.startItem(ctx, item)
	}
}

func (s *Service) isBusy(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, running := s.running[id]
	return running
}

func (s *Service) activeState(state string) bool {
	current := strings.ToLower(strings.TrimSpace(state))
	for _, active := range s.settings.Tracker.ActiveStates {
		if strings.ToLower(strings.TrimSpace(active)) == current {
			return true
		}
	}
	return false
}

func (s *Service) render(polling bool, lastEvent string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	running := make([]observability.RunningItem, 0, len(s.running))
	for _, entry := range s.running {
		running = append(running, observability.RunningItem{
			Identifier: entry.item.Identifier,
			State:      entry.item.State,
			StartedAt:  entry.startedAt,
		})
	}
	retrying := make([]observability.RetryItem, 0, len(s.retrying))
	for _, entry := range s.retrying {
		retrying = append(retrying, observability.RetryItem{
			Identifier: entry.item.Identifier,
			Attempt:    entry.attempt,
			RetryAt:    entry.retryAt,
		})
	}
	header := s.settings.Tracker.Kind
	if s.settings.Tracker.Kind == "fizzy" {
		header = "Fizzy board " + s.settings.Tracker.BoardID
	}
	s.term.Render(observability.Snapshot{
		Polling:       polling,
		Running:       running,
		Retrying:      retrying,
		TrackerHeader: header,
		LastEvent:     lastEvent,
	})
}
