package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/psalwasser/trello-tui/internal/demo"
)

func TestParseDue(t *testing.T) {
	now := time.Date(2026, 10, 11, 10, 0, 0, 0, time.Local) // a Sunday
	cases := map[string]string{
		"today":            "2026-10-11 09:00",
		"tomorrow 2pm":     "2026-10-12 14:00",
		"fri":              "2026-10-16 09:00",
		"sun":              "2026-10-18 09:00",
		"+3d":              "2026-10-14 09:00",
		"+2w":              "2026-10-25 09:00",
		"2026-12-01 17:30": "2026-12-01 17:30",
		"10/15":            "2026-10-15 09:00",
		"1/5":              "2027-01-05 09:00",
		"3pm":              "2026-10-11 15:00",
	}
	for in, want := range cases {
		got, err := parseDue(in, now)
		if err != nil {
			t.Errorf("parseDue(%q): %v", in, err)
			continue
		}
		if s := got.Format("2006-01-02 15:04"); s != want {
			t.Errorf("parseDue(%q) = %s, want %s", in, s, want)
		}
	}
	for _, bad := range []string{"someday", "25:00", "+xd"} {
		if _, err := parseDue(bad, now); err == nil {
			t.Errorf("parseDue(%q) should fail", bad)
		}
	}
}

// harness drives the Model synchronously, executing commands inline.
type harness struct {
	t *testing.T
	m Model
}

func newHarness(t *testing.T) *harness {
	be := demo.New()
	be.Latency = 0
	h := &harness{t: t, m: New(be, true)}
	h.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	h.run(h.m.loadBoards(false))
	h.run(h.m.openBoard(h.m.boards.shown[1].ID, "")) // Product Roadmap (starred, sorted second)
	return h
}

func (h *harness) send(msg tea.Msg) {
	m, cmd := h.m.Update(msg)
	h.m = m.(Model)
	h.run(cmd)
}

// run executes cmd and feeds resulting messages back, skipping timers.
func (h *harness) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(200 * time.Millisecond):
		return // a tick/blink; ignore
	}
	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
	case nil:
	default:
		h.send(msg)
	}
}

func (h *harness) keys(ks ...string) {
	for _, k := range ks {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEscape}
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		h.send(msg)
	}
}

func (h *harness) reload() {
	h.run(h.m.loadBoard(h.m.board.bd.ID, false, false))
}

func TestMoveCardPersists(t *testing.T) {
	h := newHarness(t)
	if h.m.board.bd.Name != "Product Roadmap" {
		t.Fatalf("opened %q", h.m.board.bd.Name)
	}
	first := h.m.board.sel().Name
	h.keys("L", "J") // move to "Up Next", then down one
	h.reload()
	col := h.m.board.cols[1]
	if col[1].Name != first {
		t.Fatalf("expected %q at Up Next[1], got %q", first, col[1].Name)
	}
	if h.m.board.sel().Name != first {
		t.Fatalf("selection didn't follow the moved card")
	}
}

func TestCreateCardReplacesTempID(t *testing.T) {
	h := newHarness(t)
	h.keys("n")
	h.keys("Brand new card", "enter", "esc")
	c := h.m.board.sel()
	if c.Name != "Brand new card" || strings.HasPrefix(c.ID, "tmp-") {
		t.Fatalf("card not created with real id: %+v", c)
	}
	h.reload()
	last := h.m.board.cols[0][len(h.m.board.cols[0])-1]
	if last.Name != "Brand new card" {
		t.Fatalf("new card not persisted at bottom, got %q", last.Name)
	}
}

func TestBatchedRunesAreSplit(t *testing.T) {
	h := newHarness(t)
	h.keys("jj")
	if h.m.board.row() != 2 {
		t.Fatalf("row = %d, want 2", h.m.board.row())
	}
}

func TestChecklistToggleAndComment(t *testing.T) {
	h := newHarness(t)
	h.keys("l", "enter") // SSO with Okta: 1/4 checked
	if h.m.screen != screenCard || h.m.card.detail == nil {
		t.Fatal("card view not open")
	}
	h.keys("j", "space") // second item
	h.keys("c")
	h.keys("hello", "enter")
	id := h.m.card.cardID
	d, err := h.m.be.CardDetail(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Checklists[0].CheckItems[1].Done() {
		t.Fatal("check item not toggled on backend")
	}
	if len(d.Comments) != 1 || d.Comments[0].Data.Text != "hello" {
		t.Fatalf("comment not saved: %+v", d.Comments)
	}
	if b := h.m.board.card(id).Badges; b.CheckItemsChecked != 2 || b.Comments != 1 {
		t.Fatalf("badges not updated: %+v", b)
	}
}

func TestFilter(t *testing.T) {
	h := newHarness(t)
	h.keys("/", "#bug", "enter")
	total := 0
	for _, col := range h.m.board.cols {
		total += len(col)
	}
	if total != 3 {
		t.Fatalf("#bug filter matched %d cards, want 3", total)
	}
}

func TestViewRendersAtSmallSizes(t *testing.T) {
	h := newHarness(t)
	for _, sz := range [][2]int{{40, 10}, {80, 24}, {200, 60}} {
		h.send(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		for _, screen := range []string{"board", "card"} {
			if screen == "card" {
				h.keys("enter")
			}
			out := h.m.View()
			if lines := strings.Count(out, "\n") + 1; lines != sz[1] {
				t.Errorf("%s at %dx%d rendered %d lines", screen, sz[0], sz[1], lines)
			}
			if screen == "card" {
				h.keys("esc")
			}
		}
	}
}
