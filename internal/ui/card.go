package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/psalwasser/trello-tui/internal/trello"
)

type cardView struct {
	cardID  string
	detail  *trello.CardDetail
	loading bool
	cursor  int // index into focusables()
	scroll  int
}

// focus identifies a selectable row: a checklist header (item == -1) or a
// check item.
type focus struct{ cl, item int }

func (v *cardView) focusables() []focus {
	if v.detail == nil {
		return nil
	}
	var fs []focus
	for ci, cl := range v.detail.Checklists {
		fs = append(fs, focus{ci, -1})
		for ii := range cl.CheckItems {
			fs = append(fs, focus{ci, ii})
		}
	}
	return fs
}

func (v *cardView) current() (focus, bool) {
	fs := v.focusables()
	if len(fs) == 0 {
		return focus{}, false
	}
	v.cursor = clamp(v.cursor, 0, len(fs)-1)
	return fs[v.cursor], true
}

// refreshCard is called after the board reloads; leave the card view if the
// card disappeared.
func (v *cardView) refreshCard(m *Model) {
	if m.board.card(v.cardID) == nil && !strings.HasPrefix(v.cardID, "tmp-") {
		m.screen = screenBoard
		m.setStatus("Card is no longer on this board", true)
	}
}

func (m *Model) openCard(id string) tea.Cmd {
	m.card = cardView{cardID: id, loading: true}
	m.screen = screenCard
	if strings.HasPrefix(id, "tmp-") {
		m.card.detail = &trello.CardDetail{}
		m.card.loading = false
		return nil
	}
	be := m.be
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d, err := be.CardDetail(ctx, id)
		return detailMsg{cardID: id, d: d, err: err}
	}
}

func (m Model) updateCardView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	v := &m.card
	c := m.board.card(v.cardID)
	if c == nil {
		m.screen = screenBoard
		return m, nil
	}
	id := c.ID
	switch msg.String() {
	case "q", "esc", "backspace":
		m.screen = screenBoard
		m.board.selectCard(id)
	case "j", "down":
		v.cursor++
		v.current()
	case "k", "up":
		v.cursor = max(0, v.cursor-1)
	case "ctrl+d", "pgdown":
		v.scroll += m.height / 2
	case "ctrl+u", "pgup":
		v.scroll = max(0, v.scroll-m.height/2)
	case "g", "home":
		v.cursor, v.scroll = 0, 0
	case "G", "end":
		v.cursor = 1 << 30
		v.current()
		v.scroll = 1 << 30
	case "e":
		return m, m.promptRename(id)
	case "E":
		return m, m.editDesc(id)
	case "c":
		return m, m.promptComment(id)
	case "C":
		return m, m.editComment(id)
	case "t":
		return m, m.pickLabels(id)
	case "m":
		return m, m.pickMembers(id)
	case "s":
		return m, m.toggleJoin(id)
	case "d":
		return m, m.promptDue(id)
	case "D":
		return m, m.toggleDueComplete(id)
	case "M":
		return m, m.pickMoveList(id)
	case "H", "L":
		m.board.selectCard(id)
		delta := map[string]int{"H": -1, "L": 1}[msg.String()]
		cmd := m.moveHoriz(c, delta)
		if cmd != nil {
			m.setStatus("Moved to "+m.board.listName(m.board.card(id).IDList), false)
		}
		return m, cmd
	case "A":
		m.modal = &confirm{
			text: fmt.Sprintf("Archive %q?", truncate(c.Name, 50)),
			yes: func(m *Model) tea.Cmd {
				m.board.selectCard(id)
				m.screen = screenBoard
				return m.archiveCard(id)
			},
		}
	case "o":
		return m, openURL(c.URL)
	case "y":
		return m, copyText(c.URL)
	case " ", "x", "enter":
		if f, ok := v.current(); ok && f.item >= 0 {
			return m, m.toggleCheckItem(id, f)
		}
	case "a":
		return m, m.promptCheckItem(id)
	case "N":
		return m, m.promptChecklist(id)
	case "X":
		if f, ok := v.current(); ok && f.item >= 0 {
			cl := v.detail.Checklists[f.cl]
			it := cl.CheckItems[f.item]
			m.modal = &confirm{
				text: fmt.Sprintf("Delete item %q?", truncate(it.Name, 40)),
				yes: func(m *Model) tea.Cmd {
					return m.deleteCheckItem(id, cl.ID, it.ID)
				},
			}
		}
	}
	return m, nil
}

func (m *Model) adjustBadge(cardID string, f func(*trello.Badges)) {
	if c := m.board.card(cardID); c != nil {
		f(&c.Badges)
	}
}

