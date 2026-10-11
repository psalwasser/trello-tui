package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/philsalwasser/trello-tui/internal/trello"
)

type boardView struct {
	bd       *trello.BoardData
	wantID   string
	wantName string
	loading  bool

	cols   [][]*trello.Card // visible cards per list, after filtering
	col    int
	rows   map[string]int // listID -> selected row
	scroll map[string]int // listID -> first visible card
	colOff int

	filter    textinput.Model
	filtering bool
	compact   bool

	labels  map[string]trello.Label
	members map[string]trello.Member
}

func newBoardView() boardView {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "filter: text, #label, @member, due, overdue"
	ti.PromptStyle = sAccent
	return boardView{rows: map[string]int{}, scroll: map[string]int{}, filter: ti}
}

func (v *boardView) setData(bd *trello.BoardData) {
	var selID, selList string
	if c := v.sel(); c != nil {
		selID = c.ID
	}
	if v.bd != nil && v.col < len(v.bd.Lists) {
		selList = v.bd.Lists[v.col].ID
	}
	v.bd = bd
	v.wantID = bd.ID
	v.rebuild()
	if !v.selectCard(selID) && selList != "" {
		for i, l := range bd.Lists {
			if l.ID == selList {
				v.col = i
			}
		}
	}
	v.col = clamp(v.col, 0, len(bd.Lists)-1)
}

// rebuild recomputes the per-list card columns from bd.Cards.
func (v *boardView) rebuild() {
	bd := v.bd
	if bd == nil {
		return
	}
	v.labels = make(map[string]trello.Label, len(bd.Labels))
	for _, l := range bd.Labels {
		v.labels[l.ID] = l
	}
	v.members = make(map[string]trello.Member, len(bd.Members))
	for _, mb := range bd.Members {
		v.members[mb.ID] = mb
	}
	slices.SortStableFunc(bd.Cards, func(a, b trello.Card) int {
		switch {
		case a.Pos < b.Pos:
			return -1
		case a.Pos > b.Pos:
			return 1
		}
		return 0
	})
	idx := make(map[string]int, len(bd.Lists))
	for i, l := range bd.Lists {
		idx[l.ID] = i
	}
	v.cols = make([][]*trello.Card, len(bd.Lists))
	q := strings.ToLower(strings.TrimSpace(v.filter.Value()))
	now := time.Now()
	for i := range bd.Cards {
		c := &bd.Cards[i]
		li, ok := idx[c.IDList]
		if !ok || c.Closed {
			continue
		}
		if q != "" && !v.matches(c, q, now) {
			continue
		}
		v.cols[li] = append(v.cols[li], c)
	}
}

func (v *boardView) matches(c *trello.Card, q string, now time.Time) bool {
	for _, term := range strings.Fields(q) {
		ok := false
		switch {
		case strings.HasPrefix(term, "#") && len(term) > 1:
			for _, id := range c.IDLabels {
				l := v.labels[id]
				if strings.Contains(strings.ToLower(l.Name), term[1:]) || strings.HasPrefix(l.Color, term[1:]) {
					ok = true
				}
			}
		case strings.HasPrefix(term, "@") && len(term) > 1:
			for _, id := range c.IDMembers {
				mb := v.members[id]
				if strings.Contains(strings.ToLower(mb.Username), term[1:]) || strings.Contains(strings.ToLower(mb.FullName), term[1:]) {
					ok = true
				}
			}
		case term == "due":
			ok = c.Due != nil && !c.DueComplete
		case term == "overdue":
			ok = c.Due != nil && !c.DueComplete && c.Due.Before(now)
		default:
			ok = strings.Contains(strings.ToLower(c.Name), term) || strings.Contains(strings.ToLower(c.Desc), term)
		}
		if !ok {
			return false
		}
	}
	return true
}

func (v *boardView) listID() string {
	if v.bd == nil || v.col >= len(v.bd.Lists) {
		return ""
	}
	return v.bd.Lists[v.col].ID
}

func (v *boardView) row() int {
	if v.col >= len(v.cols) {
		return 0
	}
	return clamp(v.rows[v.listID()], 0, len(v.cols[v.col])-1)
}

func (v *boardView) setRow(r int) {
	if v.col < len(v.cols) {
		v.rows[v.listID()] = clamp(r, 0, len(v.cols[v.col])-1)
	}
}

