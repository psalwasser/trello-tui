// Package ui implements the Bubble Tea interface.
package ui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/psalwasser/trello-tui/internal/config"
	"github.com/psalwasser/trello-tui/internal/trello"
)

type screen int

const (
	screenBoards screen = iota
	screenBoard
	screenCard
)

const autoRefresh = 45 * time.Second

// Messages.
type (
	boardsMsg struct {
		boards []trello.Board
		cached bool
		err    error
	}
	boardMsg struct {
		bd     *trello.BoardData
		cached bool
		silent bool
		err    error
	}
	detailMsg struct {
		cardID string
		d      *trello.CardDetail
		err    error
	}
	// opMsg reports completion of a background mutation.
	opMsg struct {
		err  error
		then func(*Model) // applied on success, in the UI goroutine
	}
	statusMsg   string
	editDoneMsg struct {
		cardID string
		kind   string // "desc" | "comment"
		text   string
		err    error
	}
	meMsg struct {
		me  *trello.Member
		err error
	}
	tickMsg time.Time
)

// serializer runs mutations one at a time, in the order they were issued,
// so rapid optimistic edits (e.g. moving a card several times) land in the
// right order. wrap must only be called from Update.
type serializer struct{ tail chan struct{} }

func (s *serializer) wrap(f func() tea.Msg) tea.Cmd {
	prev := s.tail
	done := make(chan struct{})
	s.tail = done
	return func() tea.Msg {
		if prev != nil {
			<-prev
		}
		defer close(done)
		return f()
	}
}

// ids maps temporary client-side IDs (from optimistic creates) to real ones.
type ids struct {
	mu sync.Mutex
	m  map[string]string
}

func (r *ids) set(tmp, real string) {
	r.mu.Lock()
	r.m[tmp] = real
	r.mu.Unlock()
}

func (r *ids) get(id string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.m[id]; ok {
		return v
	}
	return id
}

type Model struct {
	be     trello.Backend
	demo   bool
	width  int
	height int
	screen screen

	queue   serializer
	ids     *ids
	tmpSeq  int
	pending int
	resync  bool
	spin    spinner.Model

	status    string
	statusErr bool
	statusAt  time.Time

	boards boardsView
	board  boardView
	card   cardView

	me         *trello.Member
	modal      modal // nil when no modal is open
	help       bool
	helpScroll int
	state      config.State
}

func New(be trello.Backend, demo bool) Model {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = sAccent
	m := Model{
		be:   be,
		demo: demo,
		ids:  &ids{m: map[string]string{}},
		spin: sp,
	}
	m.boards = newBoardsView()
	m.board = newBoardView()
	if !demo {
		m.state = config.LoadState()
	}
	return m
}

func (m Model) Init() tea.Cmd {
	be := m.be
	loadMe := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		me, err := be.Me(ctx)
		return meMsg{me: me, err: err}
	}
	cmds := []tea.Cmd{m.spin.Tick, tick(), loadMe, m.loadBoards(true)}
	if m.state.LastBoardID != "" {
		cmds = append(cmds, func() tea.Msg { return m.openBoardCmd(m.state.LastBoardID) })
	}
	return tea.Batch(cmds...)
}

// openBoardCmd is used at startup: switch to the last board immediately.
func (m Model) openBoardCmd(id string) tea.Msg { return openBoardMsg{id: id} }

type openBoardMsg struct{ id string }

func tick() tea.Cmd {
	return tea.Tick(autoRefresh, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) loadBoards(useCache bool) tea.Cmd {
	be, demo := m.be, m.demo
	fetch := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		b, err := be.Boards(ctx)
		if err == nil && !demo {
			_ = config.WriteCache("boards", b)
		}
		return boardsMsg{boards: b, err: err}
	}
	if !useCache || demo {
		return fetch
	}
	cached := func() tea.Msg {
		var b []trello.Board
		if config.ReadCache("boards", &b) != nil {
			return nil
		}
		return boardsMsg{boards: b, cached: true}
	}
	return tea.Batch(cached, fetch)
}

