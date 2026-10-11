package ui

import (
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// modal is a centered dialog that captures all keys while open.
type modal interface {
	// update handles a key. done=true closes the modal.
	update(m *Model, msg tea.KeyMsg) (next modal, cmd tea.Cmd, done bool)
	view(m *Model) string
}

func modalWidth(m *Model) int { return max(30, min(m.width-6, 72)) }

// --- prompt: single-line text input ---------------------------------------

type prompt struct {
	title  string
	hint   string
	value  string
	repeat bool // stay open after submit, for adding several items quickly
	submit func(m *Model, value string) tea.Cmd
	input  textinput.Model
}

func (m *Model) openPrompt(p prompt) tea.Cmd {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.PromptStyle = sAccent
	ti.SetValue(p.value)
	ti.CursorEnd()
	ti.Focus()
	p.input = ti
	m.modal = &p
	return textinput.Blink
}

func (p *prompt) update(m *Model, msg tea.KeyMsg) (modal, tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		return p, nil, true
	case "enter":
		val := p.input.Value()
		cmd := p.submit(m, val)
		if p.repeat && strings.TrimSpace(val) != "" {
			p.input.SetValue("")
			return p, cmd, false
		}
		return p, cmd, true
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return p, cmd, false
}

func (p *prompt) view(m *Model) string {
	w := modalWidth(m)
	p.input.Width = w - 6
	parts := []string{sBold.Render(p.title), "", p.input.View()}
	hint := "enter save · esc cancel"
	if p.repeat {
		hint = "enter add another · esc done"
	}
	if p.hint != "" {
		parts = append(parts, "", sDim.Render(truncate(p.hint, w-4)))
	}
	parts = append(parts, sDim.Render(hint))
	return sModal.Width(w).Render(strings.Join(parts, "\n"))
}

// --- confirm: yes / no ----------------------------------------------------

type confirm struct {
	text string
	yes  func(m *Model) tea.Cmd
}

func (c *confirm) update(m *Model, msg tea.KeyMsg) (modal, tea.Cmd, bool) {
	switch msg.String() {
	case "y", "Y", "enter":
		return c, c.yes(m), true
	case "n", "N", "esc", "q":
		return c, nil, true
	}
	return c, nil, false
}

func (c *confirm) view(m *Model) string {
	w := min(modalWidth(m), lipgloss.Width(c.text)+6)
	w = max(w, 30)
	return sModal.Width(w).Render(c.text + "\n\n" + sKey.Render("y") + sDim.Render(" yes   ") + sKey.Render("n") + sDim.Render(" no"))
}

// --- picker: choose from a filterable list --------------------------------

type pickOpt struct {
	id     string
	label  string
	prefix string
	on     bool
}

type picker struct {
	title  string
	opts   []pickOpt
	optsFn func(m *Model) []pickOpt // recomputed every frame when set (for live toggles)
	multi  bool
	cursor int
	query  string
	choose func(m *Model, o pickOpt) (cmd tea.Cmd, close bool)
}

func (p *picker) options(m *Model) []pickOpt {
	opts := p.opts
	if p.optsFn != nil {
		opts = p.optsFn(m)
	}
	if p.query == "" {
		return opts
	}
	var out []pickOpt
	q := strings.ToLower(p.query)
	for _, o := range opts {
		if fuzzy(strings.ToLower(o.label), q) {
			out = append(out, o)
		}
	}
	return out
}

func (p *picker) update(m *Model, msg tea.KeyMsg) (modal, tea.Cmd, bool) {
	opts := p.options(m)
	switch msg.String() {
	case "esc":
		if p.query != "" {
			p.query = ""
			return p, nil, false
		}
		return p, nil, true
	case "up", "ctrl+p", "ctrl+k", "shift+tab":
		p.cursor = clamp(p.cursor-1, 0, len(opts)-1)
	case "down", "ctrl+n", "ctrl+j", "tab":
		p.cursor = clamp(p.cursor+1, 0, len(opts)-1)
	case "enter":
		if len(opts) == 0 {
			return p, nil, false
		}
		cmd, closeIt := p.choose(m, opts[clamp(p.cursor, 0, len(opts)-1)])
		return p, cmd, closeIt
	case " ":
		if p.multi && len(opts) > 0 {
			cmd, closeIt := p.choose(m, opts[clamp(p.cursor, 0, len(opts)-1)])
			return p, cmd, closeIt
		}
		p.query += " "
	case "backspace":
		if p.query != "" {
			p.query = p.query[:len(p.query)-1]
		}
	default:
		if msg.Type == tea.KeyRunes {
			// j/k navigate while there is no query, otherwise they filter.
			if p.query == "" && (msg.String() == "j" || msg.String() == "k") {
				d := map[string]int{"j": 1, "k": -1}[msg.String()]
				p.cursor = clamp(p.cursor+d, 0, len(opts)-1)
				return p, nil, false
			}
			p.query += string(msg.Runes)
			p.cursor = 0
		}
	}
	return p, nil, false
}

func (p *picker) view(m *Model) string {
	w := min(modalWidth(m), 50)
	opts := p.options(m)
	p.cursor = clamp(p.cursor, 0, len(opts)-1)
	lines := []string{sBold.Render(p.title)}
	if p.query != "" {
		lines = append(lines, sAccent.Render("› ")+p.query)
	} else {
		lines = append(lines, sDim.Render("type to filter"))
	}
	lines = append(lines, "")
	maxRows := max(3, m.height-12)
	start := 0
	if p.cursor >= maxRows {
		start = p.cursor - maxRows + 1
	}
	for i := start; i < len(opts) && i < start+maxRows; i++ {
		o := opts[i]
		mark := ""
		if p.multi {
			mark = sDim.Render("[ ] ")
			if o.on {
				mark = sOK.Render("[✓] ")
			}
		}
		line := mark + o.prefix + o.label
		if i == p.cursor {
			line = sAccent.Render("▌") + sBold.Render(line)
		} else {
			line = " " + line
		}
		lines = append(lines, truncate(line, w-4))
	}
	if len(opts) == 0 {
		lines = append(lines, sDim.Render(" no matches"))
	}
	hint := "enter select · esc cancel"
	if p.multi {
		hint = "space/enter toggle · esc done"
	}
	lines = append(lines, "", sDim.Render(hint))
	return sModal.Width(w).Render(strings.Join(lines, "\n"))
}

// --- help -------------------------------------------------------------------

func (m *Model) viewHelp() string {
	type kv struct{ k, d string }
	sections := []struct {
		title string
		keys  []kv
	}{
		{"Board", []kv{
			{"h j k l / arrows", "navigate lists and cards"},
			{"g G  0 $", "top/bottom of list · first/last list"},
			{"enter", "open card"},
			{"n / a  ·  O", "new card at bottom · at top"},
			{"H J K L", "move card between / within lists"},
			{"M", "move card to list…"},
			{"e  ·  E", "rename · edit description ($EDITOR)"},
			{"t  ·  m  ·  space", "labels · members · join/leave"},
			{"D", "mark done / not done"},
			{"d", "set due date"},
			{"x", "archive card"},
			{"N  ·  R  ·  X", "new list · rename list · archive list"},
			{"/", "filter (text, #label, @member, due, overdue)"},
			{"z", "toggle compact view"},
			{"o  ·  y", "open in browser · copy link"},
			{"r  ·  b", "refresh · switch board"},
		}},
		{"Card", []kv{
			{"j k", "select checklist item"},
			{"space / x", "toggle item"},
			{"a  ·  N  ·  X", "add item · new checklist · delete item"},
			{"c  ·  C", "comment · comment in $EDITOR"},
			{"D  ·  s", "mark done · join/leave card"},
			{"H L  ·  A", "move to prev/next list · archive"},
			{"esc", "back to board"},
		}},
	}
	blocks := make([]string, len(sections))
	for i, s := range sections {
		lines := []string{sAccent.Bold(true).Render(s.title)}
		for _, k := range s.keys {
			lines = append(lines, sKey.Render(padRight(k.k, 18))+" "+k.d)
		}
		blocks[i] = strings.Join(lines, "\n")
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, blocks[0], "    ", blocks[1])
	if lipgloss.Width(body)+4 > m.width {
		body = blocks[0] + "\n\n" + blocks[1]
	}
	lines := strings.Split(body, "\n")
	footer := "q quit · any other key closes help"
	// Scroll when the help is taller than the screen.
	if avail := m.height - 6; len(lines) > avail {
		m.helpScroll = clamp(m.helpScroll, 0, len(lines)-avail)
		lines = lines[m.helpScroll : m.helpScroll+avail]
		footer = "j/k scroll · " + footer
	}
	lines = append(lines, "", sDim.Render(footer))
	return sModal.Render(strings.Join(lines, "\n"))
}

// --- clipboard --------------------------------------------------------------

func copyText(s string) tea.Cmd {
	return func() tea.Msg {
		for _, c := range [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}, {"pbcopy"}} {
			if _, err := exec.LookPath(c[0]); err != nil {
				continue
			}
			cmd := exec.Command(c[0], c[1:]...)
			cmd.Stdin = strings.NewReader(s)
			if err := cmd.Run(); err == nil {
				return statusMsg("Copied " + s)
			}
		}
		return statusMsg("No clipboard tool found (install wl-clipboard or xclip)")
	}
}
