package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gmurray/fizel/internal/config"
	"github.com/gmurray/fizel/internal/observability"
	"github.com/gmurray/fizel/internal/orchestrator"
	"github.com/gmurray/fizel/internal/tracker"
	"github.com/gmurray/fizel/internal/tracker/fizzy"
	"github.com/gmurray/fizel/internal/tracker/memory"
	"github.com/gmurray/fizel/internal/workflow"
)

type Options struct {
	ConfigPath   string
	WorkflowPath string
	LogsRoot     string
	Port         int
	Logger       *slog.Logger
}

type App struct {
	orchestrator *orchestrator.Service
}

func New(opts Options) (*App, error) {
	if opts.WorkflowPath != "" {
		loaded, err := workflow.Load(opts.WorkflowPath)
		if err != nil {
			return nil, err
		}
		settings, err := config.FromLoaded(loaded)
		if err != nil {
			return nil, err
		}
		t, err := trackerFromSettings(settings)
		if err != nil {
			return nil, err
		}
		dashboard := observability.NewTerminal(settings)
		svc := orchestrator.New(settings, loaded, nil, t, dashboard, opts.Logger)
		return &App{orchestrator: svc}, nil
	}

	registry, err := config.LoadRegistry(opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	t, err := trackerFromRegistry(registry)
	if err != nil {
		return nil, err
	}
	dashboard := observability.NewTerminal(registry.Defaults)
	svc := orchestrator.New(registry.Defaults, workflow.Loaded{}, registry.Repos, t, dashboard, opts.Logger)
	return &App{orchestrator: svc}, nil
}

func (a *App) Run(ctx context.Context) error {
	return a.orchestrator.Run(ctx)
}

func trackerFromSettings(settings config.Settings) (tracker.Tracker, error) {
	switch settings.Tracker.Kind {
	case "memory":
		return memory.New(), nil
	case "fizzy":
		return fizzy.NewFromSettings(settings.Tracker), nil
	default:
		return nil, fmt.Errorf("unsupported tracker %q", settings.Tracker.Kind)
	}
}

func trackerFromRegistry(registry config.Registry) (tracker.Tracker, error) {
	switch registry.Defaults.Tracker.Kind {
	case "memory":
		return memory.New(), nil
	case "fizzy":
		settings := make([]config.TrackerSettings, 0, len(registry.Repos))
		for _, repo := range registry.Repos {
			settings = append(settings, repo.Settings.Tracker)
		}
		return fizzy.NewFromMany(settings), nil
	default:
		return nil, fmt.Errorf("unsupported tracker %q", registry.Defaults.Tracker.Kind)
	}
}
