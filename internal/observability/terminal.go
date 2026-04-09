package observability

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	settings       config.Settings
	out            io.Writer
	now            func() time.Time
	renderWidth    int
	renderInterval time.Duration
	interactive    bool
	mu             sync.Mutex
	program        *tea.Program
	started        bool
	lastSnapshot   Snapshot
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func NewTerminal(settings config.Settings) *Terminal {
	return NewTerminalForWriter(settings, os.Stdout)
}

func NewTerminalForWriter(settings config.Settings, out io.Writer) *Terminal {
	if out == nil {
		out = os.Stdout
	}
	interval := time.Duration(settings.Observability.RenderIntervalMS) * time.Millisecond
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	return &Terminal{
		settings:       settings,
		out:            out,
		now:            time.Now,
		renderWidth:    110,
		renderInterval: interval,
		interactive:    out == os.Stdout && settings.Observability.DashboardEnabled,
	}
}

func (t *Terminal) Render(snapshot Snapshot) {
	t.mu.Lock()
	t.lastSnapshot = snapshot
	if t.interactive {
		t.ensureProgramLocked()
		program := t.program
		t.mu.Unlock()
		program.Send(snapshotMsg(snapshot))
		return
	}
	rendered := t.format(snapshot, t.now(), t.renderWidth)
	t.mu.Unlock()
	fmt.Fprint(t.out, "\033[H\033[2J")
	fmt.Fprintln(t.out, rendered)
}

func (t *Terminal) ensureProgramLocked() {
	if t.started {
		return
	}
	model := newDashboardModel(t.settings, t.lastSnapshot, t.now, t.renderInterval)
	t.program = tea.NewProgram(
		model,
		tea.WithOutput(t.out),
		tea.WithInput(nil),
		tea.WithAltScreen(),
	)
	t.started = true
	go func(program *tea.Program) {
		_ = program.Start()
	}(t.program)
}

func (t *Terminal) format(snapshot Snapshot, now time.Time, width int) string {
	model := newDashboardModel(t.settings, snapshot, func() time.Time { return now }, t.renderInterval)
	model.width = width
	return model.View()
}

func stripANSI(v string) string {
	return ansiPattern.ReplaceAllString(v, "")
}

func truncateText(v string, width int) string {
	v = strings.TrimSpace(v)
	if width <= 0 || lipglossWidth(v) <= width {
		return v
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(v)
	out := make([]rune, 0, len(runes))
	for _, r := range runes {
		candidate := string(append(out, r))
		if lipglossWidth(candidate+"…") > width {
			break
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return "…"
	}
	return string(out) + "…"
}

func repoLabel(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
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
	return fmt.Sprintf("%dm %02ds", minutes, seconds)
}
