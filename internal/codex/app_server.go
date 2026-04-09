package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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

const nonInteractiveToolInputAnswer = "This is a non-interactive session. Operator input is unavailable."

type Session struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	scanner  *bufio.Scanner
	mu       sync.Mutex
	toolExec *ToolExecutor
	settings config.Settings
	nextID   int
	threadID string
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
		nextID:   1,
	}
	initID := session.newRequestID()
	if err := session.send(map[string]any{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"id":      initID,
		"params": map[string]any{
			"clientInfo": map[string]any{
				"name":    "fizel",
				"title":   "Fizel",
				"version": "0.1.0",
			},
			"capabilities": map[string]any{
				"experimentalApi": true,
			},
		},
	}); err != nil {
		return nil, err
	}
	if _, err := session.awaitResponse(initID, nil); err != nil {
		return nil, err
	}
	if err := session.send(map[string]any{
		"jsonrpc": "2.0",
		"method":  "initialized",
		"params":  map[string]any{},
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
	if s.threadID == "" {
		threadReqID := s.newRequestID()
		if err := s.send(map[string]any{
			"jsonrpc": "2.0",
			"method":  "thread/start",
			"id":      threadReqID,
			"params": map[string]any{
				"approvalPolicy":         s.settings.Codex.ApprovalPolicy,
				"sandbox":                s.settings.Codex.ThreadSandbox,
				"cwd":                    s.cmd.Dir,
				"dynamicTools":           s.toolExec.ToolSpecs(s.settings.Tracker.Kind),
				"experimentalRawEvents":  false,
				"persistExtendedHistory": true,
			},
		}); err != nil {
			return err
		}
		threadResponse, err := s.awaitResponse(threadReqID, onEvent)
		if err != nil {
			return err
		}
		s.threadID = extractThreadID(threadResponse)
		if s.threadID == "" {
			return fmt.Errorf("thread/start did not return a thread id")
		}
	}

	turnReqID := s.newRequestID()
	if err := s.send(map[string]any{
		"jsonrpc": "2.0",
		"method":  "turn/start",
		"id":      turnReqID,
		"params": map[string]any{
			"threadId":       s.threadID,
			"input":          []map[string]string{{"type": "text", "text": prompt}},
			"cwd":            s.cmd.Dir,
			"title":          item.Identifier + ": " + item.Title,
			"approvalPolicy": s.settings.Codex.ApprovalPolicy,
			"sandboxPolicy":  s.settings.Codex.TurnSandboxPolicy,
		},
	}); err != nil {
		return err
	}
	if _, err := s.awaitResponse(turnReqID, onEvent); err != nil {
		return err
	}

	return s.streamTurn(onEvent)
}

func (s *Session) streamTurn(onEvent func(Event)) error {
	for s.scanner.Scan() {
		line := strings.TrimSpace(s.scanner.Text())
		if line == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			s.emitEvent(onEvent, Event{Event: "malformed", Timestamp: time.Now(), Payload: map[string]any{"line": line}})
			continue
		}
		if done, err := s.handleMessage(payload, onEvent); err != nil {
			return err
		} else if done {
			return nil
		}
	}
	if err := s.scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("codex app-server exited before turn completed")
}

func (s *Session) awaitResponse(id int, onEvent func(Event)) (map[string]any, error) {
	for s.scanner.Scan() {
		line := strings.TrimSpace(s.scanner.Text())
		if line == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			s.emitEvent(onEvent, Event{Event: "malformed", Timestamp: time.Now(), Payload: map[string]any{"line": line}})
			continue
		}
		if responseID, ok := payload["id"]; ok && requestIDMatches(responseID, id) {
			if errBody, ok := payload["error"].(map[string]any); ok {
				return nil, fmt.Errorf("%v", errBody["message"])
			}
			result, _ := payload["result"].(map[string]any)
			return result, nil
		}
		if _, err := s.handleMessage(payload, onEvent); err != nil {
			return nil, err
		}
	}
	if err := s.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("codex app-server exited before responding to request %d", id)
}

