package trello

import (
	"context"
	"time"
)

type Board struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Desc             string     `json:"desc"`
	URL              string     `json:"url"`
	Closed           bool       `json:"closed"`
	Starred          bool       `json:"starred"`
	IDOrganization   string     `json:"idOrganization"`
	DateLastActivity *time.Time `json:"dateLastActivity"`
	Prefs            struct {
		BackgroundColor string `json:"backgroundColor"`
	} `json:"prefs"`

	// Filled in client-side.
	WorkspaceName string `json:"workspaceName,omitempty"`
}

type List struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	IDBoard string  `json:"idBoard"`
	Pos     float64 `json:"pos"`
	Closed  bool    `json:"closed"`
}

type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Member struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
	Username string `json:"username"`
	Initials string `json:"initials"`
}

type Badges struct {
	Comments          int  `json:"comments"`
	CheckItems        int  `json:"checkItems"`
	CheckItemsChecked int  `json:"checkItemsChecked"`
	Attachments       int  `json:"attachments"`
	Description       bool `json:"description"`
}

type Card struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Desc             string     `json:"desc"`
	IDList           string     `json:"idList"`
	IDBoard          string     `json:"idBoard"`
	Pos              float64    `json:"pos"`
	Due              *time.Time `json:"due"`
	DueComplete      bool       `json:"dueComplete"`
	IDLabels         []string   `json:"idLabels"`
	IDMembers        []string   `json:"idMembers"`
	Badges           Badges     `json:"badges"`
	URL              string     `json:"url"`
	ShortURL         string     `json:"shortUrl"`
	Closed           bool       `json:"closed"`
	DateLastActivity *time.Time `json:"dateLastActivity"`
}

type CheckItem struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	State string  `json:"state"` // "complete" | "incomplete"
	Pos   float64 `json:"pos"`
}

func (c CheckItem) Done() bool { return c.State == "complete" }

type Checklist struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	IDCard     string      `json:"idCard"`
	Pos        float64     `json:"pos"`
	CheckItems []CheckItem `json:"checkItems"`
}

type Comment struct {
	ID            string    `json:"id"`
	Date          time.Time `json:"date"`
	MemberCreator Member    `json:"memberCreator"`
	Data          struct {
		Text string `json:"text"`
	} `json:"data"`
}

// BoardData is everything needed to render a board.
type BoardData struct {
	Board
	Lists   []List   `json:"lists"`
	Cards   []Card   `json:"cards"`
	Labels  []Label  `json:"labels"`
	Members []Member `json:"members"`
}

// CardDetail is the extra data loaded when a card is opened.
type CardDetail struct {
	Checklists []Checklist
	Comments   []Comment
}

// CardUpdate describes a partial card update. Nil fields are left unchanged.
type CardUpdate struct {
	Name        *string
	Desc        *string
	IDList      *string
	Pos         *float64
	Due         *time.Time
	ClearDue    bool
	DueComplete *bool
	Closed      *bool
	IDLabels    *[]string
	IDMembers   *[]string
}

// Backend is the set of operations the UI needs. Implemented by the real
// HTTP client and by an in-memory demo backend.
type Backend interface {
	Me(ctx context.Context) (*Member, error)
	Boards(ctx context.Context) ([]Board, error)
	BoardData(ctx context.Context, boardID string) (*BoardData, error)
	CardDetail(ctx context.Context, cardID string) (*CardDetail, error)

	CreateCard(ctx context.Context, listID, name string, pos float64) (*Card, error)
	UpdateCard(ctx context.Context, cardID string, u CardUpdate) (*Card, error)
	DeleteCard(ctx context.Context, cardID string) error
	AddComment(ctx context.Context, cardID, text string) (*Comment, error)

	CreateChecklist(ctx context.Context, cardID, name string) (*Checklist, error)
	AddCheckItem(ctx context.Context, checklistID, name string) (*CheckItem, error)
	SetCheckItem(ctx context.Context, cardID, itemID string, done bool) error
	DeleteCheckItem(ctx context.Context, checklistID, itemID string) error

	CreateList(ctx context.Context, boardID, name string, pos float64) (*List, error)
	RenameList(ctx context.Context, listID, name string) error
	ArchiveList(ctx context.Context, listID string) error
}

// PosBetween returns a Trello position value that sorts between prev and
// next. Pass nil for an open end.
func PosBetween(prev, next *float64) float64 {
	const step = 65536
	switch {
	case prev == nil && next == nil:
		return step
	case prev == nil:
		return *next / 2
	case next == nil:
		return *prev + step
	default:
		return (*prev + *next) / 2
	}
}
