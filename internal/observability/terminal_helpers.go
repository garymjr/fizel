package observability

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func styleWidthForTotal(style lipgloss.Style, total int) int {
	frameWithoutPadding := style.GetHorizontalFrameSize() - style.GetHorizontalPadding()
	return max(1, total-frameWithoutPadding)
}

func padText(v string, width int) string {
	v = truncateText(v, width)
	if width <= 0 {
		return ""
	}
	padding := width - lipglossWidth(v)
	if padding <= 0 {
		return v
	}
	return v + strings.Repeat(" ", padding)
}