func (m *Model) toggleCheckItem(cardID string, f focus) tea.Cmd {
	it := &m.card.detail.Checklists[f.cl].CheckItems[f.item]
	done := !it.Done()
	if done {
		it.State = "complete"
	} else {
		it.State = "incomplete"
	}
	m.adjustBadge(cardID, func(b *trello.Badges) {
		if done {
			b.CheckItemsChecked++
		} else {
			b.CheckItemsChecked--
		}
	})
	itemID := it.ID
	be, ids := m.be, m.ids
	return m.mutate("update checklist", func(ctx context.Context) error {
		return be.SetCheckItem(ctx, ids.get(cardID), ids.get(itemID), done)
	}, nil)
}

func (m *Model) deleteCheckItem(cardID, clID, itemID string) tea.Cmd {
	d := m.card.detail
	for ci := range d.Checklists {
		cl := &d.Checklists[ci]
		if cl.ID != clID {
			continue
		}
		for ii, it := range cl.CheckItems {
			if it.ID == itemID {
				cl.CheckItems = slices.Delete(cl.CheckItems, ii, ii+1)
				m.adjustBadge(cardID, func(b *trello.Badges) {
					b.CheckItems--
					if it.Done() {
						b.CheckItemsChecked--
					}
				})
				break
			}
		}
	}
	be, ids := m.be, m.ids
	return m.mutate("delete item", func(ctx context.Context) error {
		return be.DeleteCheckItem(ctx, ids.get(clID), ids.get(itemID))
	}, nil)
}

func (m *Model) promptCheckItem(cardID string) tea.Cmd {
	v := &m.card
	if v.detail == nil {
		return nil
	}
	var cmds []tea.Cmd
	if len(v.detail.Checklists) == 0 {
		cmds = append(cmds, m.createChecklist(cardID, "Checklist"))
	}
	ci := 0
	if f, ok := v.current(); ok {
		ci = f.cl
	}
	clName := v.detail.Checklists[ci].Name
	cmds = append(cmds, m.openPrompt(prompt{
		title:  "Add item · " + clName,
		repeat: true,
		submit: func(m *Model, name string) tea.Cmd {
			name = strings.TrimSpace(name)
			if name == "" || m.card.detail == nil || ci >= len(m.card.detail.Checklists) {
				return nil
			}
			cl := &m.card.detail.Checklists[ci]
			tmp := m.tmpID()
			cl.CheckItems = append(cl.CheckItems, trello.CheckItem{ID: tmp, Name: name, State: "incomplete"})
			m.adjustBadge(cardID, func(b *trello.Badges) { b.CheckItems++ })
			// Move the cursor to the new item.
			for i, f := range m.card.focusables() {
				if f.cl == ci && f.item == len(cl.CheckItems)-1 {
					m.card.cursor = i
				}
			}
			clID := cl.ID
			be, ids := m.be, m.ids
			var created *trello.CheckItem
			return m.mutate("add item", func(ctx context.Context) error {
				it, err := be.AddCheckItem(ctx, ids.get(clID), name)
				if err == nil {
					ids.set(tmp, it.ID)
					created = it
				}
				return err
			}, func(m *Model) {
				if m.card.detail == nil {
					return
				}
				for ci := range m.card.detail.Checklists {
					items := m.card.detail.Checklists[ci].CheckItems
					for ii := range items {
						if items[ii].ID == tmp {
							items[ii].ID = created.ID
						}
					}
				}
			})
		},
	}))
	return tea.Batch(cmds...)
}

func (m *Model) createChecklist(cardID, name string) tea.Cmd {
	tmp := m.tmpID()
	m.card.detail.Checklists = append(m.card.detail.Checklists, trello.Checklist{ID: tmp, Name: name, IDCard: cardID})
	fs := m.card.focusables()
	for i, f := range fs {
		if f.cl == len(m.card.detail.Checklists)-1 {
			m.card.cursor = i
			break
		}
	}
	be, ids := m.be, m.ids
	var created *trello.Checklist
	return m.mutate("create checklist", func(ctx context.Context) error {
		cl, err := be.CreateChecklist(ctx, ids.get(cardID), name)
		if err == nil {
			ids.set(tmp, cl.ID)
			created = cl
		}
		return err
	}, func(m *Model) {
		if m.card.detail == nil {
			return
		}
		for i := range m.card.detail.Checklists {
			if m.card.detail.Checklists[i].ID == tmp {
				m.card.detail.Checklists[i].ID = created.ID
			}
		}
	})
}

func (m *Model) promptChecklist(cardID string) tea.Cmd {
	if m.card.detail == nil {
		return nil
	}
	return m.openPrompt(prompt{
		title: "New checklist",
		value: "Checklist",
		submit: func(m *Model, name string) tea.Cmd {
			name = strings.TrimSpace(name)
			if name == "" {
				return nil
			}
			return m.createChecklist(cardID, name)
		},
	})
}

