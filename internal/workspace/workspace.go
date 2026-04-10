package workspace

import (
	"context"
	"errors"
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
	if strings.TrimSpace(m.settings.Repo.Path) == "" {
		if strings.TrimSpace(workerHost) != "" {
			_, code, err := ssh.Run(workerHost, fmt.Sprintf("mkdir -p %q", path))
			if err != nil && code != 0 {
				return "", fmt.Errorf("prepare remote workspace: %w", err)
			}
			return path, nil
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return "", err
		}
		return path, nil
	}
	if strings.TrimSpace(workerHost) != "" {
		if err := m.ensureRemoteWorktree(path, workerHost, worktreeBranch(item)); err != nil {
			return "", err
		}
		return path, nil
	}
	if err := m.ensureLocalWorktree(path, worktreeBranch(item)); err != nil {
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
	if strings.TrimSpace(m.settings.Repo.Path) != "" {
		if strings.TrimSpace(workerHost) != "" {
			_, code, err := ssh.Run(workerHost, fmt.Sprintf("git -C %q worktree remove --force %q", m.settings.Repo.Path, path))
			if err == nil || code == 0 {
				return nil
			}
		} else {
			cmd := exec.Command("git", "-C", m.settings.Repo.Path, "worktree", "remove", "--force", path)
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
	}
	if strings.TrimSpace(workerHost) != "" {
		_, _, err := ssh.Run(workerHost, fmt.Sprintf("rm -rf %q", path))
		return err
	}
	return os.RemoveAll(path)
}

func (m *Manager) ensureLocalWorktree(path, branch string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	args, err := m.worktreeAddArgs(path, branch)
	if err != nil {
		return err
	}
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("create worktree: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *Manager) ensureRemoteWorktree(path, workerHost, branch string) error {
	quotedArgs, err := m.remoteWorktreeAddArgs(workerHost, path, branch)
	if err != nil {
		return err
	}
	command := fmt.Sprintf("if [ -e %q ]; then exit 0; fi; git %s", path, strings.Join(quotedArgs, " "))
	_, code, err := ssh.Run(workerHost, command)
	if err != nil || code != 0 {
		return fmt.Errorf("create remote worktree: %w", err)
	}
	return nil
}

func (m *Manager) worktreeAddArgs(path, branch string) ([]string, error) {
	exists, err := m.localBranchExists(branch)
	if err != nil {
		return nil, err
	}
	args := []string{"-C", m.settings.Repo.Path, "worktree", "add"}
	if !exists {
		args = append(args, "-b", branch)
		args = append(args, path)
		return args, nil
	}
	args = append(args, path, branch)
	return args, nil
}

func (m *Manager) remoteWorktreeAddArgs(workerHost, path, branch string) ([]string, error) {
	exists, err := m.remoteBranchExists(workerHost, branch)
	if err != nil {
		return nil, err
	}
	args := []string{"-C", shellQuote(m.settings.Repo.Path), "worktree", "add"}
	if !exists {
		args = append(args, "-b", shellQuote(branch))
		args = append(args, shellQuote(path))
		return args, nil
	}
	args = append(args, shellQuote(path), shellQuote(branch))
	return args, nil
}

func (m *Manager) localBranchExists(branch string) (bool, error) {
	cmd := exec.Command("git", "-C", m.settings.Repo.Path, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("check worktree branch: %w", err)
}

func (m *Manager) remoteBranchExists(workerHost, branch string) (bool, error) {
	_, code, err := ssh.Run(workerHost, fmt.Sprintf("git -C %q show-ref --verify --quiet refs/heads/%q", m.settings.Repo.Path, branch))
	if err == nil && code == 0 {
		return true, nil
	}
	if code == 1 {
		return false, nil
	}
	if err == nil {
		err = fmt.Errorf("exit code %d", code)
	}
	return false, fmt.Errorf("check remote worktree branch: %w", err)
}

func worktreeBranch(item model.Item) string {
	return "fizel/" + safeIdentifier(item.Identifier)
}

func shellQuote(v string) string {
	return fmt.Sprintf("%q", v)
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
