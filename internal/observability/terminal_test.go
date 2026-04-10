package observability

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gmurray/fizel/internal/config"
)

func TestTerminalFormatIdleSnapshot(t *testing.T) {
	term := NewTerminalForWriter(config.Settings{
		Polling: config.PollingSettings{IntervalMS: 30_000},
		Agent:   config.AgentSettings{MaxConcurrentAgents: 10},
	}, nil)

	rendered := stripANSI(term.format(Snapshot{
		TrackerMode: "fizzy single workflow",
	}, time.Unix(1_700_000_000, 0), 110))

	assertContains(t, rendered, "FIZEL")
	assertContains(t, rendered, "read-only orchestration dashboard")
	assertContains(t, rendered, "TRACKER: fizzy single workflow")
	assertContains(t, rendered, "AGENTS: 0/10")
	assertContains(t, rendered, "REPOS: single workflow")
	assertContains(t, rendered, "REFRESH: 30s")
	assertContains(t, rendered, "Running Agents  0")
	assertContains(t, rendered, "No active agents.")
	assertContains(t, rendered, "Backoff Queue  0")
	assertContains(t, rendered, "No queued retries.")
}

func TestTerminalFormatWideSnapshotShowsSideBySideTables(t *testing.T) {
	term := NewTerminalForWriter(config.Settings{
		Polling: config.PollingSettings{IntervalMS: 5_000},
		Agent:   config.AgentSettings{MaxConcurrentAgents: 4},
	}, nil)
	now := time.Unix(1_700_000_000, 0)

	rendered := stripANSI(term.format(Snapshot{
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
	}, now, 140))

	assertContains(t, rendered, "TRACKER: fizzy watched repos")
	assertContains(t, rendered, "REPOS: 2 watched")
	assertContains(t, rendered, "REFRESH: checking now")
	assertContains(t, rendered, "REPO")
	assertContains(t, rendered, "EVENT")
	assertContains(t, rendered, "dispatching")
	assertContains(t, rendered, "board-1:42")
	assertContains(t, rendered, "board-1:9")
	assertContains(t, rendered, "18s")
	assertContains(t, rendered, "Recent Logs  0")
}

func TestTerminalFormatShowsLiveRefreshCountdown(t *testing.T) {
	term := NewTerminalForWriter(config.Settings{
		Polling: config.PollingSettings{IntervalMS: 5_000},
		Agent:   config.AgentSettings{MaxConcurrentAgents: 4},
	}, nil)
	lastRefreshAt := time.Unix(1_700_000_000, 0)

	rendered := stripANSI(term.format(Snapshot{
		LastRefreshAt: lastRefreshAt,
	}, lastRefreshAt.Add(1100*time.Millisecond), 110))
	assertContains(t, rendered, "REFRESH: 4s")

	rendered = stripANSI(term.format(Snapshot{
		LastRefreshAt: lastRefreshAt,
	}, lastRefreshAt.Add(4100*time.Millisecond), 110))
	assertContains(t, rendered, "REFRESH: 1s")
}

func TestTerminalFormatNarrowSnapshotStacksPanelsAndDropsLowValueColumns(t *testing.T) {
	term := NewTerminalForWriter(config.Settings{
		Polling: config.PollingSettings{IntervalMS: 5_000},
		Agent:   config.AgentSettings{MaxConcurrentAgents: 4},
	}, nil)
	now := time.Unix(1_700_000_000, 0)

	rendered := stripANSI(term.format(Snapshot{
		Running: []RunningItem{{
			Identifier: "board-1:42-with-a-very-long-identifier",
			RepoKey:    "api",
			State:      "In Progress",
			StartedAt:  now.Add(-12 * time.Second),
			LastEvent:  "dispatching-agent-with-verbose-event-name",
		}},
		Retrying: []RetryItem{{
			Identifier: "board-1:9",
			RepoKey:    "web",
			Attempt:    2,
			RetryAt:    now.Add(18 * time.Second),
		}},
	}, now, 72))

	assertContains(t, rendered, "Running Agents  1")
	assertContains(t, rendered, "Backoff Queue  1")
	assertContains(t, rendered, "ID")
	assertContains(t, rendered, "STATE")
	assertNotContains(t, rendered, "EVENT")
	assertContains(t, rendered, "dispatching-agent-with-verbose-event-name")
}

func TestTerminalFormatTruncatesLongFields(t *testing.T) {
	term := NewTerminalForWriter(config.Settings{
		Polling: config.PollingSettings{IntervalMS: 5_000},
		Agent:   config.AgentSettings{MaxConcurrentAgents: 4},
	}, nil)
	now := time.Unix(1_700_000_000, 0)

	rendered := stripANSI(term.format(Snapshot{
		Running: []RunningItem{{
			Identifier: "board-1:42-with-a-very-long-identifier-that-should-truncate",
			RepoKey:    "api-service-with-very-long-name",
			State:      "Human Review",
			StartedAt:  now.Add(-72 * time.Second),
			LastEvent:  "dispatching-agent-with-an-extremely-verbose-event-name",
		}},
	}, now, 96))

	assertContains(t, rendered, "board-1:42-with-a-v…")
	assertContains(t, rendered, "api-servi…")
	assertContains(t, rendered, "dispatching-agent-with-an-extre…")
}

func TestTerminalFormatKeepsRightBordersAligned(t *testing.T) {
	term := NewTerminalForWriter(config.Settings{
		Polling: config.PollingSettings{IntervalMS: 5_000},
		Agent:   config.AgentSettings{MaxConcurrentAgents: 4},
	}, nil)
	now := time.Unix(1_700_000_000, 0)

	rendered := stripANSI(term.format(Snapshot{
		TrackerMode: "fizzy watched repos",
		WatchedRepos: []WatchedRepoStatus{
			{Key: "api", BoardID: "board-1"},
			{Key: "web", BoardID: "board-2"},
		},
	}, now, 140))

	assertRightBordersAligned(t, rendered)
}

