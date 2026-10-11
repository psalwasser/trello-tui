// Package demo provides an in-memory Trello backend with sample data, used by
// `trello-tui --demo` and tests.
package demo

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/psalwasser/trello-tui/internal/trello"
)

type Backend struct {
	mu         sync.Mutex
	seq        int
	me         trello.Member
	boards     []trello.Board
	lists      map[string][]trello.List // boardID -> lists
	cards      map[string]*trello.Card
	labels     map[string][]trello.Label
	members    []trello.Member
	checklists map[string][]trello.Checklist // cardID -> checklists
	comments   map[string][]trello.Comment
	Latency    time.Duration
}

func (b *Backend) id() string {
	b.seq++
	return fmt.Sprintf("demo%06d", b.seq)
}

func New() *Backend {
	b := &Backend{
		lists:      map[string][]trello.List{},
		cards:      map[string]*trello.Card{},
		labels:     map[string][]trello.Label{},
		checklists: map[string][]trello.Checklist{},
		comments:   map[string][]trello.Comment{},
		Latency:    120 * time.Millisecond,
	}
	b.me = trello.Member{ID: "me", FullName: "Demo User", Username: "demo", Initials: "DU"}
	b.members = []trello.Member{
		b.me,
		{ID: "m2", FullName: "Ada Lovelace", Username: "ada", Initials: "AL"},
		{ID: "m3", FullName: "Grace Hopper", Username: "grace", Initials: "GH"},
	}
	now := time.Now()
	day := func(n int) *time.Time {
		t := time.Date(now.Year(), now.Month(), now.Day(), 17, 0, 0, 0, time.Local).AddDate(0, 0, n)
		return &t
	}

	type seedCard struct {
		name    string
		desc    string
		labels  []int
		members []string
		due     *time.Time
		done    bool
		checks  []string
		checked int
		comment string
	}
	seed := func(name, ws string, starred bool, lists []string, cards map[string][]seedCard) {
		bid := b.id()
		act := now.Add(-time.Duration(len(b.boards)) * time.Hour)
		b.boards = append(b.boards, trello.Board{ID: bid, Name: name, Starred: starred, WorkspaceName: ws, DateLastActivity: &act, URL: "https://trello.com/b/" + bid})
		b.labels[bid] = []trello.Label{
			{ID: b.id(), Name: "Bug", Color: "red"},
			{ID: b.id(), Name: "Feature", Color: "green"},
			{ID: b.id(), Name: "Design", Color: "purple"},
			{ID: b.id(), Name: "Urgent", Color: "orange"},
			{ID: b.id(), Name: "Docs", Color: "blue"},
			{ID: b.id(), Name: "", Color: "yellow"},
		}
		for i, ln := range lists {
			lid := b.id()
			b.lists[bid] = append(b.lists[bid], trello.List{ID: lid, Name: ln, IDBoard: bid, Pos: float64(i+1) * 65536})
			for j, sc := range cards[ln] {
				cid := b.id()
				c := &trello.Card{ID: cid, Name: sc.name, Desc: sc.desc, IDList: lid, IDBoard: bid, Pos: float64(j+1) * 65536, Due: sc.due, DueComplete: sc.done, IDMembers: sc.members, URL: "https://trello.com/c/" + cid}
				for _, li := range sc.labels {
					c.IDLabels = append(c.IDLabels, b.labels[bid][li].ID)
				}
				if len(sc.checks) > 0 {
					cl := trello.Checklist{ID: b.id(), Name: "Checklist", IDCard: cid, Pos: 1}
					for k, item := range sc.checks {
						state := "incomplete"
						if k < sc.checked {
							state = "complete"
						}
						cl.CheckItems = append(cl.CheckItems, trello.CheckItem{ID: b.id(), Name: item, State: state, Pos: float64(k + 1)})
					}
					b.checklists[cid] = []trello.Checklist{cl}
				}
				if sc.comment != "" {
					cm := trello.Comment{ID: b.id(), Date: now.Add(-3 * time.Hour), MemberCreator: b.members[1]}
					cm.Data.Text = sc.comment
					b.comments[cid] = []trello.Comment{cm}
				}
				b.cards[cid] = c
				b.refreshBadges(c)
			}
		}
	}

	seed("Product Roadmap", "Acme Corp", true,
		[]string{"Backlog", "Up Next", "In Progress", "Review", "Done"},
		map[string][]seedCard{
			"Backlog": {
				{name: "Dark mode for the mobile app", labels: []int{1, 2}, desc: "Users keep asking. Follow the OS setting by default."},
				{name: "Export boards to CSV", labels: []int{1}},
				{name: "Investigate flaky integration tests on CI", labels: []int{0}, members: []string{"m3"}},
				{name: "Rewrite onboarding copy", labels: []int{4}},
				{name: "Keyboard shortcuts cheat sheet"},
			},
			"Up Next": {
				{name: "SSO with Okta", labels: []int{1, 3}, members: []string{"me"}, due: day(5),
					desc:   "Enterprise customers need SAML SSO.\n\n- Okta first\n- Azure AD next quarter",
					checks: []string{"Spike SAML library", "Metadata endpoint", "Admin settings UI", "Docs"}, checked: 1},
				{name: "Rate limit the public API", labels: []int{0, 3}, due: day(2)},
			},
			"In Progress": {
				{name: "New billing page", labels: []int{1, 2}, members: []string{"me", "m2"}, due: day(1),
					desc:   "Designs are in Figma. Stripe Checkout for upgrades.",
					checks: []string{"Plan comparison table", "Stripe checkout", "Invoices list", "Cancel flow", "QA"}, checked: 3,
					comment: "Stripe webhooks are wired up on staging."},
				{name: "Fix timezone bug in due-date reminders", labels: []int{0, 3}, members: []string{"m3"}, due: day(-1)},
			},
			"Review": {
				{name: "Search results ranking tweaks", labels: []int{1}, members: []string{"m2"}, comment: "LGTM pending one nit."},
			},
			"Done": {
				{name: "Upgrade to Postgres 17", labels: []int{5}, due: day(-4), done: true},
				{name: "Ship v2.3 release notes", labels: []int{4}, due: day(-2), done: true},
			},
		})
	seed("Personal", "", true,
		[]string{"To Do", "Doing", "Done"},
		map[string][]seedCard{
			"To Do": {
				{name: "Renew passport", due: day(14), labels: []int{3}},
				{name: "Book dentist appointment"},
				{name: "Groceries", checks: []string{"Eggs", "Coffee", "Spinach", "Rice"}, checked: 2},
			},
			"Doing": {{name: "Read: Designing Data-Intensive Applications"}},
			"Done":  {{name: "Fix bike tyre", done: true}},
		})
	seed("Marketing Site", "Acme Corp", false,
		[]string{"Ideas", "Writing", "Published"},
		map[string][]seedCard{
			"Ideas":     {{name: "Customer story: Globex"}, {name: "Pricing page refresh", labels: []int{2}}},
			"Writing":   {{name: "Launch blog post for SSO", members: []string{"m2"}, due: day(7)}},
			"Published": {{name: "Q3 changelog"}},
		})
	return b
}

