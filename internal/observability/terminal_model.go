package observability

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gmurray/fizel/internal/config"
)

type snapshotMsg Snapshot

type tickMsg time.Time

type dashboardModel struct {
	settings       config.Settings
	snapshot       Snapshot
	now            func() time.Time
	onQuit         func()
	currentTime    time.Time
	renderInterval time.Duration
	width          int
	height         int
	styles         dashboardStyles
}

func newDashboardModel(settings config.Settings, snapshot Snapshot, now func() time.Time, renderInterval time.Duration, onQuit func()) dashboardModel {
	if now == nil {
		now = time.Now
	}
	model := dashboardModel{
		settings:       settings,
		snapshot:       snapshot,
		now:            now,
		onQuit:         onQuit,
		currentTime:    now(),
		renderInterval: renderInterval,
		width:          110,
		styles:         newDashboardStyles(),
	}
	return model
}

func (m dashboardModel) Init() tea.Cmd {
	return tickCmd(m.renderInterval)
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case snapshotMsg:
		m.snapshot = Snapshot(msg)
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		m.currentTime = time.Time(msg)
		return m, tickCmd(m.renderInterval)
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.onQuit != nil {
				m.onQuit()
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m dashboardModel) View() string {
	return m.render()
}

func tickCmd(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	return tea.Tick(interval, func(ts time.Time) tea.Msg {
		return tickMsg(ts)
	})
}
