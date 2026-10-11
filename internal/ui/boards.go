package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/psalwasser/trello-tui/internal/trello"
)

type boardsView struct {
	all      []trello.Board
	shown    []trello.Board
	cursor   int
	offset   int
	filter   textinput.Model
	loading  bool
	selectID string // keep cursor on this board when data refreshes
}

func newBoardsView() boardsView {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "filter boards"
	ti.PromptStyle = sAccent
	return boardsView{filter: ti, loading: true}
}

func (v *boardsView) setBoards(bs []trello.Board, lastID string) {
	cur := v.selectID
	if len(v.shown) > 0 && v.cursor < len(v.shown) {
		cur = v.shown[v.cursor].ID
	} else if cur == "" {
		cur = lastID
	}
	sort.SliceStable(bs, func(i, j int) bool {
		if bs[i].Starred != bs[j].Starred {
			return bs[i].Starred
		}
		if bs[i].WorkspaceName != bs[j].WorkspaceName {
			return bs[i].WorkspaceName < bs[j].WorkspaceName
		}
		return strings.ToLower(bs[i].Name) < strings.ToLower(bs[j].Name)
	})
	v.all = bs
	v.applyFilter()
	for i, b := range v.shown {
		if b.ID == cur {
			v.cursor = i
		}
	}
}

func (v *boardsView) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(v.filter.Value()))
	v.shown = v.shown[:0]
	for _, b := range v.all {
		if q == "" || fuzzy(strings.ToLower(b.Name+" "+b.WorkspaceName), q) {
			v.shown = append(v.shown, b)
		}
	}
	v.cursor = clamp(v.cursor, 0, len(v.shown)-1)
}

// fuzzy reports whether all characters of q appear in s in order, with
// space-separated terms matched independently.
func fuzzy(s, q string) bool {
	for _, term := range strings.Fields(q) {
		if strings.Contains(s, term) {
			continue
		}
		i := 0
		for _, r := range s {
			if i < len(term) && byte(r) == term[i] {
				i++
			}
		}
		if i < len(term) {
			return false
		}
	}
	return true
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return max(lo, min(v, hi))
}

func (m Model) updateBoards(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	v := &m.boards
	if v.filter.Focused() {
		switch msg.String() {
		case "esc":
			v.filter.Blur()
			v.filter.SetValue("")
			v.applyFilter()
			return m, nil
		case "enter":
			v.filter.Blur()
			if len(v.shown) == 1 {
				return m, m.openBoard(v.shown[0].ID, v.shown[0].Name)
			}
			return m, nil
		case "up", "ctrl+p", "ctrl+k":
			v.cursor = clamp(v.cursor-1, 0, len(v.shown)-1)
			return m, nil
		case "down", "ctrl+n", "ctrl+j":
			v.cursor = clamp(v.cursor+1, 0, len(v.shown)-1)
			return m, nil
		}
		var cmd tea.Cmd
		v.filter, cmd = v.filter.Update(msg)
		v.applyFilter()
		return m, cmd
	}
	switch msg.String() {
	case "q":
		return m, m.quit()
	case "esc":
		if v.filter.Value() != "" {
			v.filter.SetValue("")
			v.applyFilter()
		} else if m.board.bd != nil {
			m.screen = screenBoard
		}
	case "j", "down":
		v.cursor = clamp(v.cursor+1, 0, len(v.shown)-1)
	case "k", "up":
		v.cursor = clamp(v.cursor-1, 0, len(v.shown)-1)
	case "g", "home":
		v.cursor = 0
	case "G", "end":
		v.cursor = max(0, len(v.shown)-1)
	case "ctrl+d", "pgdown":
		v.cursor = clamp(v.cursor+10, 0, len(v.shown)-1)
	case "ctrl+u", "pgup":
		v.cursor = clamp(v.cursor-10, 0, len(v.shown)-1)
	case "/":
		v.filter.Focus()
		return m, textinput.Blink
	case "r":
		v.loading = true
		return m, m.loadBoards(false)
	case "o":
		if len(v.shown) > 0 {
			return m, openURL(v.shown[v.cursor].URL)
		}
	case "enter", "l", "right":
		if len(v.shown) > 0 {
			b := v.shown[v.cursor]
			return m, m.openBoard(b.ID, b.Name)
		}
	default:
		// Typing any letter starts filtering, like a fuzzy finder.
		if s := msg.String(); len(s) == 1 && s[0] >= 'a' && s[0] <= 'z' || len(s) == 1 && s[0] >= 'A' && s[0] <= 'Z' {
			v.filter.Focus()
			var cmd tea.Cmd
			v.filter, cmd = v.filter.Update(msg)
			v.applyFilter()
			return m, cmd
		}
	}
	return m, nil
}

func (m *Model) viewBoards(h int) string {
	v := &m.boards
	w := min(m.width, 90)
	var b strings.Builder
	b.WriteString(sTitleBar.Render(" Trello") + sDim.Render("  boards") + "\n")
	if v.filter.Focused() || v.filter.Value() != "" {
		v.filter.Width = w - 4
		b.WriteString(" " + v.filter.View() + "\n")
	} else {
		b.WriteString("\n")
	}
	listH := h - 3
	if len(v.shown) == 0 {
		if v.loading {
			b.WriteString(sDim.Render("   Loading boards…"))
		} else {
			b.WriteString(sDim.Render("   No boards"))
		}
		return b.String()
	}
	// Keep cursor visible.
	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+listH {
		v.offset = v.cursor - listH + 1
	}
	v.offset = clamp(v.offset, 0, max(0, len(v.shown)-listH))
	for i := v.offset; i < len(v.shown) && i < v.offset+listH; i++ {
		bo := v.shown[i]
		star := "  "
		if bo.Starred {
			star = lipgloss.NewStyle().Foreground(cYellow).Render("★ ")
		}
		ws := bo.WorkspaceName
		if ws == "" {
			ws = "Personal"
		}
		if bo.Starred {
			ws = "Starred · " + ws
		}
		line := spread(star+bo.Name, sDim.Render(ws), w-4)
		if i == v.cursor {
			line = lipgloss.NewStyle().Foreground(cAccent).Bold(true).Render("▌ ") + lipgloss.NewStyle().Bold(true).Render(line)
		} else {
			line = "  " + line
		}
		b.WriteString(" " + line + "\n")
	}
	return b.String()
}
