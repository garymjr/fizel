package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
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
	repos    map[string]config.ResolvedRepo
	tracker  tracker.Tracker
	term     *observability.Terminal
	logger   *slog.Logger

	mu       sync.Mutex
	running  map[string]runningItem
	retrying map[string]retryItem
}

type runningItem struct {
	item      model.Item
	repoKey   string
	startedAt time.Time
	lastEvent string
}

type retryItem struct {
	item    model.Item
	repoKey string
	attempt int
	retryAt time.Time
}

func New(settings config.Settings, loaded workflow.Loaded, repos map[string]config.ResolvedRepo, tracker tracker.Tracker, term *observability.Terminal, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		settings: settings,
		loaded:   loaded,
		repos:    repos,
		tracker:  tracker,
		term:     term,
		logger:   logger,
		running:  map[string]runningItem{},
		retrying: map[string]retryItem{},
	}
}

func (s *Service) Terminal() *observability.Terminal {
	return s.term
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
	s.render(true)
	s.dispatchDueRetries(ctx)
	items, err := s.tracker.FetchCandidateItems()
	if err != nil {
		return err
	}
	for _, item := range items {
		repo, err := s.repoForItem(item)
		if err != nil {
			s.logger.Error("repo resolution failed", "item", item.Identifier, "error", err)
			continue
		}
		if !s.activeState(item.State, repo.Settings) {
			continue
		}
		if s.isBusy(item.ID) {
			continue
		}
		if !s.canStart(repo.Key, repo.Settings.Agent.MaxConcurrentAgents) {
			continue
		}
		s.startItem(ctx, item, repo)
	}
	s.render(false)
	return nil
}

func (s *Service) startItem(ctx context.Context, item model.Item, repo config.ResolvedRepo) {
	s.mu.Lock()
	s.running[item.ID] = runningItem{item: item, repoKey: repo.Key, startedAt: time.Now()}
	s.mu.Unlock()

	go func() {
		manager := workspace.New(repo.Settings)
		runner := agent.New(repo.Settings, manager)
		path, err := runner.Run(ctx, item, s.promptFor(item, repo.Loaded), func(event codex.Event) {
			s.mu.Lock()
			entry := s.running[item.ID]
			entry.lastEvent = event.Event
			s.running[item.ID] = entry
			s.mu.Unlock()
			s.render(true)
		})
		s.mu.Lock()
		delete(s.running, item.ID)
		s.mu.Unlock()
		if err != nil {
			s.logger.Error("agent run failed", "item", item.Identifier, "repo", repo.Key, "error", err)
			s.scheduleRetry(item, repo)
			return
		}
		if err := s.transitionCompletedItem(item, repo.Settings); err != nil {
			s.logger.Error("tracker completion transition failed", "item", item.Identifier, "repo", repo.Key, "state", repo.Settings.Tracker.PostRunState, "error", err)
			s.scheduleRetry(item, repo)
			return
		}
		s.logger.Info("agent run completed", "item", item.Identifier, "repo", repo.Key, "workspace", path)
		s.render(false)
	}()
}

func (s *Service) transitionCompletedItem(item model.Item, settings config.Settings) error {
	target := strings.TrimSpace(settings.Tracker.PostRunState)
	if target == "" || strings.EqualFold(target, item.State) {
		return nil
	}
	return s.tracker.UpdateItemState(item.ID, target)
}