func (v *boardView) sel() *trello.Card {
	if v.bd == nil || v.col >= len(v.cols) || len(v.cols[v.col]) == 0 {
		return nil
	}
	return v.cols[v.col][v.row()]
}

func (v *boardView) selectCard(id string) bool {
	if id == "" {
		return false
	}
	for ci, col := range v.cols {
		for ri, c := range col {
			if c.ID == id {
				v.col = ci
				v.rows[v.bd.Lists[ci].ID] = ri
				return true
			}
		}
	}
	return false
}

func (v *boardView) card(id string) *trello.Card {
	if v.bd == nil {
		return nil
	}
	for i := range v.bd.Cards {
		if v.bd.Cards[i].ID == id {
			return &v.bd.Cards[i]
		}
	}
	return nil
}

func (v *boardView) listName(id string) string {
	if v.bd != nil {
		for _, l := range v.bd.Lists {
			if l.ID == id {
				return l.Name
			}
		}
	}
	return ""
}

// posAt returns a position that places a card at index i of column col
// (ignoring the card being moved, skip).
func (v *boardView) posAt(col, i int, skip string) float64 {
	var cards []*trello.Card
	for _, c := range v.cols[col] {
		if c.ID != skip {
			cards = append(cards, c)
		}
	}
	var prev, next *float64
	if i > 0 && i-1 < len(cards) {
		prev = &cards[i-1].Pos
	}
	if i < len(cards) {
		next = &cards[i].Pos
	}
	return trello.PosBetween(prev, next)
}

func (m Model) updateBoard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	v := &m.board
	if v.filtering {
		switch msg.String() {
		case "esc":
			v.filtering = false
			v.filter.Blur()
			v.filter.SetValue("")
			v.rebuild()
			return m, nil
		case "enter":
			v.filtering = false
			v.filter.Blur()
			return m, nil
		}
		sel := v.sel()
		var cmd tea.Cmd
		v.filter, cmd = v.filter.Update(msg)
		v.rebuild()
		if sel != nil && !v.selectCard(sel.ID) {
			v.setRow(0)
		}
		return m, cmd
	}
	if v.bd == nil {
		switch msg.String() {
		case "q":
			return m, m.quit()
		case "b", "esc":
			m.screen = screenBoards
		}
		return m, nil
	}
	nLists := len(v.bd.Lists)
	c := v.sel()
	switch msg.String() {
	case "q":
		return m, m.quit()
	case "esc":
		if v.filter.Value() != "" {
			v.filter.SetValue("")
			sel := v.sel()
			v.rebuild()
			if sel != nil {
				v.selectCard(sel.ID)
			}
			return m, nil
		}
		m.screen = screenBoards
	case "b":
		m.screen = screenBoards
	case "h", "left":
		v.col = clamp(v.col-1, 0, nLists-1)
	case "l", "right":
		v.col = clamp(v.col+1, 0, nLists-1)
	case "j", "down":
		v.setRow(v.row() + 1)
	case "k", "up":
		v.setRow(v.row() - 1)
	case "g", "home":
		v.setRow(0)
	case "G", "end":
		v.setRow(1 << 30)
	case "0", "^":
		v.col = 0
	case "$":
		v.col = max(0, nLists-1)
	case "ctrl+d", "pgdown":
		v.setRow(v.row() + 5)
	case "ctrl+u", "pgup":
		v.setRow(v.row() - 5)
	case "/":
		v.filtering = true
		v.filter.Focus()
		return m, textinput.Blink
	case "r":
		v.loading = true
		return m, m.loadBoard(v.bd.ID, false, false)
	case "z":
		v.compact = !v.compact
	case "o":
		if c != nil {
			return m, openURL(c.URL)
		}
		return m, openURL(v.bd.URL)
	case "y":
		if c != nil {
			u := c.ShortURL
			if u == "" {
				u = c.URL
			}
			return m, copyText(u)
		}
	case "enter":
		if c != nil {
			return m, m.openCard(c.ID)
		}
	case "n", "a":
		return m, m.promptNewCard(false)
	case "O":
		return m, m.promptNewCard(true)
	case "N":
		return m, m.promptNewList()
	case "R":
		return m, m.promptRenameList()
	case "X":
		return m, m.confirmArchiveList()
	}
	if c == nil {
		return m, nil
	}
	switch msg.String() {
	case "J", "shift+down":
		return m, m.moveVert(c, 1)
	case "K", "shift+up":
		return m, m.moveVert(c, -1)
	case "H", "shift+left":
		return m, m.moveHoriz(c, -1)
	case "L", "shift+right":
		return m, m.moveHoriz(c, 1)
	case "M":
		return m, m.pickMoveList(c.ID)
	case "e":
		return m, m.promptRename(c.ID)
	case "E":
		return m, m.editDesc(c.ID)
	case "t":
		return m, m.pickLabels(c.ID)
	case "m":
		return m, m.pickMembers(c.ID)
	case "d":
		return m, m.promptDue(c.ID)
	case "D":
		return m, m.toggleDueComplete(c.ID)
	case " ":
		return m, m.toggleJoin(c.ID)
	case "x":
		return m, m.confirmArchive(c.ID)
	}
	return m, nil
}