func (b *Backend) wait(ctx context.Context) error {
	select {
	case <-time.After(b.Latency):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *Backend) refreshBadges(c *trello.Card) {
	c.Badges = trello.Badges{Description: c.Desc != "", Comments: len(b.comments[c.ID])}
	for _, cl := range b.checklists[c.ID] {
		for _, it := range cl.CheckItems {
			c.Badges.CheckItems++
			if it.Done() {
				c.Badges.CheckItemsChecked++
			}
		}
	}
}

func (b *Backend) Me(ctx context.Context) (*trello.Member, error) {
	m := b.me
	return &m, b.wait(ctx)
}

func (b *Backend) Boards(ctx context.Context) ([]trello.Board, error) {
	if err := b.wait(ctx); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]trello.Board(nil), b.boards...), nil
}

func (b *Backend) BoardData(ctx context.Context, boardID string) (*trello.BoardData, error) {
	if err := b.wait(ctx); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var bd trello.BoardData
	for _, bo := range b.boards {
		if bo.ID == boardID {
			bd.Board = bo
		}
	}
	if bd.ID == "" {
		return nil, fmt.Errorf("board not found")
	}
	for _, l := range b.lists[boardID] {
		if !l.Closed {
			bd.Lists = append(bd.Lists, l)
		}
	}
	for _, c := range b.cards {
		if c.IDBoard == boardID && !c.Closed {
			bd.Cards = append(bd.Cards, *c)
		}
	}
	sort.Slice(bd.Lists, func(i, j int) bool { return bd.Lists[i].Pos < bd.Lists[j].Pos })
	sort.Slice(bd.Cards, func(i, j int) bool { return bd.Cards[i].Pos < bd.Cards[j].Pos })
	bd.Labels = append(bd.Labels, b.labels[boardID]...)
	bd.Members = append(bd.Members, b.members...)
	return &bd, nil
}

func (b *Backend) CardDetail(ctx context.Context, cardID string) (*trello.CardDetail, error) {
	if err := b.wait(ctx); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	d := &trello.CardDetail{}
	for _, cl := range b.checklists[cardID] {
		cl.CheckItems = append([]trello.CheckItem(nil), cl.CheckItems...)
		d.Checklists = append(d.Checklists, cl)
	}
	// Newest first, like the API.
	cms := b.comments[cardID]
	for i := len(cms) - 1; i >= 0; i-- {
		d.Comments = append(d.Comments, cms[i])
	}
	return d, nil
}

func (b *Backend) CreateCard(ctx context.Context, listID, name string, pos float64) (*trello.Card, error) {
	if err := b.wait(ctx); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var boardID string
	for bid, ls := range b.lists {
		for _, l := range ls {
			if l.ID == listID {
				boardID = bid
			}
		}
	}
	c := &trello.Card{ID: b.id(), Name: name, IDList: listID, IDBoard: boardID, Pos: pos}
	c.URL = "https://trello.com/c/" + c.ID
	b.cards[c.ID] = c
	out := *c
	return &out, nil
}

