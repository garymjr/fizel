package observability

import (
	"strings"
	"testing"
	"time"

	"github.com/gmurray/fizel/internal/config"
)

func TestTerminalFormatIdleSnapshot(t *testing.T) {
	term := NewTerminalForWriter(config.Settings{
		Polling: config.PollingSettings{IntervalMS: 30_000},
		Agent:   config.AgentSettings{MaxConcurrentAgents: 10},
	}, nil)

	lines := term.format(Snapshot{
		TrackerMode: "fizzy single workflow",
	}, time.Unix(1_700_000_000, 0))

	rendered := stripANSI(strings.Join(lines, "\n"))
	assertContains(t, rendered, "╭─ FIZEL STATUS")
	assertContains(t, rendered, "│ Agents: 0/10")
	assertContains(t, rendered, "│ Tracker: fizzy single workflow")
	assertContains(t, rendered, "│ Repos: single workflow")
	assertContains(t, rendered, "│ Next refresh: 30s")
	assertContains(t, rendered, "├─ Running")
	assertContains(t, rendered, "│ No active agents")
	assertContains(t, rendered, "├─ Backoff queue")
	assertContains(t, rendered, "│ No queued retries")
}

func TestTerminalFormatActiveSnapshot(t *testing.T) {
	term := NewTerminalForWriter(config.Settings{
		Polling: config.PollingSettings{IntervalMS: 5_000},
		Agent:   config.AgentSettings{MaxConcurrentAgents: 4},
	}, nil)
	now := time.Unix(1_700_000_000, 0)

	lines := term.format(Snapshot{
		Polling:     true,
		TrackerMode: "fizzy watched repos",
		WatchedRepos: []WatchedRepoStatus{
			{Key: "api", BoardID: "board-1"},
			{Key: "web", BoardID: "board-2"},
		},
		Running: []RunningItem{{
			Identifier: "board-1:42",
			RepoKey:    "api",
			State:      "In Progress",
			StartedAt:  now.Add(-12 * time.Second),
			LastEvent:  "dispatching",
		}},
		Retrying: []RetryItem{{
			Identifier: "board-1:9",
			RepoKey:    "web",
			Attempt:    2,
			RetryAt:    now.Add(18 * time.Second),
		}},
	}, now)

	rendered := stripANSI(strings.Join(lines, "\n"))
	assertContains(t, rendered, "│ Agents: 1/4")
	assertContains(t, rendered, "│ Tracker: fizzy watched repos")
	assertContains(t, rendered, "│ Repos: 2 watched (api -> board-1, web -> board-2)")
	assertContains(t, rendered, "│ Next refresh: checking now...")
	assertContains(t, rendered, "board-1:42")
	assertContains(t, rendered, "dispatching")
	assertContains(t, rendered, "board-1:9")
	assertContains(t, rendered, "18s")
	assertContains(t, rendered, "api")
	assertContains(t, rendered, "web")
}

func TestTerminalFormatIncludesANSIColors(t *testing.T) {
	term := NewTerminalForWriter(config.Settings{
		Polling: config.PollingSettings{IntervalMS: 5_000},
		Agent:   config.AgentSettings{MaxConcurrentAgents: 4},
	}, nil)

	rendered := strings.Join(term.format(Snapshot{
		TrackerMode: "fizzy single workflow",
	}, time.Unix(1_700_000_000, 0)), "\n")

	assertContains(t, rendered, ansiBold)
	assertContains(t, rendered, ansiCyan)
	assertContains(t, rendered, ansiGray)
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q\n\n%s", want, got)
	}
}
