package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmurray/fizel/internal/config"
	"github.com/gmurray/fizel/internal/model"
)

func TestSessionInitializeAndRunTurnSendsExpectedProtocol(t *testing.T) {
	traceFile, settings := makeFakeAppServer(t, `#!/bin/sh
trace_file="${TRACE_FILE}"
count=0
while IFS= read -r line; do
  count=$((count + 1))
  printf '%s\n' "$line" >> "$trace_file"
  case "$count" in
    1)
      printf '%s\n' '{"id":1,"result":{}}'
      ;;
    2)
      ;;
    3)
      printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-1"}}}'
      ;;
    4)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-1"}}}'
      printf '%s\n' '{"method":"turn/completed"}'
      exit 0
      ;;
  esac
done
`)
	settings.Codex.TurnSandboxPolicy = map[string]any{
		"type":          "workspace-write",
		"networkAccess": true,
	}

	session := startTestSession(t, settings)
	defer session.Stop()

	if err := session.RunTurn("Handle it", testItem(), nil); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	lines := readTraceLines(t, traceFile)
	if len(lines) != 4 {
		t.Fatalf("expected 4 protocol messages, got %d", len(lines))
	}

	initialize := decodeJSONLine(t, lines[0])
	assertEqual(t, initialize["method"], "initialize")
	assertEqual(t, getInMap(initialize, "params", "clientInfo", "version"), "0.1.0")
	assertEqual(t, getInMap(initialize, "params", "capabilities", "experimentalApi"), true)

	initialized := decodeJSONLine(t, lines[1])
	assertEqual(t, initialized["method"], "initialized")

	threadStart := decodeJSONLine(t, lines[2])
	assertEqual(t, threadStart["method"], "thread/start")
	if got := getInMap(threadStart, "params", "dynamicTools"); got == nil {
		t.Fatalf("expected dynamicTools in thread/start params")
	}

	turnStart := decodeJSONLine(t, lines[3])
	assertEqual(t, turnStart["method"], "turn/start")
	assertEqual(t, getInMap(turnStart, "params", "threadId"), "thread-1")
	assertEqual(t, getInMap(turnStart, "params", "approvalPolicy"), "never")
	assertEqual(t, getInMap(turnStart, "params", "sandboxPolicy", "type"), "workspace-write")
	assertEqual(t, getInMap(turnStart, "params", "sandboxPolicy", "networkAccess"), true)
}

func TestRunTurnReturnsInputRequiredError(t *testing.T) {
	_, settings := makeFakeAppServer(t, `#!/bin/sh
count=0
while IFS= read -r _line; do
  count=$((count + 1))
  case "$count" in
    1)
      printf '%s\n' '{"id":1,"result":{}}'
      ;;
    2)
      ;;
    3)
      printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-2"}}}'
      ;;
    4)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-2"}}}'
      printf '%s\n' '{"method":"turn/input_required","params":{"reason":"blocked"}}'
      exit 0
      ;;
  esac
done
`)

	session := startTestSession(t, settings)
	defer session.Stop()

	err := session.RunTurn("Handle it", testItem(), nil)
	if err == nil || !strings.Contains(err.Error(), "turn/input_required") {
		t.Fatalf("expected turn/input_required error, got %v", err)
	}
}

func TestRunTurnAutoApprovesCommandExecutionRequestsWhenPolicyIsNever(t *testing.T) {
	traceFile, settings := makeFakeAppServer(t, `#!/bin/sh
trace_file="${TRACE_FILE}"
count=0
while IFS= read -r line; do
  count=$((count + 1))
  printf '%s\n' "$line" >> "$trace_file"
  case "$count" in
    1)
      printf '%s\n' '{"id":1,"result":{}}'
      ;;
    2)
      ;;
    3)
      printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-3"}}}'
      ;;
    4)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-3"}}}'
      printf '%s\n' '{"id":99,"method":"item/commandExecution/requestApproval","params":{"command":"gh pr view"}}'
      ;;
    5)
      printf '%s\n' '{"method":"turn/completed"}'
      exit 0
      ;;
  esac
done
`)

	session := startTestSession(t, settings)
	defer session.Stop()

	if err := session.RunTurn("Handle it", testItem(), nil); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	lines := readTraceLines(t, traceFile)
	found := false
	for _, line := range lines {
		payload := decodeJSONLine(t, line)
		if payload["id"] == float64(99) && getInMap(payload, "result", "decision") == "acceptForSession" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected approval response for request 99 in trace")
	}
}

