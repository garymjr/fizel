package observability

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

type dashboardStyles struct {
	doc         lipgloss.Style
	header      lipgloss.Style
	title       lipgloss.Style
	subtitle    lipgloss.Style
	panel       lipgloss.Style
	panelTitle  lipgloss.Style
	label       lipgloss.Style
	muted       lipgloss.Style
	value       lipgloss.Style
	accent      lipgloss.Style
	success     lipgloss.Style
	warn        lipgloss.Style
	idle        lipgloss.Style
	tableHeader lipgloss.Style
	footer      lipgloss.Style
}

func newDashboardStyles() dashboardStyles {
	frame := lipgloss.AdaptiveColor{Light: "#94A3B8", Dark: "#5F6C7B"}
	text := lipgloss.AdaptiveColor{Light: "#0F172A", Dark: "#E6EDF3"}
	muted := lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#8B9BB4"}
	accent := lipgloss.AdaptiveColor{Light: "#0369A1", Dark: "#6CCFF6"}
	success := lipgloss.AdaptiveColor{Light: "#047857", Dark: "#7FD1AE"}
	warn := lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#F6C177"}
	idle := lipgloss.AdaptiveColor{Light: "#475569", Dark: "#A0AEC0"}

	return dashboardStyles{
		doc:         lipgloss.NewStyle().Foreground(text).Padding(1, 2),
		header:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(frame).Padding(0, 1),
		title:       lipgloss.NewStyle().Bold(true).Foreground(accent),
		subtitle:    lipgloss.NewStyle().Foreground(muted),
		panel:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(frame).Padding(0, 1),
		panelTitle:  lipgloss.NewStyle().Bold(true).Foreground(text),
		label:       lipgloss.NewStyle().Foreground(muted),
		muted:       lipgloss.NewStyle().Foreground(muted),
		value:       lipgloss.NewStyle().Foreground(text),
		accent:      lipgloss.NewStyle().Bold(true).Foreground(accent),
		success:     lipgloss.NewStyle().Bold(true).Foreground(success),
		warn:        lipgloss.NewStyle().Bold(true).Foreground(warn),
		idle:        lipgloss.NewStyle().Foreground(idle),
		tableHeader: lipgloss.NewStyle().Foreground(muted).Underline(true),
		footer:      lipgloss.NewStyle().Foreground(muted).Padding(0, 1),
	}
}

func (m dashboardModel) render() string {
	width := m.width
	if width <= 0 {
		width = 110
	}
	if width < 60 {
		width = 60
	}

	header := m.renderHeader(width)
	body := m.renderBody(width)
	footer := m.renderFooter(width)

	content := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	return m.styles.doc.Render(content)
}

func (m dashboardModel) renderHeader(width int) string {
	chips := []string{
		m.renderChip("tracker", defaultText(m.snapshot.TrackerMode, "idle"), m.styles.accent),
		m.renderChip("agents", fmt.Sprintf("%d/%d", len(m.snapshot.Running), m.settings.Agent.MaxConcurrentAgents), m.styles.success),
		m.renderChip("repos", m.repoCountLabel(), m.styles.value),
		m.renderChip("refresh", m.nextRefreshLabel(), m.refreshStyle()),
	}
	top := lipgloss.JoinHorizontal(lipgloss.Top, m.styles.title.Render("FIZEL"), "  ", m.styles.subtitle.Render("read-only orchestration dashboard"))
	bottom := lipgloss.JoinHorizontal(lipgloss.Top, chips...)
	content := lipgloss.JoinVertical(lipgloss.Left, top, bottom)
	return m.styles.header.Width(boxContentWidth(width - m.styles.doc.GetHorizontalPadding())).Render(content)
}

func (m dashboardModel) renderChip(label, value string, style lipgloss.Style) string {
	return lipgloss.NewStyle().MarginRight(1).Render(
		m.styles.label.Render(strings.ToUpper(label)+": ") + style.Render(value),
	)
}

