package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gmurray/fizel/internal/workflow"
	"gopkg.in/yaml.v3"
)

type Settings struct {
	Tracker       TrackerSettings       `yaml:"tracker"`
	Polling       PollingSettings       `yaml:"polling"`
	Workspace     WorkspaceSettings     `yaml:"workspace"`
	Worker        WorkerSettings        `yaml:"worker"`
	Agent         AgentSettings         `yaml:"agent"`
	Codex         CodexSettings         `yaml:"codex"`
	Hooks         HookSettings          `yaml:"hooks"`
	Observability ObservabilitySettings `yaml:"observability"`
	Server        ServerSettings        `yaml:"server"`
	Repo          RepoSettings          `yaml:"-"`
	Prompt        string
}

type RepoSettings struct {
	Key          string
	Path         string
	WorkflowPath string
}

type TrackerSettings struct {
	Kind           string   `yaml:"kind"`
	APIURL         string   `yaml:"api_url"`
	APIKey         string   `yaml:"api_key"`
	BoardID        string   `yaml:"board_id"`
	Profile        string   `yaml:"profile"`
	Assignee       string   `yaml:"assignee"`
	ActiveStates   []string `yaml:"active_states"`
	TerminalStates []string `yaml:"terminal_states"`
	PostRunState   string   `yaml:"post_run_state"`
}

type PollingSettings struct {
	IntervalMS int `yaml:"interval_ms"`
}

type WorkspaceSettings struct {
	Root string `yaml:"root"`
}

type WorkerSettings struct {
	SSHHosts                   []string `yaml:"ssh_hosts"`
	MaxConcurrentAgentsPerHost int      `yaml:"max_concurrent_agents_per_host"`
}

type AgentSettings struct {
	MaxConcurrentAgents        int            `yaml:"max_concurrent_agents"`
	MaxTurns                   int            `yaml:"max_turns"`
	MaxRetryBackoffMS          int            `yaml:"max_retry_backoff_ms"`
	MaxConcurrentAgentsByState map[string]int `yaml:"max_concurrent_agents_by_state"`
}

type CodexSettings struct {
	Command           string         `yaml:"command"`
	ApprovalPolicy    any            `yaml:"approval_policy"`
	ThreadSandbox     string         `yaml:"thread_sandbox"`
	TurnSandboxPolicy map[string]any `yaml:"turn_sandbox_policy"`
	TurnTimeoutMS     int            `yaml:"turn_timeout_ms"`
	ReadTimeoutMS     int            `yaml:"read_timeout_ms"`
	StallTimeoutMS    int            `yaml:"stall_timeout_ms"`
}

type HookSettings struct {
	AfterCreate  string `yaml:"after_create"`
	BeforeRun    string `yaml:"before_run"`
	AfterRun     string `yaml:"after_run"`
	BeforeRemove string `yaml:"before_remove"`
	TimeoutMS    int    `yaml:"timeout_ms"`
}

type ObservabilitySettings struct {
	DashboardEnabled bool `yaml:"dashboard_enabled"`
	RefreshMS        int  `yaml:"refresh_ms"`
	RenderIntervalMS int  `yaml:"render_interval_ms"`
}

type ServerSettings struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type GlobalConfig struct {
	TrackerDefaults map[string]any `yaml:"tracker_defaults"`
	Settings        map[string]any `yaml:"settings"`
	WatchedRepos    []WatchedRepo  `yaml:"watched_repos"`
}

type WatchedRepo struct {
	Key  string `yaml:"key"`
	Path string `yaml:"path"`
}

type ResolvedRepo struct {
	Key          string
	Path         string
	WorkflowPath string
	Settings     Settings
	Loaded       workflow.Loaded
}

type Registry struct {
	Defaults Settings
	Repos    map[string]ResolvedRepo
}

func FromLoaded(loaded workflow.Loaded) (Settings, error) {
	return Merge(defaultSettings(), loaded)
}

func FromRaw(raw map[string]any) (Settings, error) {
	s := cloneSettings(defaultSettings())
	if err := decodeMapIntoSettings(raw, &s); err != nil {
		return Settings{}, err
	}
	applyEnvFallbacks(&s)
	if err := normalize(&s, false); err != nil {
		return Settings{}, err
	}
	return s, nil
}

func Merge(base Settings, loaded workflow.Loaded) (Settings, error) {
	s := cloneSettings(base)
	if err := decodeMapIntoSettings(loaded.Config, &s); err != nil {
		return Settings{}, err
	}
	s.Prompt = loaded.Prompt
	applyEnvFallbacks(&s)
	if err := normalize(&s, true); err != nil {
		return Settings{}, err
	}
	return s, nil
}

