package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// overlay draws fg centered over bg, which is w x h cells. bg must be the
// final string (after zone.Scan): its lines are cut at the popup's edges.
// The backdrop loses its colors and is drawn in dim, so the popup stands out.
func overlay(bg, fg string, w, h int, dim lipgloss.Style) string {
	lines := strings.Split(bg, "\n")
	for i, l := range lines {
		lines[i] = dim.Render(ansi.Strip(l))
	}
	pop := strings.Split(fg, "\n")
	pw := lipgloss.Width(fg)
	top := max((h-len(pop))/2, 0)
	left := max((w-pw)/2, 0)
	for i, p := range pop {
		y := top + i
		if y >= len(lines) {
			break
		}
		line := lines[y]
		if lw := ansi.StringWidth(line); lw < left {
			line += strings.Repeat(" ", left-lw)
		}
		lines[y] = ansi.Truncate(line, left, "") + "\x1b[0m" + p + "\x1b[0m" + ansi.TruncateLeft(line, left+pw, "")
	}
	return strings.Join(lines, "\n")
}
