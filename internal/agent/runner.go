package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/gmurray/fizel/internal/codex"
	"github.com/gmurray/fizel/internal/config"
	"github.com/gmurray/fizel/internal/model"
	"github.com/gmurray/fizel/internal/workspace"
)

type Runner struct {
	settings   config.Settings
	workspaces *workspace.Manager
}

func New(settings config.Settings, workspaces *workspace.Manager) *Runner {
	return &Runner{settings: settings, workspaces: workspaces}
}

func (r *Runner) Run(ctx context.Context, item model.Item, prompt string, onEvent func(codex.Event)) (string, error) {
	workspacePath, err := r.workspaces.CreateForItem(item, "")
	if err != nil {
		return "", err
	}
	if err := r.workspaces.RunBefore(workspacePath, ""); err != nil {
		return workspacePath, err
	}
	defer func() { _ = r.workspaces.RunAfter(workspacePath, "") }()

	session, err := codex.StartSession(ctx, r.settings, workspacePath)
	if err != nil {
		return workspacePath, err
	}
	defer session.Stop()

	for turn := 1; turn <= r.settings.Agent.MaxTurns; turn++ {
		turnPrompt := prompt
		if turn > 1 {
			turnPrompt = strings.TrimSpace(fmt.Sprintf("%s\n\nContinuation turn %d.", prompt, turn))
		}
		if err := session.RunTurn(turnPrompt, item, onEvent); err != nil {
			return workspacePath, err
		}
	}
	return workspacePath, nil
}