func MergeTrackerOnly(base Settings, loaded workflow.Loaded) (Settings, error) {
	s := cloneSettings(base)
	if err := decodeMapIntoSettings(trackerOnlyConfig(loaded.Config), &s); err != nil {
		return Settings{}, err
	}
	s.Prompt = loaded.Prompt
	applyEnvFallbacks(&s)
	if err := normalize(&s, true); err != nil {
		return Settings{}, err
	}
	return s, nil
}

func defaultSettings() Settings {
	return Settings{
		Tracker: TrackerSettings{
			Kind:           "fizzy",
			APIURL:         "https://app.fizzy.do",
			ActiveStates:   []string{"Todo", "In Progress"},
			TerminalStates: []string{"Done", "Not Now"},
			PostRunState:   "Human Review",
		},
		Polling:   PollingSettings{IntervalMS: 30_000},
		Workspace: WorkspaceSettings{Root: filepath.Join(os.TempDir(), "fizel-workspaces")},
		Worker: WorkerSettings{
			SSHHosts:                   []string{},
			MaxConcurrentAgentsPerHost: 1,
		},
		Agent: AgentSettings{
			MaxConcurrentAgents:        10,
			MaxTurns:                   20,
			MaxRetryBackoffMS:          300_000,
			MaxConcurrentAgentsByState: map[string]int{},
		},
		Codex: CodexSettings{
			Command:        "codex app-server",
			ApprovalPolicy: "never",
			ThreadSandbox:  "workspace-write",
			TurnTimeoutMS:  3_600_000,
			ReadTimeoutMS:  5_000,
			StallTimeoutMS: 300_000,
		},
		Hooks: HookSettings{
			TimeoutMS: 60_000,
		},
		Observability: ObservabilitySettings{
			DashboardEnabled: true,
			RefreshMS:        1_000,
			RenderIntervalMS: 100,
		},
		Server: ServerSettings{
			Host: "127.0.0.1",
		},
	}
}

func decodeMapIntoSettings(raw map[string]any, s *Settings) error {
	if raw == nil {
		return nil
	}
	blob, err := workflowYAML(raw)
	if err != nil {
		return err
	}
	return workflowUnmarshal(blob, s)
}

func workflowYAML(raw map[string]any) ([]byte, error) {
	return yamlMarshal(raw)
}

func trackerOnlyConfig(raw map[string]any) map[string]any {
	if raw == nil {
		return nil
	}
	tracker, ok := raw["tracker"]
	if !ok {
		return nil
	}
	return map[string]any{"tracker": tracker}
}

func yamlMarshal(v any) ([]byte, error)         { return yaml.Marshal(v) }
func workflowUnmarshal(b []byte, out any) error { return yaml.Unmarshal(b, out) }

func applyEnvFallbacks(s *Settings) {
	s.Tracker.APIKey = resolveSecret(s.Tracker.APIKey, "FIZZY_TOKEN")
	s.Workspace.Root = resolvePath(s.Workspace.Root)
}

func normalize(s *Settings, requireBoardID bool) error {
	s.Tracker.Kind = strings.TrimSpace(strings.ToLower(s.Tracker.Kind))
	s.Tracker.PostRunState = strings.TrimSpace(s.Tracker.PostRunState)
	s.Workspace.Root = filepath.Clean(s.Workspace.Root)
	s.Repo.Key = normalizeRepoKey(s.Repo.Key)
	s.Repo.Path = resolvePath(s.Repo.Path)
	s.Repo.WorkflowPath = resolvePath(s.Repo.WorkflowPath)
	if s.Tracker.Kind == "" {
		return errors.New("tracker.kind is required")
	}
	if s.Tracker.Kind != "fizzy" && s.Tracker.Kind != "memory" {
		return fmt.Errorf("unsupported tracker.kind %q", s.Tracker.Kind)
	}
	if s.Tracker.Kind == "fizzy" {
		if strings.TrimSpace(s.Tracker.APIKey) == "" {
			return errors.New("fizzy tracker requires tracker.api_key or FIZZY_TOKEN")
		}
		if requireBoardID && strings.TrimSpace(s.Tracker.BoardID) == "" {
			return errors.New("fizzy tracker requires tracker.board_id")
		}
	}
	if s.Polling.IntervalMS <= 0 || s.Agent.MaxConcurrentAgents <= 0 || s.Agent.MaxTurns <= 0 {
		return errors.New("polling and agent concurrency settings must be positive")
	}
	if s.Hooks.TimeoutMS <= 0 || s.Codex.TurnTimeoutMS <= 0 || s.Codex.ReadTimeoutMS <= 0 {
		return errors.New("hook and codex timeout settings must be positive")
	}
	if strings.TrimSpace(s.Codex.Command) == "" {
		return errors.New("codex.command is required")
	}
	for k, v := range s.Agent.MaxConcurrentAgentsByState {
		delete(s.Agent.MaxConcurrentAgentsByState, k)
		s.Agent.MaxConcurrentAgentsByState[normalizeState(k)] = v
	}
	return nil
}