func (s *Session) handleMessage(payload map[string]any, onEvent func(Event)) (bool, error) {
	method, _ := payload["method"].(string)
	if method == "" {
		return false, nil
	}
	s.emitEvent(onEvent, Event{Event: method, Timestamp: time.Now(), Payload: payload})
	switch method {
	case "turn/input_required":
		return false, fmt.Errorf("turn/input_required")
	case "item/tool/call":
		params, _ := payload["params"].(map[string]any)
		name, _ := params["tool"].(string)
		if name == "" {
			name, _ = params["name"].(string)
		}
		args := params["arguments"]
		resp := s.toolExec.Execute(name, args)
		reqID := payload["id"]
		if err := s.send(map[string]any{
			"jsonrpc": "2.0",
			"id":      reqID,
			"result":  resp,
		}); err != nil {
			return false, err
		}
		return false, nil
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		return false, s.handleApprovalRequest(payload, "acceptForSession")
	case "execCommandApproval", "applyPatchApproval":
		return false, s.handleApprovalRequest(payload, "approved_for_session")
	case "item/tool/requestUserInput":
		return false, s.handleToolRequestUserInput(payload)
	}
	return method == "turn/completed", nil
}

func (s *Session) handleApprovalRequest(payload map[string]any, decision string) error {
	if !autoApprove(s.settings.Codex.ApprovalPolicy) {
		return fmt.Errorf("approval required: %s", payload["method"])
	}
	return s.send(map[string]any{
		"jsonrpc": "2.0",
		"id":      payload["id"],
		"result":  map[string]any{"decision": decision},
	})
}

func (s *Session) handleToolRequestUserInput(payload map[string]any) error {
	params, _ := payload["params"].(map[string]any)
	answers, err := toolRequestAnswers(params, autoApprove(s.settings.Codex.ApprovalPolicy))
	if err != nil {
		return fmt.Errorf("turn/input_required")
	}
	return s.send(map[string]any{
		"jsonrpc": "2.0",
		"id":      payload["id"],
		"result":  map[string]any{"answers": answers},
	})
}

func autoApprove(policy any) bool {
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(policy)), "never")
}

func toolRequestAnswers(params map[string]any, approve bool) (map[string]any, error) {
	questions, _ := params["questions"].([]any)
	if len(questions) == 0 {
		return nil, errors.New("missing questions")
	}
	answers := map[string]any{}
	for _, rawQuestion := range questions {
		question, _ := rawQuestion.(map[string]any)
		id, _ := question["id"].(string)
		if strings.TrimSpace(id) == "" {
			return nil, errors.New("missing question id")
		}
		answer := nonInteractiveToolInputAnswer
		if approve {
			if approvedLabel := approvalOptionLabel(question); approvedLabel != "" {
				answer = approvedLabel
			}
		}
		answers[id] = map[string]any{"answers": []string{answer}}
	}
	return answers, nil
}

func approvalOptionLabel(question map[string]any) string {
	options, _ := question["options"].([]any)
	labels := make([]string, 0, len(options))
	for _, rawOption := range options {
		option, _ := rawOption.(map[string]any)
		label, _ := option["label"].(string)
		if strings.TrimSpace(label) != "" {
			labels = append(labels, label)
		}
	}
	for _, preferred := range []string{"Approve this Session", "Approve Once"} {
		for _, label := range labels {
			if label == preferred {
				return label
			}
		}
	}
	for _, label := range labels {
		lower := strings.ToLower(label)
		if strings.Contains(lower, "approve") || strings.Contains(lower, "allow") || strings.Contains(lower, "accept") {
			return label
		}
	}
	return ""
}

func (s *Session) emitEvent(onEvent func(Event), event Event) {
	if onEvent != nil {
		onEvent(event)
	}
}

func (s *Session) newRequestID() int {
	id := s.nextID
	s.nextID++
	return id
}

func requestIDMatches(value any, want int) bool {
	switch v := value.(type) {
	case float64:
		return int(v) == want
	case int:
		return v == want
	default:
		return false
	}
}

func extractThreadID(result map[string]any) string {
	thread, _ := result["thread"].(map[string]any)
	id, _ := thread["id"].(string)
	return strings.TrimSpace(id)
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