func (m *Model) promptComment(cardID string) tea.Cmd {
	return m.openPrompt(prompt{
		title: "Comment",
		hint:  "enter to post · C opens $EDITOR for longer comments",
		submit: func(m *Model, text string) tea.Cmd {
			return m.addComment(cardID, text)
		},
	})
}

func (m *Model) addComment(cardID, text string) tea.Cmd {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	cm := trello.Comment{ID: m.tmpID(), Date: time.Now()}
	if m.me != nil {
		cm.MemberCreator = *m.me
	}
	cm.Data.Text = text
	if m.card.detail != nil && m.card.cardID == cardID {
		m.card.detail.Comments = append([]trello.Comment{cm}, m.card.detail.Comments...)
	}
	m.adjustBadge(cardID, func(b *trello.Badges) { b.Comments++ })
	be, ids := m.be, m.ids
	return m.mutate("add comment", func(ctx context.Context) error {
		_, err := be.AddComment(ctx, ids.get(cardID), text)
		return err
	}, nil)
}

// --- external editor -----------------------------------------------------

func editorCmd() string {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if e := os.Getenv(env); e != "" {
			return e
		}
	}
	for _, e := range []string{"nvim", "vim", "nano", "vi"} {
		if _, err := exec.LookPath(e); err == nil {
			return e
		}
	}
	return "vi"
}

func (m *Model) runEditor(cardID, kind, initial string) tea.Cmd {
	f, err := os.CreateTemp("", "trello-tui-*.md")
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	path := f.Name()
	_, _ = f.WriteString(initial)
	f.Close()
	parts := strings.Fields(editorCmd())
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer os.Remove(path)
		if err != nil {
			return editDoneMsg{cardID: cardID, kind: kind, err: err}
		}
		data, err := os.ReadFile(path)
		return editDoneMsg{cardID: cardID, kind: kind, text: string(data), err: err}
	})
}

func (m *Model) editDesc(cardID string) tea.Cmd {
	c := m.board.card(cardID)
	return m.runEditor(cardID, "desc", c.Desc)
}

func (m *Model) editComment(cardID string) tea.Cmd {
	return m.runEditor(cardID, "comment", "")
}

func (m Model) handleEditDone(msg editDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setStatus("editor: "+msg.err.Error(), true)
		return m, nil
	}
	switch msg.kind {
	case "desc":
		c := m.board.card(msg.cardID)
		if c == nil {
			return m, nil
		}
		desc := strings.TrimRight(msg.text, "\n")
		if desc == c.Desc {
			m.setStatus("Description unchanged", false)
			return m, nil
		}
		m.setStatus("Description saved", false)
		return m, m.updateCard(msg.cardID, func(c *trello.Card) {
			c.Desc = desc
			c.Badges.Description = desc != ""
		}, trello.CardUpdate{Desc: &desc}, "save description")
	case "comment":
		return m, m.addComment(msg.cardID, msg.text)
	}
	return m, nil
}

// --- rendering -----------------------------------------------------------

func fmtAgo(t time.Time, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case t.Year() == now.Year():
		return t.Local().Format("Jan 2")
	default:
		return t.Local().Format("Jan 2 2006")
	}
}

func section(title string, w int) string {
	return sAccent.Bold(true).Render(title) + " " + sDim.Render(strings.Repeat("─", max(0, w-lipgloss.Width(title)-1)))
}

func renderMarkdownish(text string, w int) []string {
	var out []string
	inCode := false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			inCode = !inCode
			continue
		case inCode:
			out = append(out, sDim.Render("│ ")+truncate(line, w-2))
			continue
		case strings.HasPrefix(trimmed, "#"):
			out = append(out, sBold.Render(strings.TrimSpace(strings.TrimLeft(trimmed, "#"))))
			continue
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* "):
			indent := len(line) - len(strings.TrimLeft(line, " "))
			pad := strings.Repeat(" ", indent)
			for i, l := range wrap(trimmed[2:], w-indent-2) {
				if i == 0 {
					out = append(out, pad+"• "+l)
				} else {
					out = append(out, pad+"  "+l)
				}
			}
			continue
		}
		if line == "" {
			out = append(out, "")
			continue
		}
		out = append(out, wrap(line, w)...)
	}
	return out
}

func progressBar(done, total, w int) string {
	if total == 0 || w <= 0 {
		return ""
	}
	n := done * w / total
	st := sAccent
	if done == total {
		st = sOK
	}
	return st.Render(strings.Repeat("━", n)) + sDim.Render(strings.Repeat("━", w-n))
}

