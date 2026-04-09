package observability

import "strings"

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func boxContentWidth(total int) int {
	return max(1, total-4)
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