// --- mutations -----------------------------------------------------------

func (m *Model) updateCard(id string, apply func(*trello.Card), u trello.CardUpdate, desc string) tea.Cmd {
	c := m.board.card(id)
	if c == nil {
		return nil
	}
	apply(c)
	sel := m.board.sel()
	m.board.rebuild()
	if sel != nil {
		m.board.selectCard(sel.ID)
	}
	be, ids := m.be, m.ids
	return m.mutate(desc, func(ctx context.Context) error {
		_, err := be.UpdateCard(ctx, ids.get(id), u)
		return err
	}, nil)
}

func (m *Model) moveVert(c *trello.Card, delta int) tea.Cmd {
	v := &m.board
	r := v.row()
	t := r + delta
	if t < 0 || t >= len(v.cols[v.col]) {
		return nil
	}
	// Index among the other cards where c should land.
	pos := v.posAt(v.col, t, c.ID)
	id := c.ID
	cmd := m.updateCard(id, func(c *trello.Card) { c.Pos = pos }, trello.CardUpdate{Pos: &pos}, "move card")
	v.selectCard(id)
	return cmd
}

func (m *Model) moveHoriz(c *trello.Card, delta int) tea.Cmd {
	v := &m.board
	t := v.col + delta
	if t < 0 || t >= len(v.bd.Lists) {
		return nil
	}
	r := min(v.row(), len(v.cols[t]))
	return m.moveTo(c.ID, t, r)
}

func (m *Model) moveTo(id string, col, row int) tea.Cmd {
	v := &m.board
	pos := v.posAt(col, row, id)
	listID := v.bd.Lists[col].ID
	cmd := m.updateCard(id, func(c *trello.Card) { c.IDList = listID; c.Pos = pos },
		trello.CardUpdate{IDList: &listID, Pos: &pos}, "move card")
	v.selectCard(id)
	return cmd
}

func (m *Model) promptNewCard(top bool) tea.Cmd {
	v := &m.board
	if len(v.bd.Lists) == 0 {
		m.setStatus("Create a list first (N)", true)
		return nil
	}
	col := v.col
	where := "bottom of"
	if top {
		where = "top of"
	}
	return m.openPrompt(prompt{
		title:  fmt.Sprintf("New card · %s %s", where, v.bd.Lists[col].Name),
		repeat: true,
		submit: func(m *Model, name string) tea.Cmd {
			name = strings.TrimSpace(name)
			if name == "" {
				return nil
			}
			v := &m.board
			var pos float64
			if top {
				pos = v.posAt(col, 0, "")
			} else {
				pos = v.posAt(col, len(v.cols[col]), "")
			}
			tmp := m.tmpID()
			listID := v.bd.Lists[col].ID
			v.bd.Cards = append(v.bd.Cards, trello.Card{ID: tmp, Name: name, IDList: listID, IDBoard: v.bd.ID, Pos: pos})
			v.rebuild()
			v.selectCard(tmp)
			be, ids := m.be, m.ids
			var created *trello.Card
			return m.mutate("create card", func(ctx context.Context) error {
				c, err := be.CreateCard(ctx, listID, name, pos)
				if err == nil {
					ids.set(tmp, c.ID)
					created = c
				}
				return err
			}, func(m *Model) {
				if c := m.board.card(tmp); c != nil {
					c.ID, c.URL, c.ShortURL = created.ID, created.URL, created.ShortURL
				}
				if m.card.cardID == tmp {
					m.card.cardID = created.ID
				}
			})
		},
	})
}

