package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// UI chrome uses the 16 ANSI colors so it follows the terminal theme.
var (
	cAccent = lipgloss.Color("4")
	cDim    = lipgloss.Color("8")
	cFg     = lipgloss.Color("7")
	cBright = lipgloss.Color("15")
	cRed    = lipgloss.Color("1")
	cGreen  = lipgloss.Color("2")
	cYellow = lipgloss.Color("3")
	cMag    = lipgloss.Color("5")

	sDim      = lipgloss.NewStyle().Foreground(cDim)
	sBold     = lipgloss.NewStyle().Bold(true)
	sAccent   = lipgloss.NewStyle().Foreground(cAccent)
	sErr      = lipgloss.NewStyle().Foreground(cRed)
	sOK       = lipgloss.NewStyle().Foreground(cGreen)
	sKey      = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sTitleBar = lipgloss.NewStyle().Bold(true).Foreground(cBright)

	sCard = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cDim).
		Padding(0, 1)
	sCardSel = sCard.BorderForeground(cAccent).Border(lipgloss.ThickBorder())

	sListHead    = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	sListHeadSel = sListHead.Background(cAccent).Foreground(lipgloss.Color("0"))

	sModal = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cAccent).
		Padding(0, 1)
)

// Trello label colors.
var labelColors = map[string]string{
	"green": "#4bce97", "yellow": "#e2b203", "orange": "#faa53d", "red": "#f87462",
	"purple": "#9f8fef", "blue": "#579dff", "sky": "#6cc3e0", "lime": "#94c748",
	"pink": "#e774bb", "black": "#8590a2",
	"green_dark": "#1f845a", "yellow_dark": "#946f00", "orange_dark": "#c25100", "red_dark": "#c9372c",
	"purple_dark": "#6e5dc6", "blue_dark": "#0c66e4", "sky_dark": "#227d9b", "lime_dark": "#5b7f24",
	"pink_dark": "#ae4787", "black_dark": "#626f86",
	"green_light": "#baf3db", "yellow_light": "#f8e6a0", "orange_light": "#fedec8", "red_light": "#ffd5d2",
	"purple_light": "#dfd8fd", "blue_light": "#cce0ff", "sky_light": "#c6edfb", "lime_light": "#d3f1a7",
	"pink_light": "#fdd0ec", "black_light": "#dcdfe4",
}

func labelStyle(color string) lipgloss.Style {
	hex, ok := labelColors[color]
	if !ok {
		return lipgloss.NewStyle().Foreground(cFg).Background(cDim)
	}
	fg := lipgloss.Color("#1d2125")
	if strings.HasSuffix(color, "_dark") {
		fg = lipgloss.Color("#ffffff")
	}
	return lipgloss.NewStyle().Background(lipgloss.Color(hex)).Foreground(fg)
}

func labelDot(color string) string {
	hex, ok := labelColors[color]
	if !ok {
		return sDim.Render("●")
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render("●")
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

// wrap word-wraps s to width w, hard-breaking long words.
func wrap(s string, w int) []string {
	if w <= 0 {
		return nil
	}
	return strings.Split(ansi.Wrap(s, w, ""), "\n")
}

// padRight pads s with spaces to visible width w (truncating if longer).
func padRight(s string, w int) string {
	sw := ansi.StringWidth(s)
	if sw > w {
		return truncate(s, w)
	}
	return s + strings.Repeat(" ", w-sw)
}

// spread renders left and right text separated by spaces across width w.
func spread(left, right string, w int) string {
	lw, rw := ansi.StringWidth(left), ansi.StringWidth(right)
	if lw+rw+1 > w {
		left = truncate(left, max(0, w-rw-1))
		lw = ansi.StringWidth(left)
	}
	gap := max(1, w-lw-rw)
	return left + strings.Repeat(" ", gap) + right
}

// overlay draws fg centered on top of bg.
func overlay(bg, fg string, width, height int) string {
	bgLines := strings.Split(bg, "\n")
	for len(bgLines) < height {
		bgLines = append(bgLines, "")
	}
	fgLines := strings.Split(fg, "\n")
	fw := 0
	for _, l := range fgLines {
		fw = max(fw, ansi.StringWidth(l))
	}
	x := max(0, (width-fw)/2)
	y := max(0, (height-len(fgLines))/2)
	for i, fl := range fgLines {
		row := y + i
		if row >= len(bgLines) {
			break
		}
		line := padRight(bgLines[row], width)
		left := ansi.Cut(line, 0, x)
		right := ansi.Cut(line, x+fw, width)
		bgLines[row] = left + padRight(fl, fw) + right
	}
	return strings.Join(bgLines[:height], "\n")
}