func (b *Backend) UpdateCard(ctx context.Context, cardID string, u trello.CardUpdate) (*trello.Card, error) {
	if err := b.wait(ctx); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.cards[cardID]
	if !ok {
		return nil, fmt.Errorf("card not found")
	}
	if u.Name != nil {
		c.Name = *u.Name
	}
	if u.Desc != nil {
		c.Desc = *u.Desc
	}
	if u.IDList != nil {
		c.IDList = *u.IDList
	}
	if u.Pos != nil {
		c.Pos = *u.Pos
	}
	if u.ClearDue {
		c.Due = nil
	} else if u.Due != nil {
		d := *u.Due
		c.Due = &d
	}
	if u.DueComplete != nil {
		c.DueComplete = *u.DueComplete
	}
	if u.Closed != nil {
		c.Closed = *u.Closed
	}
	if u.IDLabels != nil {
		c.IDLabels = append([]string(nil), (*u.IDLabels)...)
	}
	if u.IDMembers != nil {
		c.IDMembers = append([]string(nil), (*u.IDMembers)...)
	}
	b.refreshBadges(c)
	out := *c
	return &out, nil
}

func (b *Backend) DeleteCard(ctx context.Context, cardID string) error {
	if err := b.wait(ctx); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.cards, cardID)
	return nil
}

func (b *Backend) AddComment(ctx context.Context, cardID, text string) (*trello.Comment, error) {
	if err := b.wait(ctx); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	cm := trello.Comment{ID: b.id(), Date: time.Now(), MemberCreator: b.me}
	cm.Data.Text = text
	b.comments[cardID] = append(b.comments[cardID], cm)
	if c, ok := b.cards[cardID]; ok {
		b.refreshBadges(c)
	}
	return &cm, nil
}

func (b *Backend) CreateChecklist(ctx context.Context, cardID, name string) (*trello.Checklist, error) {
	if err := b.wait(ctx); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	cl := trello.Checklist{ID: b.id(), Name: name, IDCard: cardID, Pos: float64(len(b.checklists[cardID]) + 1)}
	b.checklists[cardID] = append(b.checklists[cardID], cl)
	return &cl, nil
}

func (b *Backend) findChecklist(id string) (cardID string, idx int) {
	for cid, cls := range b.checklists {
		for i, cl := range cls {
			if cl.ID == id {
				return cid, i
			}
		}
	}
	return "", -1
}

func (b *Backend) AddCheckItem(ctx context.Context, checklistID, name string) (*trello.CheckItem, error) {
	if err := b.wait(ctx); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	cid, i := b.findChecklist(checklistID)
	if i < 0 {
		return nil, fmt.Errorf("checklist not found")
	}
	cl := &b.checklists[cid][i]
	it := trello.CheckItem{ID: b.id(), Name: name, State: "incomplete", Pos: float64(len(cl.CheckItems) + 1)}
	cl.CheckItems = append(cl.CheckItems, it)
	if c, ok := b.cards[cid]; ok {
		b.refreshBadges(c)
	}
	return &it, nil
}

func (b *Backend) SetCheckItem(ctx context.Context, cardID, itemID string, done bool) error {
	if err := b.wait(ctx); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.checklists[cardID] {
		items := b.checklists[cardID][i].CheckItems
		for j := range items {
			if items[j].ID == itemID {
				items[j].State = map[bool]string{true: "complete", false: "incomplete"}[done]
			}
		}
	}
	if c, ok := b.cards[cardID]; ok {
		b.refreshBadges(c)
	}
	return nil
}

func (b *Backend) DeleteCheckItem(ctx context.Context, checklistID, itemID string) error {
	if err := b.wait(ctx); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	cid, i := b.findChecklist(checklistID)
	if i < 0 {
		return fmt.Errorf("checklist not found")
	}
	cl := &b.checklists[cid][i]
	for j, it := range cl.CheckItems {
		if it.ID == itemID {
			cl.CheckItems = append(cl.CheckItems[:j], cl.CheckItems[j+1:]...)
			break
		}
	}
	if c, ok := b.cards[cid]; ok {
		b.refreshBadges(c)
	}
	return nil
}

func (b *Backend) CreateList(ctx context.Context, boardID, name string, pos float64) (*trello.List, error) {
	if err := b.wait(ctx); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	l := trello.List{ID: b.id(), Name: name, IDBoard: boardID, Pos: pos}
	b.lists[boardID] = append(b.lists[boardID], l)
	return &l, nil
}

func (b *Backend) updateList(listID string, f func(*trello.List)) {
	for bid := range b.lists {
		for i := range b.lists[bid] {
			if b.lists[bid][i].ID == listID {
				f(&b.lists[bid][i])
			}
		}
	}
}

func (b *Backend) RenameList(ctx context.Context, listID, name string) error {
	if err := b.wait(ctx); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.updateList(listID, func(l *trello.List) { l.Name = name })
	return nil
}

func (b *Backend) ArchiveList(ctx context.Context, listID string) error {
	if err := b.wait(ctx); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.updateList(listID, func(l *trello.List) { l.Closed = true })
	return nil
}