func (m *Model) promptRename(id string) tea.Cmd {
	c := m.board.card(id)
	return m.openPrompt(prompt{
		title: "Rename card",
		value: c.Name,
		submit: func(m *Model, name string) tea.Cmd {
			name = strings.TrimSpace(name)
			if name == "" {
				return nil
			}
			return m.updateCard(id, func(c *trello.Card) { c.Name = name }, trello.CardUpdate{Name: &name}, "rename card")
		},
	})
}

func (m *Model) promptDue(id string) tea.Cmd {
	c := m.board.card(id)
	val := ""
	if c.Due != nil {
		val = c.Due.Local().Format("2006-01-02 15:04")
	}
	return m.openPrompt(prompt{
		title: "Due date",
		hint:  "today · tomorrow · fri · +3d · 10/15 · 2026-10-15 14:00 · empty clears",
		value: val,
		submit: func(m *Model, s string) tea.Cmd {
			if strings.TrimSpace(s) == "" {
				return m.updateCard(id, func(c *trello.Card) { c.Due = nil }, trello.CardUpdate{ClearDue: true}, "clear due date")
			}
			t, err := parseDue(s, time.Now())
			if err != nil {
				m.setStatus(err.Error(), true)
				return nil
			}
			m.setStatus("Due "+t.Format("Mon Jan 2 15:04"), false)
			return m.updateCard(id, func(c *trello.Card) { c.Due = &t }, trello.CardUpdate{Due: &t}, "set due date")
		},
	})
}

func (m *Model) toggleDueComplete(id string) tea.Cmd {
	c := m.board.card(id)
	done := !c.DueComplete
	if done {
		m.setStatus("Marked done ✓  (x to archive)", false)
	} else {
		m.setStatus("Marked not done", false)
	}
	return m.updateCard(id, func(c *trello.Card) { c.DueComplete = done }, trello.CardUpdate{DueComplete: &done}, "mark done")
}

func toggleID(ids []string, id string) []string {
	if i := slices.Index(ids, id); i >= 0 {
		return slices.Delete(slices.Clone(ids), i, i+1)
	}
	return append(slices.Clone(ids), id)
}

func (m *Model) toggleLabel(cardID, labelID string) tea.Cmd {
	c := m.board.card(cardID)
	next := toggleID(c.IDLabels, labelID)
	return m.updateCard(cardID, func(c *trello.Card) { c.IDLabels = next }, trello.CardUpdate{IDLabels: &next}, "update labels")
}

func (m *Model) toggleMember(cardID, memberID string) tea.Cmd {
	c := m.board.card(cardID)
	next := toggleID(c.IDMembers, memberID)
	return m.updateCard(cardID, func(c *trello.Card) { c.IDMembers = next }, trello.CardUpdate{IDMembers: &next}, "update members")
}

func (m *Model) toggleJoin(cardID string) tea.Cmd {
	if m.me == nil {
		m.setStatus("Still loading your profile…", true)
		return nil
	}
	c := m.board.card(cardID)
	if slices.Contains(c.IDMembers, m.me.ID) {
		m.setStatus("Left card", false)
	} else {
		m.setStatus("Joined card", false)
	}
	return m.toggleMember(cardID, m.me.ID)
}

func (m *Model) confirmArchive(id string) tea.Cmd {
	c := m.board.card(id)
	m.modal = &confirm{
		text: fmt.Sprintf("Archive %q?", truncate(c.Name, 50)),
		yes: func(m *Model) tea.Cmd {
			return m.archiveCard(id)
		},
	}
	return nil
}

func (m *Model) archiveCard(id string) tea.Cmd {
	v := &m.board
	r := v.row()
	closed := true
	cmd := m.updateCard(id, func(c *trello.Card) { c.Closed = true }, trello.CardUpdate{Closed: &closed}, "archive card")
	v.setRow(r)
	m.setStatus("Card archived", false)
	return cmd
}

func (m *Model) pickMoveList(id string) tea.Cmd {
	v := &m.board
	var opts []pickOpt
	for i, l := range v.bd.Lists {
		opts = append(opts, pickOpt{id: fmt.Sprint(i), label: l.Name})
	}
	m.modal = &picker{
		title:  "Move to list",
		opts:   opts,
		cursor: v.col,
		choose: func(m *Model, o pickOpt) (tea.Cmd, bool) {
			var col int
			fmt.Sscan(o.id, &col)
			return m.moveTo(id, col, len(m.board.cols[col])), true
		},
	}
	return nil
}