func (m *Model) viewCard(h int) string {
	v := &m.card
	c := m.board.card(v.cardID)
	if c == nil {
		return ""
	}
	now := time.Now()
	w := min(m.width-4, 96)
	var lines []string
	add := func(s ...string) { lines = append(lines, s...) }
	selLine := -1

	crumb := sDim.Render(m.board.bd.Name + " › " + m.board.listName(c.IDList))
	add(crumb, "")
	for _, l := range wrap(c.Name, w) {
		add(sTitleBar.Render(l))
	}
	add("")

	label := func(s string) string { return sDim.Render(padRight(s, 10)) }
	if len(c.IDLabels) > 0 {
		var chips []string
		for _, id := range c.IDLabels {
			if l, ok := m.board.labels[id]; ok {
				name := l.Name
				if name == "" {
					name = "  "
				}
				chips = append(chips, labelStyle(l.Color).Render(" "+name+" "))
			}
		}
		add(label("Labels") + truncate(strings.Join(chips, " "), w-10))
	}
	if len(c.IDMembers) > 0 {
		var names []string
		for _, id := range c.IDMembers {
			if mb, ok := m.board.members[id]; ok {
				names = append(names, lipgloss.NewStyle().Foreground(cMag).Render(mb.FullName))
			}
		}
		add(label("Members") + strings.Join(names, sDim.Render(", ")))
	}
	if c.Due != nil {
		s := dueStyle(*c.Due, c.DueComplete, now).Render(c.Due.Local().Format("Mon Jan 2 2006, 15:04"))
		switch {
		case c.DueComplete:
			s += sOK.Render("  ✓ complete")
		case c.Due.Before(now):
			s += sErr.Render("  overdue")
		}
		add(label("Due") + s)
	} else if c.DueComplete {
		add(label("Status") + sOK.Render("✓ Done"))
	}
	add("")

	add(section("Description", w))
	if strings.TrimSpace(c.Desc) == "" {
		add(sDim.Render("No description. Press E to edit."))
	} else {
		add(renderMarkdownish(c.Desc, w)...)
	}
	add("")

	switch {
	case v.loading:
		add(sDim.Render("Loading checklists and comments…"))
	case v.detail != nil:
		cur, hasCur := v.current()
		for ci, cl := range v.detail.Checklists {
			done := 0
			for _, it := range cl.CheckItems {
				if it.Done() {
					done++
				}
			}
			head := sAccent.Bold(true).Render("☑ "+cl.Name) + sDim.Render(fmt.Sprintf("  %d/%d  ", done, len(cl.CheckItems)))
			head += progressBar(done, len(cl.CheckItems), max(0, min(30, w-lipgloss.Width(head))))
			if hasCur && cur.cl == ci && cur.item == -1 {
				selLine = len(lines)
				head = sAccent.Render("▌") + head
			} else {
				head = " " + head
			}
			add(head)
			for ii, it := range cl.CheckItems {
				box := sDim.Render("[ ]")
				text := it.Name
				if it.Done() {
					box = sOK.Render("[✓]")
					text = sDim.Strikethrough(true).Render(text)
				}
				prefix := "   "
				if hasCur && cur.cl == ci && cur.item == ii {
					selLine = len(lines)
					prefix = sAccent.Render("▌") + "  "
					if !it.Done() {
						text = sBold.Render(text)
					}
				}
				wl := wrap(it.Name, w-7)
				if len(wl) <= 1 {
					add(prefix + box + " " + text)
				} else {
					// Re-wrap plain text and style each line.
					for i, l := range wl {
						if it.Done() {
							l = sDim.Strikethrough(true).Render(l)
						}
						if i == 0 {
							add(prefix + box + " " + l)
						} else {
							add("       " + l)
						}
					}
				}
			}
			add("")
		}
		add(section(fmt.Sprintf("Comments (%d)", len(v.detail.Comments)), w))
		if len(v.detail.Comments) == 0 {
			add(sDim.Render("No comments. Press c to add one."))
		}
		for _, cm := range v.detail.Comments {
			who := cm.MemberCreator.FullName
			if who == "" {
				who = "You"
			}
			add(lipgloss.NewStyle().Foreground(cMag).Bold(true).Render(who) + sDim.Render(" · "+fmtAgo(cm.Date, now)))
			for _, l := range renderMarkdownish(cm.Data.Text, w-2) {
				add("  " + l)
			}
			add("")
		}
	}

	// Scroll so the selected row is visible.
	maxScroll := max(0, len(lines)-h)
	if selLine >= 0 {
		if selLine < v.scroll {
			v.scroll = max(0, selLine-2)
		}
		if selLine >= v.scroll+h {
			v.scroll = selLine - h + 3
		}
	}
	v.scroll = clamp(v.scroll, 0, maxScroll)
	end := min(len(lines), v.scroll+h)
	visible := lines[v.scroll:end]
	margin := strings.Repeat(" ", max(2, (m.width-w)/2))
	for i := range visible {
		visible[i] = margin + visible[i]
	}
	return strings.Join(visible, "\n")
}