func TestRunTurnFailsApprovalRequestsWhenPolicyIsNotNever(t *testing.T) {
	_, settings := makeFakeAppServer(t, `#!/bin/sh
count=0
while IFS= read -r _line; do
  count=$((count + 1))
  case "$count" in
    1)
      printf '%s\n' '{"id":1,"result":{}}'
      ;;
    2)
      ;;
    3)
      printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-4"}}}'
      ;;
    4)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-4"}}}'
      printf '%s\n' '{"id":44,"method":"item/fileChange/requestApproval","params":{"reason":"write file"}}'
      exit 0
      ;;
  esac
done
`)
	settings.Codex.ApprovalPolicy = "on-request"

	session := startTestSession(t, settings)
	defer session.Stop()

	err := session.RunTurn("Handle it", testItem(), nil)
	if err == nil || !strings.Contains(err.Error(), "approval required") {
		t.Fatalf("expected approval required error, got %v", err)
	}
}

func TestRunTurnRepliesToToolCalls(t *testing.T) {
	traceFile, settings := makeFakeAppServer(t, `#!/bin/sh
trace_file="${TRACE_FILE}"
count=0
while IFS= read -r line; do
  count=$((count + 1))
  printf '%s\n' "$line" >> "$trace_file"
  case "$count" in
    1)
      printf '%s\n' '{"id":1,"result":{}}'
      ;;
    2)
      ;;
    3)
      printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-5"}}}'
      ;;
    4)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-5"}}}'
      printf '%s\n' '{"id":77,"method":"item/tool/call","params":{"name":"missing","arguments":{"foo":"bar"}}}'
      ;;
    5)
      printf '%s\n' '{"method":"turn/completed"}'
      exit 0
      ;;
  esac
done
`)

	session := startTestSession(t, settings)
	defer session.Stop()

	if err := session.RunTurn("Handle it", testItem(), nil); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	lines := readTraceLines(t, traceFile)
	found := false
	for _, line := range lines {
		payload := decodeJSONLine(t, line)
		if payload["id"] == float64(77) && getInMap(payload, "result", "success") == false {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected tool result for request 77 in trace")
	}
}

func TestRunTurnAutoApprovesToolRequestUserInputWhenPolicyIsNever(t *testing.T) {
	traceFile, settings := makeFakeAppServer(t, `#!/bin/sh
trace_file="${TRACE_FILE}"
count=0
while IFS= read -r line; do
  count=$((count + 1))
  printf '%s\n' "$line" >> "$trace_file"
  case "$count" in
    1)
      printf '%s\n' '{"id":1,"result":{}}'
      ;;
    2)
      ;;
    3)
      printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-6"}}}'
      ;;
    4)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-6"}}}'
      printf '%s\n' '{"id":110,"method":"item/tool/requestUserInput","params":{"questions":[{"id":"approval","options":[{"label":"Approve Once"},{"label":"Approve this Session"},{"label":"Deny"}]}]}}'
      ;;
    5)
      printf '%s\n' '{"method":"turn/completed"}'
      exit 0
      ;;
  esac
done
`)

	session := startTestSession(t, settings)
	defer session.Stop()

	if err := session.RunTurn("Handle it", testItem(), nil); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	lines := readTraceLines(t, traceFile)
	found := false
	for _, line := range lines {
		payload := decodeJSONLine(t, line)
		if payload["id"] == float64(110) && getInMap(payload, "result", "answers", "approval", "answers") != nil {
			answers, _ := getInMap(payload, "result", "answers", "approval", "answers").([]any)
			if len(answers) == 1 && answers[0] == "Approve this Session" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected tool request approval answer in trace")
	}
}

func startTestSession(t *testing.T, settings config.Settings) *Session {
	t.Helper()
	session, err := StartSession(context.Background(), settings, t.TempDir())
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	return session
}

func makeFakeAppServer(t *testing.T, script string) (string, config.Settings) {
	t.Helper()
	root := t.TempDir()
	traceFile := filepath.Join(root, "trace.jsonl")
	scriptPath := filepath.Join(root, "fake-codex.sh")
	script = strings.ReplaceAll(script, "${TRACE_FILE}", traceFile)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex script: %v", err)
	}
	settings := config.Settings{
		Tracker: config.TrackerSettings{
			Kind:    "fizzy",
			APIKey:  "token",
			BoardID: "board-1",
		},
		Codex: config.CodexSettings{
			Command:        scriptPath,
			ApprovalPolicy: "never",
			ThreadSandbox:  "workspace-write",
		},
	}
	return traceFile, settings
}

func readTraceLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func decodeJSONLine(t *testing.T, line string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("decode json line %q: %v", line, err)
	}
	return payload
}

func getInMap(payload map[string]any, path ...string) any {
	current := any(payload)
	for _, key := range path {
		next, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = next[key]
	}
	return current
}

func assertEqual(t *testing.T, got, want any) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func testItem() model.Item {
	return model.Item{
		ID:         "board-1:42",
		Identifier: "board-1:42",
		Title:      "Test item",
		State:      "In Progress",
	}
}