func (m *Model) pickLabels(cardID string) tea.Cmd {
	v := &m.board
	if len(v.bd.Labels) == 0 {
		m.setStatus("This board has no labels", true)
		return nil
	}
	m.modal = &picker{
		title: "Labels",
		multi: true,
		optsFn: func(m *Model) []pickOpt {
			c := m.board.card(cardID)
			var opts []pickOpt
			for _, l := range m.board.bd.Labels {
				name := l.Name
				if name == "" {
					name = sDim.Render("(" + l.Color + ")")
				}
				opts = append(opts, pickOpt{id: l.ID, label: name, prefix: labelDot(l.Color) + " ", on: c != nil && slices.Contains(c.IDLabels, l.ID)})
			}
			return opts
		},
		choose: func(m *Model, o pickOpt) (tea.Cmd, bool) {
			return m.toggleLabel(cardID, o.id), false
		},
	}
	return nil
}

func (m *Model) pickMembers(cardID string) tea.Cmd {
	m.modal = &picker{
		title: "Members",
		multi: true,
		optsFn: func(m *Model) []pickOpt {
			c := m.board.card(cardID)
			var opts []pickOpt
			for _, mb := range m.board.bd.Members {
				opts = append(opts, pickOpt{id: mb.ID, label: mb.FullName + sDim.Render(" @"+mb.Username), on: c != nil && slices.Contains(c.IDMembers, mb.ID)})
			}
			return opts
		},
		choose: func(m *Model, o pickOpt) (tea.Cmd, bool) {
			return m.toggleMember(cardID, o.id), false
		},
	}
	return nil
}

func (m *Model) promptNewList() tea.Cmd {
	return m.openPrompt(prompt{
		title: "New list (after current)",
		submit: func(m *Model, name string) tea.Cmd {
			name = strings.TrimSpace(name)
			if name == "" {
				return nil
			}
			v := &m.board
			var prev, next *float64
			at := 0
			if len(v.bd.Lists) > 0 {
				prev = &v.bd.Lists[v.col].Pos
				at = v.col + 1
				if at < len(v.bd.Lists) {
					next = &v.bd.Lists[at].Pos
				}
			}
			pos := trello.PosBetween(prev, next)
			tmp := m.tmpID()
			boardID := v.bd.ID
			v.bd.Lists = slices.Insert(v.bd.Lists, at, trello.List{ID: tmp, Name: name, IDBoard: boardID, Pos: pos})
			v.rebuild()
			v.col = at
			be, ids := m.be, m.ids
			var created *trello.List
			return m.mutate("create list", func(ctx context.Context) error {
				l, err := be.CreateList(ctx, boardID, name, pos)
				if err == nil {
					ids.set(tmp, l.ID)
					created = l
				}
				return err
			}, func(m *Model) {
				for i := range m.board.bd.Lists {
					if m.board.bd.Lists[i].ID == tmp {
						m.board.bd.Lists[i].ID = created.ID
					}
				}
				for i := range m.board.bd.Cards {
					if m.board.bd.Cards[i].IDList == tmp {
						m.board.bd.Cards[i].IDList = created.ID
					}
				}
				m.board.rows[created.ID] = m.board.rows[tmp]
				m.board.rebuild()
			})
		},
	})
}

func (m *Model) promptRenameList() tea.Cmd {
	v := &m.board
	if len(v.bd.Lists) == 0 {
		return nil
	}
	id := v.bd.Lists[v.col].ID
	return m.openPrompt(prompt{
		title: "Rename list",
		value: v.bd.Lists[v.col].Name,
		submit: func(m *Model, name string) tea.Cmd {
			name = strings.TrimSpace(name)
			if name == "" {
				return nil
			}
			for i := range m.board.bd.Lists {
				if m.board.bd.Lists[i].ID == id {
					m.board.bd.Lists[i].Name = name
				}
			}
			be, ids := m.be, m.ids
			return m.mutate("rename list", func(ctx context.Context) error {
				return be.RenameList(ctx, ids.get(id), name)
			}, nil)
		},
	})
}

