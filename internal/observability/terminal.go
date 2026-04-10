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
	Polling       bool
	LastRefreshAt time.Time
	Running       []RunningItem
	Retrying      []RetryItem
	Logs          []string
	TrackerMode   string
	WatchedRepos  []WatchedRepoStatus
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
	in             io.Reader
	now            func() time.Time
	onQuit         func()
	renderWidth    int
	renderInterval time.Duration
	interactive    bool
	mu             sync.Mutex
	program        *tea.Program
	started        bool
	lastSnapshot   Snapshot
	logLines       []string
	logPartial     string
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

const maxLogLines = 200

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
		in:             os.Stdin,
		now:            time.Now,
		renderWidth:    110,
		renderInterval: interval,
		interactive:    out == os.Stdout && settings.Observability.DashboardEnabled,
	}
}

func (t *Terminal) Render(snapshot Snapshot) {
	t.mu.Lock()
	snapshot.Logs = append([]string(nil), t.logLines...)
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

func (t *Terminal) LogWriter() io.Writer {
	return terminalLogWriter{term: t}
}

func (t *Terminal) appendLogs(lines []string) {
	if len(lines) == 0 {
		return
	}
	cleaned := make([]string, 0, len(lines))
	t.mu.Lock()
	for _, line := range lines {
		line = strings.TrimSpace(stripANSI(strings.TrimSuffix(line, "\r")))
		if line == "" {
			continue
		}
		t.logLines = append(t.logLines, line)
		cleaned = append(cleaned, line)
	}
	if len(cleaned) == 0 {
		t.mu.Unlock()
		return
	}
	if len(t.logLines) > maxLogLines {
		t.logLines = append([]string(nil), t.logLines[len(t.logLines)-maxLogLines:]...)
	}
	t.lastSnapshot.Logs = append([]string(nil), t.logLines...)
	program := t.program
	started := t.started
	interactive := t.interactive
	t.mu.Unlock()

	if interactive && started && program != nil {
		program.Send(logMsg(cleaned))
	}
}

func (t *Terminal) ensureProgramLocked() {
	if t.started {
		return
	}
	model := newDashboardModel(t.settings, t.lastSnapshot, t.now, t.renderInterval, t.onQuit)
	t.program = tea.NewProgram(
		model,
		tea.WithOutput(t.out),
		tea.WithInput(t.in),
		tea.WithAltScreen(),
	)
	t.started = true
	go func(program *tea.Program) {
		_ = program.Start()
	}(t.program)
}

func (t *Terminal) SetOnQuit(fn func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onQuit = fn
}

func (t *Terminal) Close() {
	t.mu.Lock()
	program := t.program
	started := t.started
	interactive := t.interactive
	out := t.out
	t.mu.Unlock()

	if !started || program == nil {
		return
	}
	program.Quit()
	program.Wait()
	if interactive {
		fmt.Fprint(out, "\r")
	}
}

func (t *Terminal) format(snapshot Snapshot, now time.Time, width int) string {
	model := newDashboardModel(t.settings, snapshot, func() time.Time { return now }, t.renderInterval, nil)
	model.width = width
	return model.View()
}

func stripANSI(v string) string {
	return ansiPattern.ReplaceAllString(v, "")
}

type terminalLogWriter struct {
	term *Terminal
}

func (w terminalLogWriter) Write(p []byte) (int, error) {
	if w.term == nil {
		return len(p), nil
	}

	w.term.mu.Lock()
	combined := w.term.logPartial + string(p)
	parts := strings.Split(combined, "\n")
	w.term.logPartial = parts[len(parts)-1]
	lines := append([]string(nil), parts[:len(parts)-1]...)
	w.term.mu.Unlock()

	w.term.appendLogs(lines)
	return len(p), nil
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
