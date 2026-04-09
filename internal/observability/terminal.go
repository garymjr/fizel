package observability

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gmurray/fizel/internal/config"
)

type Snapshot struct {
	Polling      bool
	Running      []RunningItem
	Retrying     []RetryItem
	TrackerMode  string
	WatchedRepos []WatchedRepoStatus
}

type WatchedRepoStatus struct {
	Key     string
	BoardID string
}

type RunningItem struct {
	Identifier string
	RepoKey    string
	State      string
	StartedAt  time.Time
	LastEvent  string
}

type RetryItem struct {
	Identifier string
	RepoKey    string
	Attempt    int
	RetryAt    time.Time
}

type Terminal struct {
	settings config.Settings
	out      io.Writer
	mu       sync.Mutex
}

const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiCyan    = "\033[36m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiMagenta = "\033[35m"
	ansiGray    = "\033[90m"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func NewTerminal(settings config.Settings) *Terminal {
	return &Terminal{settings: settings, out: os.Stdout}
}

func NewTerminalForWriter(settings config.Settings, out io.Writer) *Terminal {
	if out == nil {
		out = os.Stdout
	}
	return &Terminal{settings: settings, out: out}
}

func (t *Terminal) Render(snapshot Snapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Fprint(t.out, "\033[H\033[2J")
	fmt.Fprintln(t.out, strings.Join(t.format(snapshot, time.Now()), "\n"))
}

func (t *Terminal) format(snapshot Snapshot, now time.Time) []string {
	lines := []string{
		colorize("╭─ FIZEL STATUS", ansiBold),
		colorize("│ Agents: ", ansiBold) +
			colorize(fmt.Sprintf("%d", len(snapshot.Running)), ansiGreen) +
			colorize("/", ansiGray) +
			colorize(fmt.Sprintf("%d", t.settings.Agent.MaxConcurrentAgents), ansiGray),
		colorize("│ Tracker: ", ansiBold) + colorize(snapshot.TrackerMode, ansiCyan),
		colorize("│ Repos: ", ansiBold) + colorize(t.repoSummary(snapshot.WatchedRepos), ansiCyan),
		colorize("│ Next refresh: ", ansiBold) + colorize(t.nextRefreshLabel(snapshot.Polling), refreshColor(snapshot.Polling)),
		colorize("├─ Running", ansiBold),
		"│",
	}
	lines = append(lines, t.runningLines(snapshot.Running, now)...)
	lines = append(lines, "│", "├─ Backoff queue", "│")
	lines = append(lines, t.retryLines(snapshot.Retrying, now)...)
	lines = append(lines, "╰")
	return lines
}

func (t *Terminal) nextRefreshLabel(polling bool) string {
	if polling {
		return "checking now..."
	}
	seconds := max(1, int(time.Duration(t.settings.Polling.IntervalMS)*time.Millisecond/time.Second))
	return fmt.Sprintf("%ds", seconds)
}

func (t *Terminal) runningLines(running []RunningItem, now time.Time) []string {
	lines := []string{
		colorize(fmt.Sprintf("│ %-10s %-24s %-16s %-8s %s", "REPO", "ID", "STATE", "AGE", "EVENT"), ansiGray),
		colorize(fmt.Sprintf("│ %-10s %-24s %-16s %-8s %s", strings.Repeat("─", 10), strings.Repeat("─", 24), strings.Repeat("─", 16), strings.Repeat("─", 8), strings.Repeat("─", 24)), ansiGray),
	}
	if len(running) == 0 {
		return append(lines, colorize("│ No active agents", ansiGray))
	}
	for _, item := range running {
		event := strings.TrimSpace(item.LastEvent)
		if event == "" {
			event = "-"
		}
		lines = append(lines, fmt.Sprintf(
			"%s %s %s %s %s %s",
			colorize("│", ansiGray),
			padColored(colorize(truncate(repoLabel(item.RepoKey), 10), ansiGreen), 10),
			padColored(colorize(truncate(item.Identifier, 24), ansiCyan), 24),
			padColored(colorize(truncate(item.State, 16), ansiMagenta), 16),
			padColored(colorize(formatAge(now.Sub(item.StartedAt)), ansiYellow), 8),
			colorize(truncate(event, 24), ansiGray),
		))
	}
	return lines
}

func (t *Terminal) retryLines(retrying []RetryItem, now time.Time) []string {
	lines := []string{
		colorize(fmt.Sprintf("│ %-10s %-24s %-8s %s", "REPO", "ID", "ATTEMPT", "RETRY IN"), ansiGray),
		colorize(fmt.Sprintf("│ %-10s %-24s %-8s %s", strings.Repeat("─", 10), strings.Repeat("─", 24), strings.Repeat("─", 8), strings.Repeat("─", 16)), ansiGray),
	}
	if len(retrying) == 0 {
		return append(lines, colorize("│ No queued retries", ansiGray))
	}
	for _, item := range retrying {
		lines = append(lines, fmt.Sprintf(
			"%s %s %s %s %s",
			colorize("│", ansiGray),
			padColored(colorize(truncate(repoLabel(item.RepoKey), 10), ansiGreen), 10),
			padColored(colorize(truncate(item.Identifier, 24), ansiCyan), 24),
			padColored(colorize(fmt.Sprintf("%d", item.Attempt), ansiYellow), 8),
			colorize(formatAge(item.RetryAt.Sub(now)), ansiMagenta),
		))
	}
	return lines
}

func (t *Terminal) repoSummary(repos []WatchedRepoStatus) string {
	if len(repos) == 0 {
		if t.settings.Repo.Key != "" {
			return fmt.Sprintf("1 watched (%s)", t.settings.Repo.Key)
		}
		return "single workflow"
	}
	parts := make([]string, 0, len(repos))
	for _, repo := range repos {
		label := repo.Key
		if strings.TrimSpace(repo.BoardID) != "" {
			label += " -> " + repo.BoardID
		}
		parts = append(parts, label)
	}
	return fmt.Sprintf("%d watched (%s)", len(repos), strings.Join(parts, ", "))
}

func colorize(v, color string) string {
	if v == "" || color == "" {
		return v
	}
	return color + v + ansiReset
}

func refreshColor(polling bool) string {
	if polling {
		return ansiCyan
	}
	return ansiGray
}

func stripANSI(v string) string {
	return ansiPattern.ReplaceAllString(v, "")
}

func padColored(v string, width int) string {
	plain := stripANSI(v)
	if len(plain) >= width {
		return v
	}
	return v + strings.Repeat(" ", width-len(plain))
}

func formatAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	minutes := int(d / time.Minute)
	seconds := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%dm %ds", minutes, seconds)
}

func truncate(v string, width int) string {
	v = strings.TrimSpace(v)
	if len(v) <= width {
		return v
	}
	if width <= 1 {
		return v[:width]
	}
	return v[:width-1] + "…"
}

func repoLabel(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}