func (m *Model) confirmArchiveList() tea.Cmd {
	v := &m.board
	if len(v.bd.Lists) == 0 {
		return nil
	}
	l := v.bd.Lists[v.col]
	m.modal = &confirm{
		text: fmt.Sprintf("Archive list %q and its %d cards?", truncate(l.Name, 40), len(v.cols[v.col])),
		yes: func(m *Model) tea.Cmd {
			v := &m.board
			for i := range v.bd.Lists {
				if v.bd.Lists[i].ID == l.ID {
					v.bd.Lists = slices.Delete(v.bd.Lists, i, i+1)
					break
				}
			}
			v.rebuild()
			v.col = clamp(v.col, 0, len(v.bd.Lists)-1)
			be, ids := m.be, m.ids
			return m.mutate("archive list", func(ctx context.Context) error {
				return be.ArchiveList(ctx, ids.get(l.ID))
			}, nil)
		},
	}
	return nil
}

// --- rendering -----------------------------------------------------------

func (m *Model) viewBoard(h int) string {
	v := &m.board
	name := v.wantName
	if v.bd != nil {
		name = v.bd.Name
	}
	header := sTitleBar.Render(" " + name)
	if v.filtering || v.filter.Value() != "" {
		v.filter.Width = max(10, m.width/2)
		header += "   " + v.filter.View()
	}
	if v.bd == nil {
		return header + "\n\n" + sDim.Render("   Loading…")
	}
	lists := v.bd.Lists
	if len(lists) == 0 {
		return header + "\n\n" + sDim.Render("   This board has no lists. Press N to create one.")
	}

	const gap = 1
	const minW, maxW = 28, 40
	avail := m.width - 2
	nvis := min(len(lists), max(1, (avail+gap)/(minW+gap)))
	cw := min(maxW, (avail-(nvis-1)*gap)/nvis)
	if v.col < v.colOff {
		v.colOff = v.col
	}
	if v.col >= v.colOff+nvis {
		v.colOff = v.col - nvis + 1
	}
	v.colOff = clamp(v.colOff, 0, len(lists)-nvis)

	var more string
	if v.colOff > 0 {
		more += sDim.Render(fmt.Sprintf("‹ %d ", v.colOff))
	}
	if rest := len(lists) - v.colOff - nvis; rest > 0 {
		more += sDim.Render(fmt.Sprintf(" %d ›", rest))
	}
	header = spread(header, more, m.width)

	cols := make([]string, 0, nvis*2)
	for i := v.colOff; i < v.colOff+nvis; i++ {
		if i > v.colOff {
			cols = append(cols, strings.Repeat(" ", gap))
		}
		cols = append(cols, m.viewColumn(i, cw, h-2))
	}
	return header + "\n\n" + " " + lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}

func (m *Model) viewColumn(li, w, h int) string {
	v := &m.board
	l := v.bd.Lists[li]
	cards := v.cols[li]
	selCol := li == v.col
	count := fmt.Sprint(len(cards))
	var head string
	if selCol {
		head = sListHeadSel.Width(w).Render(spread(l.Name, count, w-2))
	} else {
		head = sListHead.Width(w).Render(spread(sBold.Render(l.Name), sDim.Render(count), w-2))
	}
	out := []string{head}
	avail := h - 1
	if len(cards) == 0 {
		out = append(out, sDim.Render(padRight("  (empty)", w)))
		return strings.Join(out, "\n")
	}

	selRow := v.rows[l.ID]
	selRow = clamp(selRow, 0, len(cards)-1)
	rendered := make(map[int]string)
	render := func(i int) string {
		if s, ok := rendered[i]; ok {
			return s
		}
		s := m.viewCard1(cards[i], w, selCol && i == selRow)
		rendered[i] = s
		return s
	}
	height := func(i int) int { return strings.Count(render(i), "\n") + 1 }

	sc := clamp(v.scroll[l.ID], 0, len(cards)-1)
	if selCol || sc > selRow {
		if selRow < sc {
			sc = selRow
		}
		for sc < selRow {
			used := 0
			if sc > 0 {
				used++
			}
			for i := sc; i <= selRow; i++ {
				used += height(i)
			}
			if selRow < len(cards)-1 {
				used++
			}
			if used <= avail {
				break
			}
			sc++
		}
	}
	v.scroll[l.ID] = sc

	used := 0
	if sc > 0 {
		out = append(out, sDim.Render(fmt.Sprintf("  ↑ %d more", sc)))
		used++
	}
	i := sc
	for ; i < len(cards); i++ {
		hgt := height(i)
		reserve := 0
		if i < len(cards)-1 {
			reserve = 1
		}
		if used+hgt+reserve > avail && i > sc {
			break
		}
		out = append(out, render(i))
		used += hgt
	}
	if i < len(cards) {
		out = append(out, sDim.Render(fmt.Sprintf("  ↓ %d more", len(cards)-i)))
	}
	return strings.Join(out, "\n")
}

