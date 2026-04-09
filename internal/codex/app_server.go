package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gmurray/fizel/internal/config"
	"github.com/gmurray/fizel/internal/model"
)

type Event struct {
	Event     string
	Timestamp time.Time
	Payload   map[string]any
}

type Session struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	scanner  *bufio.Scanner
	mu       sync.Mutex
	toolExec *ToolExecutor
	settings config.Settings
}

func StartSession(ctx context.Context, settings config.Settings, cwd string) (*Session, error) {
	cmd := exec.CommandContext(ctx, "bash", "-lc", settings.Codex.Command)
	cmd.Dir = cwd
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 128*1024), 1024*1024)
	session := &Session{
		cmd:      cmd,
		stdin:    stdin,
		scanner:  scanner,
		toolExec: NewToolExecutor(settings.Tracker),
		settings: settings,
	}
	if err := session.send(map[string]any{
		"method": "initialize",
		"id":     1,
		"params": map[string]any{"clientInfo": map[string]any{"name": "fizel"}},
	}); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *Session) Stop() error {
	_ = s.stdin.Close()
	return s.cmd.Process.Kill()
}

func (s *Session) RunTurn(prompt string, item model.Item, onEvent func(Event)) error {
	if err := s.send(map[string]any{
		"method": "thread/start",
		"id":     2,
		"params": map[string]any{
			"approvalPolicy": s.settings.Codex.ApprovalPolicy,
			"sandbox":        s.settings.Codex.ThreadSandbox,
			"cwd":            s.cmd.Dir,
			"dynamicTools":   s.toolExec.ToolSpecs(s.settings.Tracker.Kind),
		},
	}); err != nil {
		return err
	}
	if err := s.send(map[string]any{
		"method": "turn/start",
		"id":     3,
		"params": map[string]any{
			"threadId": "thread",
			"cwd":      s.cmd.Dir,
			"title":    item.Identifier + ": " + item.Title,
			"input":    []map[string]string{{"type": "text", "text": prompt}},
		},
	}); err != nil {
		return err
	}

	for s.scanner.Scan() {
		line := strings.TrimSpace(s.scanner.Text())
		if line == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			onEvent(Event{Event: "malformed", Timestamp: time.Now(), Payload: map[string]any{"line": line}})
			continue
		}
		if method, _ := payload["method"].(string); method != "" {
			onEvent(Event{Event: method, Timestamp: time.Now(), Payload: payload})
			if method == "item/tool/call" {
				params, _ := payload["params"].(map[string]any)
				name, _ := params["tool"].(string)
				if name == "" {
					name, _ = params["name"].(string)
				}
				args := params["arguments"]
				resp := s.toolExec.Execute(name, args)
				_ = s.send(map[string]any{
					"method": "item/tool/result",
					"params": map[string]any{"result": resp},
				})
			}
			if method == "turn/completed" {
				return nil
			}
		}
	}
	if err := s.scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("codex app-server exited before turn completed")
}

func (s *Session) send(payload map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(s.stdin, string(raw))
	return err
}
