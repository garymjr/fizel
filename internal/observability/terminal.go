package observability

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gmurray/fizel/internal/config"
)

type Snapshot struct {
	Polling       bool
	Running       []RunningItem
	Retrying      []RetryItem
	TrackerHeader string
	LastEvent     string
}

type RunningItem struct {
	Identifier string
	State      string
	StartedAt  time.Time
}

type RetryItem struct {
	Identifier string
	Attempt    int
	RetryAt    time.Time
}

type Terminal struct {
	settings config.Settings
	mu       sync.Mutex
}

func NewTerminal(settings config.Settings) *Terminal {
	return &Terminal{settings: settings}
}

func (t *Terminal) Render(snapshot Snapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var lines []string
	lines = append(lines, "╭─ FIZEL STATUS")
	lines = append(lines, fmt.Sprintf("│ Tracker: %s", snapshot.TrackerHeader))
	if snapshot.Polling {
		lines = append(lines, "│ Polling: checking")
	} else {
		lines = append(lines, "│ Polling: idle")
	}
	lines = append(lines, fmt.Sprintf("│ Running: %d", len(snapshot.Running)))
	for _, item := range snapshot.Running {
		lines = append(lines, fmt.Sprintf("│  %s [%s] age=%s", item.Identifier, item.State, time.Since(item.StartedAt).Round(time.Second)))
	}
	if len(snapshot.Retrying) > 0 {
		lines = append(lines, "│ Retries:")
		for _, item := range snapshot.Retrying {
			lines = append(lines, fmt.Sprintf("│  %s attempt=%d due=%s", item.Identifier, item.Attempt, item.RetryAt.Format(time.RFC3339)))
		}
	}
	if strings.TrimSpace(snapshot.LastEvent) != "" {
		lines = append(lines, fmt.Sprintf("│ Event: %s", snapshot.LastEvent))
	}
	lines = append(lines, "╰")
	fmt.Print("\033[H\033[2J")
	fmt.Println(strings.Join(lines, "\n"))
}