// viewCard1 renders a single card tile for the board.
func (m *Model) viewCard1(c *trello.Card, w int, selected bool) string {
	v := &m.board
	now := time.Now()
	if v.compact {
		bar := " "
		if len(c.IDLabels) > 0 {
			bar = labelStyle(v.labels[c.IDLabels[0]].Color).Render(" ")
		}
		badge := ""
		if c.Due != nil {
			badge = dueStyle(*c.Due, c.DueComplete, now).Render(fmtDue(*c.Due, now))
		} else if c.DueComplete {
			badge = sOK.Render("✓ Done")
		}
		line := spread(bar+" "+c.Name, badge, w-1)
		if selected {
			return lipgloss.NewStyle().Foreground(cAccent).Bold(true).Render("▌") + lipgloss.NewStyle().Bold(true).Render(line)
		}
		return " " + line
	}

	iw := w - 4
	var lines []string
	if len(c.IDLabels) > 0 {
		var chips []string
		for _, id := range c.IDLabels {
			l, ok := v.labels[id]
			if !ok {
				continue
			}
			name := l.Name
			if name == "" {
				name = "  "
			}
			chips = append(chips, labelStyle(l.Color).Render(" "+truncate(name, 14)+" "))
		}
		if len(chips) > 0 {
			lines = append(lines, truncate(strings.Join(chips, " "), iw))
		}
	}
	title := wrap(c.Name, iw)
	if len(title) > 4 {
		title = append(title[:3], truncate(title[3]+"…", iw))
	}
	ts := lipgloss.NewStyle()
	if selected {
		ts = ts.Bold(true).Foreground(cBright)
	}
	if strings.HasPrefix(c.ID, "tmp-") {
		ts = ts.Foreground(cDim)
	}
	for _, t := range title {
		lines = append(lines, ts.Render(t))
	}
	var badges []string
	if c.Due != nil {
		icon := "◷ "
		if c.DueComplete {
			icon = "✓ "
		}
		badges = append(badges, dueStyle(*c.Due, c.DueComplete, now).Render(icon+fmtDue(*c.Due, now)))
	} else if c.DueComplete {
		badges = append(badges, sOK.Render("✓ Done"))
	}
	if c.Badges.Description {
		badges = append(badges, sDim.Render("≡"))
	}
	if c.Badges.Comments > 0 {
		badges = append(badges, sDim.Render(fmt.Sprintf("✎ %d", c.Badges.Comments)))
	}
	if c.Badges.Attachments > 0 {
		badges = append(badges, sDim.Render(fmt.Sprintf("⎘ %d", c.Badges.Attachments)))
	}
	if n := c.Badges.CheckItems; n > 0 {
		st := sDim
		if c.Badges.CheckItemsChecked == n {
			st = sOK
		}
		badges = append(badges, st.Render(fmt.Sprintf("☑ %d/%d", c.Badges.CheckItemsChecked, n)))
	}
	var mem []string
	for _, id := range c.IDMembers {
		if mb, ok := v.members[id]; ok {
			st := lipgloss.NewStyle().Foreground(cMag)
			if m.me != nil && id == m.me.ID {
				st = st.Bold(true)
			}
			mem = append(mem, st.Render(mb.Initials))
		}
	}
	bs, ms := strings.Join(badges, " "), strings.Join(mem, " ")
	switch {
	case bs == "" && ms == "":
	case lipgloss.Width(bs)+lipgloss.Width(ms)+1 > iw:
		lines = append(lines, truncate(bs, iw), spread("", ms, iw))
	default:
		lines = append(lines, spread(bs, ms, iw))
	}
	st := sCard
	if selected {
		st = sCardSel
	}
	return st.Width(w - 2).Render(strings.Join(lines, "\n"))
}
