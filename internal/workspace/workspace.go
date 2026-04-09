package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gmurray/fizel/internal/config"
	"github.com/gmurray/fizel/internal/model"
	"github.com/gmurray/fizel/internal/ssh"
)

var unsafePath = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type Manager struct {
	settings config.Settings
}

func New(settings config.Settings) *Manager {
	return &Manager{settings: settings}
}

func (m *Manager) PathForItem(item model.Item) string {
	return filepath.Join(m.settings.Workspace.Root, safeIdentifier(item.Identifier))
}

func (m *Manager) CreateForItem(item model.Item, workerHost string) (string, error) {
	path := m.PathForItem(item)
	if strings.TrimSpace(workerHost) != "" {
		_, code, err := ssh.Run(workerHost, fmt.Sprintf("mkdir -p %q", path))
		if err != nil && code != 0 {
			return "", fmt.Errorf("prepare remote workspace: %w", err)
		}
		if err := m.runHook(m.settings.Hooks.AfterCreate, path, workerHost); err != nil {
			return "", err
		}
		return path, nil
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}
	if err := m.runHook(m.settings.Hooks.AfterCreate, path, ""); err != nil {
		return "", err
	}
	return path, nil
}

func (m *Manager) RunBefore(path, workerHost string) error {
	return m.runHook(m.settings.Hooks.BeforeRun, path, workerHost)
}

func (m *Manager) RunAfter(path, workerHost string) error {
	return m.runHook(m.settings.Hooks.AfterRun, path, workerHost)
}

func (m *Manager) Remove(path, workerHost string) error {
	_ = m.runHook(m.settings.Hooks.BeforeRemove, path, workerHost)
	if strings.TrimSpace(workerHost) != "" {
		_, _, err := ssh.Run(workerHost, fmt.Sprintf("rm -rf %q", path))
		return err
	}
	return os.RemoveAll(path)
}

func (m *Manager) runHook(script, path, workerHost string) error {
	if strings.TrimSpace(script) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(m.settings.Hooks.TimeoutMS)*time.Millisecond)
	defer cancel()
	if strings.TrimSpace(workerHost) != "" {
		_, code, err := ssh.Run(workerHost, fmt.Sprintf("%s cd %q && %s", remoteHookExports(m.settings), path, script))
		if err != nil && code != 0 {
			return fmt.Errorf("remote hook failed: %w", err)
		}
		return nil
	}
	cmd := exec.CommandContext(ctx, "bash", "-lc", script)
	cmd.Dir = path
	cmd.Env = append(os.Environ(), hookEnv(m.settings)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("hook failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func safeIdentifier(id string) string {
	if strings.TrimSpace(id) == "" {
		return "item"
	}
	return unsafePath.ReplaceAllString(id, "_")
}

func hookEnv(settings config.Settings) []string {
	if strings.TrimSpace(settings.Repo.Path) == "" {
		return nil
	}
	return []string{
		"SOURCE_REPO_URL=" + settings.Repo.Path,
		"SOURCE_REPO_PATH=" + settings.Repo.Path,
		"SOURCE_REPO_KEY=" + settings.Repo.Key,
		"SOURCE_WORKFLOW_PATH=" + settings.Repo.WorkflowPath,
	}
}

func remoteHookExports(settings config.Settings) string {
	env := hookEnv(settings)
	if len(env) == 0 {
		return ""
	}
	parts := make([]string, 0, len(env))
	for _, entry := range env {
		pair := strings.SplitN(entry, "=", 2)
		if len(pair) != 2 {
			continue
		}
		parts = append(parts, fmt.Sprintf("export %s=%q;", pair[0], pair[1]))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ") + " "
}
