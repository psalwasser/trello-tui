package trello

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type recorded struct {
	method, path string
	query, form  map[string]string
	auth         string
}

func fakeServer(t *testing.T, routes map[string]any) (*Client, *[]recorded) {
	t.Helper()
	var reqs []recorded
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		rec := recorded{method: r.Method, path: r.URL.Path, query: map[string]string{}, form: map[string]string{}, auth: r.Header.Get("Authorization")}
		for k := range r.URL.Query() {
			rec.query[k] = r.URL.Query().Get(k)
		}
		for k := range r.PostForm {
			rec.form[k] = r.PostForm.Get(k)
		}
		mu.Lock()
		reqs = append(reqs, rec)
		mu.Unlock()
		body, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			http.Error(w, "not found", 404)
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	c := NewClient("KEY", "TOKEN")
	c.Base = srv.URL
	return c, &reqs
}

func TestBoardsMergesStarsAndWorkspaces(t *testing.T) {
	c, reqs := fakeServer(t, map[string]any{
		"GET /members/me/boards":        []map[string]any{{"id": "b1", "name": "One", "idOrganization": "o1"}, {"id": "b2", "name": "Two"}},
		"GET /members/me/organizations": []map[string]any{{"id": "o1", "displayName": "Acme"}},
		"GET /members/me/boardStars":    []map[string]any{{"idBoard": "b2"}},
	})
	bs, err := c.Boards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bs[0].WorkspaceName != "Acme" || bs[0].Starred || !bs[1].Starred {
		t.Fatalf("unexpected boards: %+v", bs)
	}
	for _, r := range *reqs {
		if r.auth != `OAuth oauth_consumer_key="KEY", oauth_token="TOKEN"` {
			t.Fatalf("bad auth header %q", r.auth)
		}
	}
}

func TestBoardDataSortsByPos(t *testing.T) {
	c, reqs := fakeServer(t, map[string]any{
		"GET /boards/b1": map[string]any{
			"id": "b1", "name": "B",
			"lists": []map[string]any{{"id": "l2", "pos": 2}, {"id": "l1", "pos": 1}},
			"cards": []map[string]any{{"id": "c2", "pos": 20, "idList": "l1"}, {"id": "c1", "pos": 10, "idList": "l1", "due": "2026-10-15T09:00:00.000Z"}},
		},
	})
	bd, err := c.BoardData(context.Background(), "b1")
	if err != nil {
		t.Fatal(err)
	}
	if bd.Lists[0].ID != "l1" || bd.Cards[0].ID != "c1" || bd.Cards[0].Due == nil {
		t.Fatalf("unexpected board data: %+v", bd)
	}
	q := (*reqs)[0].query
	if q["cards"] != "open" || q["lists"] != "open" || !strings.Contains(q["card_fields"], "badges") {
		t.Fatalf("unexpected query %v", q)
	}
}

func TestUpdateCardForm(t *testing.T) {
	c, reqs := fakeServer(t, map[string]any{"PUT /cards/c1": map[string]any{"id": "c1"}})
	name, pos, labels := "New", 1234.5, []string{"a", "b"}
	due := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	if _, err := c.UpdateCard(context.Background(), "c1", CardUpdate{Name: &name, Pos: &pos, IDLabels: &labels, Due: &due}); err != nil {
		t.Fatal(err)
	}
	f := (*reqs)[0].form
	if f["name"] != "New" || f["pos"] != "1234.5" || f["idLabels"] != "a,b" || f["due"] != "2026-10-15T09:00:00Z" {
		t.Fatalf("unexpected form %v", f)
	}
	if _, ok := f["desc"]; ok {
		t.Fatal("unset fields must not be sent")
	}

	if _, err := c.UpdateCard(context.Background(), "c1", CardUpdate{ClearDue: true}); err != nil {
		t.Fatal(err)
	}
	if (*reqs)[1].form["due"] != "null" {
		t.Fatalf("clear due sent %v", (*reqs)[1].form)
	}
}

func TestAPIError(t *testing.T) {
	c, _ := fakeServer(t, nil)
	_, err := c.Me(context.Background())
	if apiErr, ok := err.(*APIError); !ok || apiErr.Status != 404 {
		t.Fatalf("want 404 APIError, got %v", err)
	}
}

func TestPosBetween(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		prev, next *float64
		want       float64
	}{
		{nil, nil, 65536},
		{nil, f(100), 50},
		{f(100), nil, 65636},
		{f(100), f(200), 150},
	}
	for _, c := range cases {
		if got := PosBetween(c.prev, c.next); got != c.want {
			t.Errorf("PosBetween = %v, want %v", got, c.want)
		}
	}
}