func (m dashboardModel) renderBody(width int) string {
	innerWidth := width - m.styles.doc.GetHorizontalPadding()
	if innerWidth >= 110 {
		leftWidth := innerWidth*2/3 - 1
		rightWidth := innerWidth - leftWidth - 1
		left := m.renderRunningPanel(leftWidth)
		right := m.renderRetryPanel(rightWidth)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderRunningPanel(innerWidth),
		m.renderRetryPanel(innerWidth),
	)
}

func (m dashboardModel) renderRunningPanel(width int) string {
	contentWidth := boxContentWidth(width)
	rows := make([]string, 0, len(m.snapshot.Running)+3)
	rows = append(rows, m.styles.panelTitle.Render(fmt.Sprintf("Running Agents  %d", len(m.snapshot.Running))))
	rows = append(rows, "")
	if len(m.snapshot.Running) == 0 {
		rows = append(rows, m.styles.muted.Render("No active agents."))
		return m.styles.panel.Width(contentWidth).Render(strings.Join(rows, "\n"))
	}

	now := m.now()
	if contentWidth >= 74 {
		rows = append(rows, m.tableRow(contentWidth, []tableCell{
			{text: "REPO", width: 10, style: m.styles.tableHeader},
			{text: "ID", width: 20, style: m.styles.tableHeader},
			{text: "STATE", width: 14, style: m.styles.tableHeader},
			{text: "AGE", width: 8, style: m.styles.tableHeader},
			{text: "EVENT", width: contentWidth - 56, style: m.styles.tableHeader},
		}))
		for _, item := range m.snapshot.Running {
			rows = append(rows, m.tableRow(contentWidth, []tableCell{
				{text: repoLabel(item.RepoKey), width: 10, style: m.styles.success},
				{text: item.Identifier, width: 20, style: m.styles.accent},
				{text: item.State, width: 14, style: m.stateStyle(item.State)},
				{text: formatAge(now.Sub(item.StartedAt)), width: 8, style: m.styles.warn},
				{text: defaultText(item.LastEvent, "-"), width: contentWidth - 56, style: m.styles.muted},
			}))
		}
		return m.styles.panel.Width(contentWidth).Render(strings.Join(rows, "\n"))
	}

	rows = append(rows, m.tableRow(contentWidth, []tableCell{
		{text: "ID", width: max(14, contentWidth-20), style: m.styles.tableHeader},
		{text: "STATE", width: 10, style: m.styles.tableHeader},
		{text: "AGE", width: 6, style: m.styles.tableHeader},
	}))
	for _, item := range m.snapshot.Running {
		rows = append(rows, m.tableRow(contentWidth, []tableCell{
			{text: item.Identifier, width: max(14, contentWidth-20), style: m.styles.accent},
			{text: item.State, width: 10, style: m.stateStyle(item.State)},
			{text: formatAge(now.Sub(item.StartedAt)), width: 6, style: m.styles.warn},
		}))
		if contentWidth >= 48 {
			rows = append(rows, m.styles.muted.Render("  "+truncateText(defaultText(item.LastEvent, "-"), contentWidth-2)))
		}
	}
	return m.styles.panel.Width(contentWidth).Render(strings.Join(rows, "\n"))
}