func TestTerminalFormatShowsRecentLogsPanel(t *testing.T) {
	term := NewTerminalForWriter(config.Settings{
		Polling: config.PollingSettings{IntervalMS: 5_000},
		Agent:   config.AgentSettings{MaxConcurrentAgents: 4},
	}, nil)
	now := time.Unix(1_700_000_000, 0)

	rendered := stripANSI(term.format(Snapshot{
		Logs: []string{
			`time=2026-04-09T21:12:52-06:00 level=INFO msg="agent run completed" item=board-1:42 repo=api`,
			`time=2026-04-09T21:13:10-06:00 level=ERROR msg="agent run failed" item=board-1:43 repo=api error="boom"`,
		},
	}, now, 120))

	assertContains(t, rendered, "Recent Logs  2")
	assertContains(t, rendered, `msg="agent run completed"`)
	assertContains(t, rendered, `msg="agent run failed"`)
}

func TestTerminalLogWriterCapturesCompletedLines(t *testing.T) {
	term := NewTerminalForWriter(config.Settings{}, nil)
	term.Render(Snapshot{TrackerMode: "memory"})

	if _, err := term.LogWriter().Write([]byte("first line")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if len(term.lastSnapshot.Logs) != 0 {
		t.Fatalf("expected buffered partial line, got %v", term.lastSnapshot.Logs)
	}

	if _, err := term.LogWriter().Write([]byte(" continued\nsecond line\n")); err != nil {
		t.Fatalf("second write: %v", err)
	}

	if len(term.lastSnapshot.Logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(term.lastSnapshot.Logs))
	}
	if got := term.lastSnapshot.Logs[0]; got != "first line continued" {
		t.Fatalf("unexpected first log %q", got)
	}
	if got := term.lastSnapshot.Logs[1]; got != "second line" {
		t.Fatalf("unexpected second log %q", got)
	}
}

func TestDashboardModelHandlesResizeAndSnapshotUpdate(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	model := newDashboardModel(config.Settings{
		Polling: config.PollingSettings{IntervalMS: 15_000},
		Agent:   config.AgentSettings{MaxConcurrentAgents: 4},
	}, Snapshot{
		TrackerMode: "memory",
	}, func() time.Time { return now }, 100*time.Millisecond, nil)

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
	resized := updated.(dashboardModel)
	if resized.width != 132 || resized.height != 40 {
		t.Fatalf("unexpected size %dx%d", resized.width, resized.height)
	}

	updated, _ = resized.Update(snapshotMsg(Snapshot{
		Polling:     true,
		TrackerMode: "fizzy watched repos",
		Running: []RunningItem{{
			Identifier: "board-1:42",
			State:      "In Progress",
			StartedAt:  now.Add(-5 * time.Second),
		}},
	}))
	got := updated.(dashboardModel)
	if !got.snapshot.Polling {
		t.Fatalf("expected polling snapshot to be stored")
	}
	if got.snapshot.TrackerMode != "fizzy watched repos" {
		t.Fatalf("unexpected tracker mode %q", got.snapshot.TrackerMode)
	}
	if len(got.snapshot.Running) != 1 {
		t.Fatalf("expected 1 running item, got %d", len(got.snapshot.Running))
	}
}

func TestDashboardModelTickCmdKeepsRefreshing(t *testing.T) {
	model := newDashboardModel(config.Settings{}, Snapshot{}, time.Now, 50*time.Millisecond, nil)
	cmd := model.Init()
	if cmd == nil {
		t.Fatalf("expected init command")
	}

	updated, next := model.Update(tickMsg(time.Now()))
	if next == nil {
		t.Fatalf("expected tick to schedule another refresh")
	}
	if _, ok := updated.(dashboardModel); !ok {
		t.Fatalf("expected dashboardModel after tick update")
	}
}

func TestDashboardModelQuitKeyInvokesOnQuit(t *testing.T) {
	called := false
	model := newDashboardModel(config.Settings{}, Snapshot{}, time.Now, 50*time.Millisecond, func() {
		called = true
	})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !called {
		t.Fatalf("expected quit callback to run")
	}
	if cmd == nil {
		t.Fatalf("expected quit command")
	}
	if _, ok := updated.(dashboardModel); !ok {
		t.Fatalf("expected dashboardModel after quit update")
	}
}

func TestNewTerminalForWriterUsesStdinForInteractiveDashboard(t *testing.T) {
	term := NewTerminalForWriter(config.Settings{
		Observability: config.ObservabilitySettings{DashboardEnabled: true},
	}, os.Stdout)

	if !term.interactive {
		t.Fatalf("expected stdout dashboard to be interactive")
	}
	if term.in != os.Stdin {
		t.Fatalf("expected stdin to be wired for interactive dashboard")
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q\n\n%s", want, got)
	}
}

func assertNotContains(t *testing.T, got, want string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Fatalf("expected output to omit %q\n\n%s", want, got)
	}
}

func assertRightBordersAligned(t *testing.T, output string) {
	t.Helper()

	var expected int
	for _, line := range strings.Split(output, "\n") {
		if !strings.ContainsAny(line, "╭╮╰╯│") {
			continue
		}
		width := len([]rune(strings.TrimRight(line, " ")))
		if expected == 0 {
			expected = width
			continue
		}
		if width != expected {
			t.Fatalf("expected aligned right border width %d, got %d for line %q", expected, width, line)
		}
	}
}