func normalizeRepoKey(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func cloneSettings(s Settings) Settings {
	out := s
	out.Tracker.ActiveStates = append([]string{}, s.Tracker.ActiveStates...)
	out.Tracker.TerminalStates = append([]string{}, s.Tracker.TerminalStates...)
	out.Worker.SSHHosts = append([]string{}, s.Worker.SSHHosts...)
	if s.Agent.MaxConcurrentAgentsByState != nil {
		out.Agent.MaxConcurrentAgentsByState = make(map[string]int, len(s.Agent.MaxConcurrentAgentsByState))
		for k, v := range s.Agent.MaxConcurrentAgentsByState {
			out.Agent.MaxConcurrentAgentsByState[k] = v
		}
	}
	return out
}

func normalizeState(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func resolveSecret(value, env string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "$"+env {
		return os.Getenv(env)
	}
	if strings.HasPrefix(value, "$") {
		return os.Getenv(strings.TrimPrefix(value, "$"))
	}
	return value
}

func resolvePath(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "$") {
		value = os.Getenv(strings.TrimPrefix(value, "$"))
	}
	if strings.HasPrefix(value, "~") {
		home, _ := os.UserHomeDir()
		if home != "" {
			value = filepath.Join(home, strings.TrimPrefix(value, "~/"))
		}
	}
	if value == "" {
		return value
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return value
	}
	return abs
}

func DefaultGlobalPath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "config.yaml"
	}
	return filepath.Join(home, ".config", "fizel", "config.yaml")
}

func LoadRegistry(path string) (Registry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Registry{}, fmt.Errorf("read config: %w", err)
	}
	var top map[string]any
	if err := yaml.Unmarshal(content, &top); err != nil {
		return Registry{}, fmt.Errorf("parse config: %w", err)
	}
	if _, ok := top["defaults"]; ok {
		return Registry{}, errors.New("config.defaults is no longer supported; use tracker_defaults and settings")
	}
	var raw GlobalConfig
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return Registry{}, fmt.Errorf("parse config: %w", err)
	}
	if _, ok := raw.Settings["tracker"]; ok {
		return Registry{}, errors.New("config.settings.tracker is not allowed; use tracker_defaults")
	}
	defaults, err := FromRaw(globalDefaultsConfig(raw.Settings, raw.TrackerDefaults))
	if err != nil {
		return Registry{}, fmt.Errorf("load defaults: %w", err)
	}
	if len(raw.WatchedRepos) == 0 {
		return Registry{}, errors.New("config must define at least one watched_repo")
	}
	repos := make(map[string]ResolvedRepo, len(raw.WatchedRepos))
	for _, watched := range raw.WatchedRepos {
		key := normalizeRepoKey(watched.Key)
		if key == "" {
			return Registry{}, errors.New("watched repo key is required")
		}
		if _, exists := repos[key]; exists {
			return Registry{}, fmt.Errorf("duplicate watched repo key %q", key)
		}
		repoPath := resolvePath(watched.Path)
		if strings.TrimSpace(repoPath) == "" {
			return Registry{}, fmt.Errorf("watched repo %q path is required", key)
		}
		info, err := os.Stat(repoPath)
		if err != nil {
			return Registry{}, fmt.Errorf("stat watched repo %q: %w", key, err)
		}
		if !info.IsDir() {
			return Registry{}, fmt.Errorf("watched repo %q path is not a directory", key)
		}
		workflowPath := filepath.Join(repoPath, "WORKFLOW.md")
		loaded, err := workflow.Load(workflowPath)
		if err != nil {
			return Registry{}, fmt.Errorf("load workflow for repo %q: %w", key, err)
		}
		settings, err := MergeTrackerOnly(defaults, loaded)
		if err != nil {
			return Registry{}, fmt.Errorf("merge workflow for repo %q: %w", key, err)
		}
		settings.Repo = RepoSettings{
			Key:          key,
			Path:         repoPath,
			WorkflowPath: workflowPath,
		}
		repos[key] = ResolvedRepo{
			Key:          key,
			Path:         repoPath,
			WorkflowPath: workflowPath,
			Settings:     settings,
			Loaded:       loaded,
		}
	}
	return Registry{Defaults: defaults, Repos: repos}, nil
}

func globalDefaultsConfig(settings map[string]any, trackerDefaults map[string]any) map[string]any {
	raw := map[string]any{}
	for k, v := range settings {
		raw[k] = v
	}
	if trackerDefaults != nil {
		raw["tracker"] = trackerDefaults
	}
	return raw
}
