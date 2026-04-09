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
	WorkflowPath string
	LogsRoot     string
	Port         int
	Logger       *slog.Logger
}

type App struct {
	orchestrator *orchestrator.Service
}

func New(opts Options) (*App, error) {
	loaded, err := workflow.Load(opts.WorkflowPath)
	if err != nil {
		return nil, err
	}
	settings, err := config.FromLoaded(loaded)
	if err != nil {
		return nil, err
	}

	var t tracker.Tracker
	switch settings.Tracker.Kind {
	case "memory":
		t = memory.New()
	case "fizzy":
		t = fizzy.NewFromSettings(settings.Tracker)
	default:
		return nil, fmt.Errorf("unsupported tracker %q", settings.Tracker.Kind)
	}

	dashboard := observability.NewTerminal(settings)
	svc := orchestrator.New(settings, loaded, t, dashboard, opts.Logger)
	return &App{orchestrator: svc}, nil
}

func (a *App) Run(ctx context.Context) error {
	return a.orchestrator.Run(ctx)
}