func (m *Model) loadBoard(id string, useCache, silent bool) tea.Cmd {
	be, demo := m.be, m.demo
	fetch := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		bd, err := be.BoardData(ctx, id)
		if err == nil && !demo {
			_ = config.WriteCache("board-"+id, bd)
		}
		return boardMsg{bd: bd, err: err, silent: silent}
	}
	if !useCache || demo {
		return fetch
	}
	cached := func() tea.Msg {
		var bd trello.BoardData
		if config.ReadCache("board-"+id, &bd) != nil {
			return nil
		}
		return boardMsg{bd: &bd, cached: true}
	}
	return tea.Sequence(cached, fetch)
}

// mutate runs f in the background (serialized) with optimistic UI already
// applied by the caller. On failure the board is reloaded to resync.
func (m *Model) mutate(desc string, f func(ctx context.Context) error, then func(*Model)) tea.Cmd {
	m.pending++
	return m.queue.wrap(func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := f(ctx); err != nil {
			return opMsg{err: fmt.Errorf("%s: %w", desc, err)}
		}
		return opMsg{then: then}
	})
}

func (m *Model) setStatus(s string, isErr bool) {
	m.status, m.statusErr, m.statusAt = s, isErr, time.Now()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case tickMsg:
		cmds := []tea.Cmd{tick()}
		if m.board.bd != nil && m.pending == 0 && m.modal == nil && m.screen != screenBoards {
			cmds = append(cmds, m.loadBoard(m.board.bd.ID, false, true))
		}
		return m, tea.Batch(cmds...)

	case meMsg:
		if msg.err != nil {
			m.setStatus("Loading profile: "+msg.err.Error(), true)
		} else {
			m.me = msg.me
		}
		return m, nil

	case statusMsg:
		m.setStatus(string(msg), false)
		return m, nil

	case boardsMsg:
		if msg.err != nil {
			m.setStatus("Loading boards: "+msg.err.Error(), true)
			m.boards.loading = false
			return m, nil
		}
		m.boards.setBoards(msg.boards, m.state.LastBoardID)
		if !msg.cached {
			m.boards.loading = false
		}
		return m, nil

	case openBoardMsg:
		return m, m.openBoard(msg.id, "")

	case boardMsg:
		if msg.err != nil {
			if m.board.bd == nil {
				m.screen = screenBoards
				m.state.LastBoardID = ""
			}
			if !msg.silent {
				m.setStatus("Loading board: "+msg.err.Error(), true)
			}
			m.board.loading = false
			return m, nil
		}
		if m.board.wantID != "" && msg.bd.ID != m.board.wantID {
			return m, nil // stale response for a board we navigated away from
		}
		// Don't clobber optimistic state with a refresh that raced a mutation.
		if !msg.cached && m.pending > 0 {
			return m, nil
		}
		m.board.setData(msg.bd)
		if !msg.cached {
			m.board.loading = false
		}
		if m.screen == screenCard {
			m.card.refreshCard(&m)
		}
		return m, nil

	case detailMsg:
		if m.screen == screenCard && m.card.cardID == msg.cardID {
			if msg.err != nil {
				m.setStatus("Loading card: "+msg.err.Error(), true)
			} else {
				m.card.detail = msg.d
				// Start on the first check item rather than a header.
				for i, f := range m.card.focusables() {
					if f.item >= 0 {
						m.card.cursor = i
						break
					}
				}
			}
			m.card.loading = false
		}
		return m, nil

	case opMsg:
		m.pending--
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			m.resync = true
		} else if msg.then != nil {
			msg.then(&m)
		}
		// After a failure, reload once the queue drains so optimistic state
		// is replaced by what the server actually has.
		if m.resync && m.pending == 0 && m.board.bd != nil {
			m.resync = false
			return m, m.loadBoard(m.board.bd.ID, false, true)
		}
		return m, nil

	case editDoneMsg:
		return m.handleEditDone(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, m.quit()
	}
	// Fast typing can deliver several keys as one multi-rune event; replay
	// them individually unless a text input wants the whole chunk.
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste && !m.wantsText() {
		var cmds []tea.Cmd
		var model tea.Model = m
		for _, r := range msg.Runes {
			var cmd tea.Cmd
			model, cmd = model.(Model).handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			cmds = append(cmds, cmd)
		}
		return model, tea.Batch(cmds...)
	}
	if m.help {
		switch msg.String() {
		case "j", "down":
			m.helpScroll++
		case "k", "up":
			m.helpScroll = max(0, m.helpScroll-1)
		case "q":
			return m, m.quit()
		default:
			m.help = false
		}
		return m, nil
	}
	if m.modal != nil {
		md, cmd, done := m.modal.update(&m, msg)
		if done {
			m.modal = nil
		} else {
			m.modal = md
		}
		return m, cmd
	}
	if msg.String() == "?" && !m.textFocused() {
		m.help, m.helpScroll = true, 0
		return m, nil
	}
	switch m.screen {
	case screenBoards:
		return m.updateBoards(msg)
	case screenBoard:
		return m.updateBoard(msg)
	case screenCard:
		return m.updateCardView(msg)
	}
	return m, nil
}

