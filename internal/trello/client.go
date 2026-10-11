package trello

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

const apiBase = "https://api.trello.com/1"

// Client talks to the Trello REST API.
type Client struct {
	Key, Token string
	HTTP       *http.Client
	Base       string
}

func NewClient(key, token string) *Client {
	return &Client{
		Key:   key,
		Token: token,
		Base:  apiBase,
		HTTP: &http.Client{
			Timeout: 20 * time.Second,
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				MaxIdleConnsPerHost: 8,
				IdleConnTimeout:     90 * time.Second,
				ForceAttemptHTTP2:   true,
			},
		},
	}
}

type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	msg := strings.TrimSpace(e.Body)
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return fmt.Sprintf("trello: %d %s", e.Status, msg)
}

func (c *Client) do(ctx context.Context, method, path string, params url.Values, out any) error {
	u := c.Base + path
	var body io.Reader
	if method == http.MethodGet || method == http.MethodDelete {
		if len(params) > 0 {
			u += "?" + params.Encode()
		}
	} else if params != nil {
		body = strings.NewReader(params.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf(`OAuth oauth_consumer_key="%s", oauth_token="%s"`, c.Key, c.Token))
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode, Body: string(data)}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *Client) Me(ctx context.Context) (*Member, error) {
	var m Member
	err := c.do(ctx, "GET", "/members/me", url.Values{"fields": {"fullName,username,initials"}}, &m)
	return &m, err
}

