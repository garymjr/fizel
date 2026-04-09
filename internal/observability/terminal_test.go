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
		TrackerHeader: "Fizzy board board-1",
	}, time.Unix(1_700_000_000, 0))

	rendered := stripANSI(strings.Join(lines, "\n"))
	assertContains(t, rendered, "╭─ FIZEL STATUS")
	assertContains(t, rendered, "│ Agents: 0/10")
	assertContains(t, rendered, "│ Tracker: Fizzy board board-1")
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
		Polling:       true,
		TrackerHeader: "Fizzy board board-1",
		LastEvent:     "dispatching",
		Running: []RunningItem{{
			Identifier: "board-1:42",
			State:      "In Progress",
			StartedAt:  now.Add(-12 * time.Second),
		}},
		Retrying: []RetryItem{{
			Identifier: "board-1:9",
			Attempt:    2,
			RetryAt:    now.Add(18 * time.Second),
		}},
	}, now)

	rendered := stripANSI(strings.Join(lines, "\n"))
	assertContains(t, rendered, "│ Agents: 1/4")
	assertContains(t, rendered, "│ Next refresh: checking now...")
	assertContains(t, rendered, "board-1:42")
	assertContains(t, rendered, "dispatching")
	assertContains(t, rendered, "board-1:9")
	assertContains(t, rendered, "18s")
}

func TestTerminalFormatIncludesANSIColors(t *testing.T) {
	term := NewTerminalForWriter(config.Settings{
		Polling: config.PollingSettings{IntervalMS: 5_000},
		Agent:   config.AgentSettings{MaxConcurrentAgents: 4},
	}, nil)

	rendered := strings.Join(term.format(Snapshot{
		TrackerHeader: "Fizzy board board-1",
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