func (m *Model) wantsText() bool {
	if _, ok := m.modal.(*prompt); ok {
		return true
	}
	return m.modal == nil && m.textFocused()
}

// textFocused reports whether a text input is capturing keystrokes.
func (m *Model) textFocused() bool {
	switch m.screen {
	case screenBoards:
		return m.boards.filter.Focused()
	case screenBoard:
		return m.board.filtering
	}
	return false
}

func (m *Model) quit() tea.Cmd {
	if !m.demo {
		_ = config.SaveState(m.state)
		if bd := m.board.bd; bd != nil && m.pending == 0 {
			_ = config.WriteCache("board-"+bd.ID, bd)
		}
	}
	return tea.Quit
}

func (m *Model) openBoard(id, name string) tea.Cmd {
	if m.board.bd == nil || m.board.bd.ID != id {
		m.board = newBoardView()
		m.board.wantID = id
		m.board.wantName = name
		m.board.loading = true
	}
	m.screen = screenBoard
	m.state.LastBoardID = id
	if !m.demo {
		_ = config.SaveState(m.state)
	}
	return m.loadBoard(id, true, false)
}

func (m *Model) tmpID() string {
	m.tmpSeq++
	return fmt.Sprintf("tmp-%d", m.tmpSeq)
}

func openURL(u string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", u)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
		default:
			cmd = exec.Command("xdg-open", u)
		}
		if err := cmd.Start(); err != nil {
			return statusMsg("open: " + err.Error())
		}
		go cmd.Wait()
		return statusMsg("Opened in browser")
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	var body string
	bodyH := m.height - 1
	switch m.screen {
	case screenBoards:
		body = m.viewBoards(bodyH)
	case screenBoard:
		body = m.viewBoard(bodyH)
	case screenCard:
		body = m.viewCard(bodyH)
	}
	body = lipgloss.NewStyle().MaxHeight(bodyH).Render(body)
	if h := lipgloss.Height(body); h < bodyH {
		body += strings.Repeat("\n", bodyH-h)
	}
	if m.help {
		body = overlay(body, m.viewHelp(), m.width, bodyH)
	} else if m.modal != nil {
		body = overlay(body, m.modal.view(&m), m.width, bodyH)
	}
	return body + "\n" + m.viewStatus()
}

func (m *Model) viewStatus() string {
	var left string
	if m.pending > 0 || m.boards.loading && m.screen == screenBoards || m.board.loading && m.screen != screenBoards {
		left = m.spin.View() + " "
	}
	if m.status != "" && time.Since(m.statusAt) < 6*time.Second || m.statusErr && m.status != "" {
		if m.statusErr {
			left += sErr.Render(m.status)
		} else {
			left += sOK.Render(m.status)
		}
	} else {
		left += m.hints()
	}
	right := sDim.Render("? help")
	if m.demo {
		right = lipgloss.NewStyle().Foreground(cMag).Render("DEMO ") + right
	}
	return spread(left, right, m.width)
}

func (m *Model) hints() string {
	k := func(key, desc string) string { return sKey.Render(key) + sDim.Render(" "+desc) }
	var parts []string
	switch m.screen {
	case screenBoards:
		parts = []string{k("enter", "open"), k("/", "filter"), k("r", "refresh"), k("q", "quit")}
	case screenBoard:
		parts = []string{k("hjkl", "move"), k("enter", "open"), k("n", "new"), k("HJKL", "drag"), k("D", "done"), k("x", "archive"), k("d", "due"), k("/", "filter")}
	case screenCard:
		parts = []string{k("e", "title"), k("E", "desc"), k("c", "comment"), k("space", "check"), k("D", "done"), k("A", "archive"), k("esc", "back")}
	}
	return strings.Join(parts, "  ")
}