func (c *Client) Boards(ctx context.Context) ([]Board, error) {
	var (
		boards []Board
		orgs   []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
		}
		stars []struct {
			IDBoard string `json:"idBoard"`
		}
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return c.do(gctx, "GET", "/members/me/boards", url.Values{
			"filter": {"open"},
			"fields": {"name,desc,url,closed,starred,idOrganization,dateLastActivity,prefs"},
		}, &boards)
	})
	g.Go(func() error {
		return c.do(gctx, "GET", "/members/me/organizations", url.Values{"fields": {"displayName"}}, &orgs)
	})
	g.Go(func() error {
		return c.do(gctx, "GET", "/members/me/boardStars", nil, &stars)
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	orgNames := map[string]string{}
	for _, o := range orgs {
		orgNames[o.ID] = o.DisplayName
	}
	starred := map[string]bool{}
	for _, s := range stars {
		starred[s.IDBoard] = true
	}
	for i := range boards {
		boards[i].WorkspaceName = orgNames[boards[i].IDOrganization]
		if starred[boards[i].ID] {
			boards[i].Starred = true
		}
	}
	return boards, nil
}

func (c *Client) BoardData(ctx context.Context, boardID string) (*BoardData, error) {
	var bd BoardData
	err := c.do(ctx, "GET", "/boards/"+boardID, url.Values{
		"fields":        {"name,desc,url,closed,idOrganization,prefs"},
		"lists":         {"open"},
		"list_fields":   {"name,idBoard,pos,closed"},
		"cards":         {"open"},
		"card_fields":   {"name,desc,idList,idBoard,pos,due,dueComplete,idLabels,idMembers,badges,url,shortUrl,closed,dateLastActivity"},
		"labels":        {"all"},
		"label_fields":  {"name,color"},
		"labels_limit":  {"1000"},
		"members":       {"all"},
		"member_fields": {"fullName,username,initials"},
	}, &bd)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(bd.Lists, func(i, j int) bool { return bd.Lists[i].Pos < bd.Lists[j].Pos })
	sort.SliceStable(bd.Cards, func(i, j int) bool { return bd.Cards[i].Pos < bd.Cards[j].Pos })
	return &bd, nil
}

func (c *Client) CardDetail(ctx context.Context, cardID string) (*CardDetail, error) {
	var d CardDetail
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return c.do(gctx, "GET", "/cards/"+cardID+"/checklists", url.Values{"checkItem_fields": {"name,state,pos"}}, &d.Checklists)
	})
	g.Go(func() error {
		return c.do(gctx, "GET", "/cards/"+cardID+"/actions", url.Values{"filter": {"commentCard"}, "limit": {"100"}}, &d.Comments)
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	sort.SliceStable(d.Checklists, func(i, j int) bool { return d.Checklists[i].Pos < d.Checklists[j].Pos })
	for _, cl := range d.Checklists {
		items := cl.CheckItems
		sort.SliceStable(items, func(i, j int) bool { return items[i].Pos < items[j].Pos })
	}
	return &d, nil
}

func fmtPos(p float64) string { return strconv.FormatFloat(p, 'f', -1, 64) }

func (c *Client) CreateCard(ctx context.Context, listID, name string, pos float64) (*Card, error) {
	var card Card
	err := c.do(ctx, "POST", "/cards", url.Values{"idList": {listID}, "name": {name}, "pos": {fmtPos(pos)}}, &card)
	return &card, err
}

func (c *Client) UpdateCard(ctx context.Context, cardID string, u CardUpdate) (*Card, error) {
	v := url.Values{}
	if u.Name != nil {
		v.Set("name", *u.Name)
	}
	if u.Desc != nil {
		v.Set("desc", *u.Desc)
	}
	if u.IDList != nil {
		v.Set("idList", *u.IDList)
	}
	if u.Pos != nil {
		v.Set("pos", fmtPos(*u.Pos))
	}
	if u.ClearDue {
		v.Set("due", "null")
	} else if u.Due != nil {
		v.Set("due", u.Due.UTC().Format(time.RFC3339))
	}
	if u.DueComplete != nil {
		v.Set("dueComplete", strconv.FormatBool(*u.DueComplete))
	}
	if u.Closed != nil {
		v.Set("closed", strconv.FormatBool(*u.Closed))
	}
	if u.IDLabels != nil {
		v.Set("idLabels", strings.Join(*u.IDLabels, ","))
	}
	if u.IDMembers != nil {
		v.Set("idMembers", strings.Join(*u.IDMembers, ","))
	}
	var card Card
	err := c.do(ctx, "PUT", "/cards/"+cardID, v, &card)
	return &card, err
}

func (c *Client) DeleteCard(ctx context.Context, cardID string) error {
	return c.do(ctx, "DELETE", "/cards/"+cardID, nil, nil)
}

func (c *Client) AddComment(ctx context.Context, cardID, text string) (*Comment, error) {
	var cm Comment
	err := c.do(ctx, "POST", "/cards/"+cardID+"/actions/comments", url.Values{"text": {text}}, &cm)
	return &cm, err
}

func (c *Client) CreateChecklist(ctx context.Context, cardID, name string) (*Checklist, error) {
	var cl Checklist
	err := c.do(ctx, "POST", "/checklists", url.Values{"idCard": {cardID}, "name": {name}}, &cl)
	return &cl, err
}

func (c *Client) AddCheckItem(ctx context.Context, checklistID, name string) (*CheckItem, error) {
	var it CheckItem
	err := c.do(ctx, "POST", "/checklists/"+checklistID+"/checkItems", url.Values{"name": {name}, "pos": {"bottom"}}, &it)
	return &it, err
}

func (c *Client) SetCheckItem(ctx context.Context, cardID, itemID string, done bool) error {
	state := "incomplete"
	if done {
		state = "complete"
	}
	return c.do(ctx, "PUT", "/cards/"+cardID+"/checkItem/"+itemID, url.Values{"state": {state}}, nil)
}

func (c *Client) DeleteCheckItem(ctx context.Context, checklistID, itemID string) error {
	return c.do(ctx, "DELETE", "/checklists/"+checklistID+"/checkItems/"+itemID, nil, nil)
}

func (c *Client) CreateList(ctx context.Context, boardID, name string, pos float64) (*List, error) {
	var l List
	err := c.do(ctx, "POST", "/lists", url.Values{"idBoard": {boardID}, "name": {name}, "pos": {fmtPos(pos)}}, &l)
	return &l, err
}

func (c *Client) RenameList(ctx context.Context, listID, name string) error {
	return c.do(ctx, "PUT", "/lists/"+listID, url.Values{"name": {name}}, nil)
}

func (c *Client) ArchiveList(ctx context.Context, listID string) error {
	return c.do(ctx, "PUT", "/lists/"+listID+"/closed", url.Values{"value": {"true"}}, nil)
}
