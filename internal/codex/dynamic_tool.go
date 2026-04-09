package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gmurray/fizel/internal/config"
	"github.com/gmurray/fizel/internal/tracker/fizzy"
)

type ToolExecutor struct {
	fizzy *fizzy.Tracker
}

func NewToolExecutor(settings config.TrackerSettings) *ToolExecutor {
	return &ToolExecutor{fizzy: fizzy.NewFromSettings(settings)}
}

type ToolResponse struct {
	Success      bool                `json:"success"`
	Output       string              `json:"output"`
	ContentItems []map[string]string `json:"contentItems"`
}

func (e *ToolExecutor) Execute(name string, args any) ToolResponse {
	switch name {
	case "fizzy":
		return e.executeFizzy(args)
	default:
		return failure(fmt.Sprintf("unsupported dynamic tool: %s", name))
	}
}

func (e *ToolExecutor) ToolSpecs(kind string) []map[string]any {
	if kind != "fizzy" {
		return nil
	}
	return []map[string]any{{
		"name":        "fizzy",
		"description": "Execute a Fizzy CLI command using configured auth.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"argv"},
			"properties": map[string]any{
				"argv": map[string]any{
					"type":        "array",
					"minItems":    1,
					"items":       map[string]any{"type": "string"},
					"description": "Fizzy command arguments.",
				},
			},
		},
	}}
}

func (e *ToolExecutor) executeFizzy(args any) ToolResponse {
	argv, err := normalizeArgv(args)
	if err != nil {
		return failure(err.Error())
	}
	payload, err := e.fizzyRunner(argv)
	if err != nil {
		return failure(err.Error())
	}
	return success(payload)
}

func (e *ToolExecutor) fizzyRunner(argv []string) (any, error) {
	blocked := map[string]struct{}{"auth": {}, "setup": {}, "signup": {}}
	if _, ok := blocked[argv[0]]; ok {
		return nil, errors.New("interactive fizzy commands are blocked")
	}
	// Reuse the tracker runner path by issuing a harmless list/show style command.
	// The tracker methods cover the orchestration use-cases; dynamic tool access
	// needs raw argv passthrough for agent turns.
	type rawRunner interface {
		RunRaw(args []string) (any, error)
	}
	if passthrough, ok := any(e.fizzy).(rawRunner); ok {
		return passthrough.RunRaw(argv)
	}
	return nil, errors.New("fizzy raw tool passthrough unavailable")
}

func normalizeArgv(args any) ([]string, error) {
	row, ok := args.(map[string]any)
	if !ok {
		return nil, errors.New("fizzy expects an object with argv")
	}
	list, ok := row["argv"].([]any)
	if !ok || len(list) == 0 {
		return nil, errors.New("fizzy requires a non-empty argv array")
	}
	out := make([]string, 0, len(list))
	for _, value := range list {
		s, ok := value.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, errors.New("fizzy argv entries must be non-empty strings")
		}
		out = append(out, s)
	}
	return out, nil
}

func success(payload any) ToolResponse {
	raw, _ := json.MarshalIndent(payload, "", "  ")
	text := string(raw)
	return ToolResponse{
		Success: true,
		Output:  text,
		ContentItems: []map[string]string{{
			"type": "inputText",
			"text": text,
		}},
	}
}

func failure(message string) ToolResponse {
	return ToolResponse{
		Success: false,
		Output:  message,
		ContentItems: []map[string]string{{
			"type": "inputText",
			"text": message,
		}},
	}
}