func (s *Service) promptFor(item model.Item, loaded workflow.Loaded) string {
	prompt := strings.TrimSpace(loaded.PromptTemplate)
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

func (s *Service) scheduleRetry(item model.Item, repo config.ResolvedRepo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.retrying[item.ID]
	entry.item = item
	entry.repoKey = repo.Key
	entry.attempt++
	backoff := time.Duration(entry.attempt*10) * time.Second
	max := time.Duration(repo.Settings.Agent.MaxRetryBackoffMS) * time.Millisecond
	if backoff > max {
		backoff = max
	}
	entry.retryAt = time.Now().Add(backoff)
	s.retrying[item.ID] = entry
}

func (s *Service) dispatchDueRetries(ctx context.Context) {
	now := time.Now()
	var due []retryItem
	s.mu.Lock()
	for id, entry := range s.retrying {
		if entry.retryAt.IsZero() || entry.retryAt.After(now) {
			continue
		}
		delete(s.retrying, id)
		due = append(due, entry)
	}
	s.mu.Unlock()
	for _, entry := range due {
		repo, err := s.repoForRetry(entry)
		if err != nil {
			s.logger.Error("retry repo resolution failed", "item", entry.item.Identifier, "error", err)
			continue
		}
		if s.isBusy(entry.item.ID) || !s.canStart(repo.Key, repo.Settings.Agent.MaxConcurrentAgents) {
			s.scheduleRetry(entry.item, repo)
			continue
		}
		s.startItem(ctx, entry.item, repo)
	}
}

func (s *Service) isBusy(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, running := s.running[id]
	return running
}

func (s *Service) activeState(state string, settings config.Settings) bool {
	current := strings.ToLower(strings.TrimSpace(state))
	for _, active := range settings.Tracker.ActiveStates {
		if strings.ToLower(strings.TrimSpace(active)) == current {
			return true
		}
	}
	return false
}

func (s *Service) render(polling bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lastRefreshAt := time.Time{}
	if !polling {
		lastRefreshAt = time.Now()
	}
	running := make([]observability.RunningItem, 0, len(s.running))
	for _, entry := range s.running {
		running = append(running, observability.RunningItem{
			Identifier: entry.item.Identifier,
			RepoKey:    entry.repoKey,
			State:      entry.item.State,
			StartedAt:  entry.startedAt,
			LastEvent:  entry.lastEvent,
		})
	}
	retrying := make([]observability.RetryItem, 0, len(s.retrying))
	for _, entry := range s.retrying {
		retrying = append(retrying, observability.RetryItem{
			Identifier: entry.item.Identifier,
			RepoKey:    entry.repoKey,
			Attempt:    entry.attempt,
			RetryAt:    entry.retryAt,
		})
	}
	mode := s.settings.Tracker.Kind
	if len(s.repos) > 0 {
		mode = "fizzy watched repos"
	} else if s.settings.Tracker.Kind == "fizzy" {
		mode = "fizzy single workflow"
	}
	watched := make([]observability.WatchedRepoStatus, 0, len(s.repos))
	for _, repo := range s.repos {
		watched = append(watched, observability.WatchedRepoStatus{
			Key:     repo.Key,
			BoardID: repo.Settings.Tracker.BoardID,
		})
	}
	sort.Slice(watched, func(i, j int) bool { return watched[i].Key < watched[j].Key })
	s.term.Render(observability.Snapshot{
		Polling:       polling,
		LastRefreshAt: lastRefreshAt,
		Running:       running,
		Retrying:      retrying,
		TrackerMode:   mode,
		WatchedRepos:  watched,
	})
}

func (s *Service) repoForItem(item model.Item) (config.ResolvedRepo, error) {
	if len(s.repos) == 0 {
		return config.ResolvedRepo{
			Settings: s.settings,
			Loaded:   s.loaded,
		}, nil
	}
	var keys []string
	for _, label := range item.Labels {
		label = strings.TrimSpace(strings.ToLower(label))
		if !strings.HasPrefix(label, "repo:") {
			continue
		}
		key := strings.TrimSpace(strings.TrimPrefix(label, "repo:"))
		if key != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		repo, err := s.repoForBoardID(boardIDFromItem(item.ID))
		if err != nil {
			return config.ResolvedRepo{}, fmt.Errorf("item is missing repo:<key> label: %w", err)
		}
		return repo, nil
	}
	if len(keys) > 1 {
		return config.ResolvedRepo{}, fmt.Errorf("item has multiple repo labels: %s", strings.Join(keys, ", "))
	}
	repo, ok := s.repos[keys[0]]
	if !ok {
		return config.ResolvedRepo{}, fmt.Errorf("watched repo %q not found", keys[0])
	}
	return repo, nil
}

func (s *Service) repoForBoardID(boardID string) (config.ResolvedRepo, error) {
	boardID = strings.TrimSpace(boardID)
	if boardID == "" {
		return config.ResolvedRepo{}, fmt.Errorf("item id does not include a board id")
	}
	var matched config.ResolvedRepo
	found := false
	for _, repo := range s.repos {
		if strings.TrimSpace(repo.Settings.Tracker.BoardID) != boardID {
			continue
		}
		if found {
			return config.ResolvedRepo{}, fmt.Errorf("multiple watched repos use board %q", boardID)
		}
		matched = repo
		found = true
	}
	if !found {
		return config.ResolvedRepo{}, fmt.Errorf("no watched repo configured for board %q", boardID)
	}
	return matched, nil
}

func boardIDFromItem(id string) string {
	boardID, _, ok := strings.Cut(strings.TrimSpace(id), ":")
	if !ok {
		return ""
	}
	return boardID
}

func (s *Service) repoForRetry(entry retryItem) (config.ResolvedRepo, error) {
	if len(s.repos) == 0 {
		return config.ResolvedRepo{
			Settings: s.settings,
			Loaded:   s.loaded,
		}, nil
	}
	if entry.repoKey == "" {
		return s.repoForItem(entry.item)
	}
	repo, ok := s.repos[entry.repoKey]
	if !ok {
		return config.ResolvedRepo{}, fmt.Errorf("watched repo %q not found", entry.repoKey)
	}
	return repo, nil
}

func (s *Service) canStart(repoKey string, limit int) bool {
	if limit <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, entry := range s.running {
		if entry.repoKey == repoKey {
			count++
		}
	}
	return count < limit
}
