package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// overlay draws fg over bg with its top-left cell at (x, y). bg must be the
// final string (after zone.Scan): its lines are cut at the popup's edges.
// The backdrop loses its colors and is drawn in dim, so the popup stands out.
func overlay(bg, fg string, x, y int, dim lipgloss.Style) string {
	lines := strings.Split(bg, "\n")
	for i, l := range lines {
		lines[i] = dim.Render(ansi.Strip(l))
	}
	pop := strings.Split(fg, "\n")
	pw := lipgloss.Width(fg)
	left := x
	for i, p := range pop {
		row := y + i
		if row >= len(lines) {
			break
		}
		line := lines[row]
		if lw := ansi.StringWidth(line); lw < left {
			line += strings.Repeat(" ", left-lw)
		}
		lines[row] = ansi.Truncate(line, left, "") + "\x1b[0m" + p + "\x1b[0m" + ansi.TruncateLeft(line, left+pw, "")
	}
	return strings.Join(lines, "\n")
}