func (m dashboardModel) renderRetryPanel(width int) string {
	contentWidth := boxContentWidth(width)
	rows := make([]string, 0, len(m.snapshot.Retrying)+3)
	rows = append(rows, m.styles.panelTitle.Render(fmt.Sprintf("Backoff Queue  %d", len(m.snapshot.Retrying))))
	rows = append(rows, "")
	if len(m.snapshot.Retrying) == 0 {
		rows = append(rows, m.styles.idle.Render("No queued retries."))
		return m.styles.panel.Width(contentWidth).Render(strings.Join(rows, "\n"))
	}

	now := m.now()
	if contentWidth >= 48 {
		rows = append(rows, m.tableRow(contentWidth, []tableCell{
			{text: "REPO", width: 10, style: m.styles.tableHeader},
			{text: "ID", width: 18, style: m.styles.tableHeader},
			{text: "TRY", width: 5, style: m.styles.tableHeader},
			{text: "RETRY IN", width: contentWidth - 36, style: m.styles.tableHeader},
		}))
		for _, item := range m.snapshot.Retrying {
			rows = append(rows, m.tableRow(contentWidth, []tableCell{
				{text: repoLabel(item.RepoKey), width: 10, style: m.styles.success},
				{text: item.Identifier, width: 18, style: m.styles.accent},
				{text: fmt.Sprintf("%d", item.Attempt), width: 5, style: m.styles.warn},
				{text: formatAge(item.RetryAt.Sub(now)), width: contentWidth - 36, style: m.styles.value},
			}))
		}
		return m.styles.panel.Width(contentWidth).Render(strings.Join(rows, "\n"))
	}

	for _, item := range m.snapshot.Retrying {
		line := fmt.Sprintf("%s  try %d  in %s", truncateText(item.Identifier, max(10, width-16)), item.Attempt, formatAge(item.RetryAt.Sub(now)))
		rows = append(rows, m.styles.value.Render(line))
	}
	return m.styles.panel.Width(contentWidth).Render(strings.Join(rows, "\n"))
}

func (m dashboardModel) renderFooter(width int) string {
	parts := []string{
		"auto-refreshing",
		fmt.Sprintf("%d running", len(m.snapshot.Running)),
		fmt.Sprintf("%d queued", len(m.snapshot.Retrying)),
		m.repoSummaryCompact(),
	}
	content := strings.Join(parts, "  •  ")
	if width >= 84 {
		content += "  •  q quit"
	}
	return m.styles.footer.Width(width - m.styles.doc.GetHorizontalPadding()).Render(content)
}

func (m dashboardModel) nextRefreshLabel() string {
	if m.snapshot.Polling {
		return "checking now"
	}
	seconds := max(1, int(time.Duration(m.settings.Polling.IntervalMS)*time.Millisecond/time.Second))
	return fmt.Sprintf("%ds", seconds)
}

func (m dashboardModel) refreshStyle() lipgloss.Style {
	if m.snapshot.Polling {
		return m.styles.accent
	}
	return m.styles.idle
}

func (m dashboardModel) repoCountLabel() string {
	if len(m.snapshot.WatchedRepos) == 0 {
		if m.settings.Repo.Key != "" {
			return "1 watched"
		}
		return "single workflow"
	}
	return fmt.Sprintf("%d watched", len(m.snapshot.WatchedRepos))
}

func (m dashboardModel) repoSummaryCompact() string {
	if len(m.snapshot.WatchedRepos) == 0 {
		if m.settings.Repo.Key != "" {
			return repoLabel(m.settings.Repo.Key)
		}
		return "single workflow"
	}
	keys := make([]string, 0, len(m.snapshot.WatchedRepos))
	for _, repo := range m.snapshot.WatchedRepos {
		keys = append(keys, repo.Key)
	}
	return strings.Join(keys, ", ")
}

func (m dashboardModel) stateStyle(state string) lipgloss.Style {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "todo":
		return m.styles.idle
	case "in progress":
		return m.styles.success
	case "human review":
		return m.styles.warn
	default:
		return m.styles.value
	}
}

type tableCell struct {
	text  string
	width int
	style lipgloss.Style
}

func (m dashboardModel) tableRow(width int, cells []tableCell) string {
	rendered := make([]string, 0, len(cells))
	for _, cell := range cells {
		if cell.width <= 0 {
			continue
		}
		rendered = append(rendered, cell.style.Render(padText(cell.text, cell.width)))
	}
	return truncateText(strings.Join(rendered, " "), width-2)
}

func defaultText(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}

func lipglossWidth(v string) int {
	return lipgloss.Width(v)
}
