package trello

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer serves canned Trello responses and records the credentials it
// was called with.
func newTestServer(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "the-key" || r.URL.Query().Get("token") != "the-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return server
}

func TestClient(t *testing.T) {
	server := newTestServer(t, map[string]string{
		"/1/members/me/boards": `[{"id":"b1","name":"Board One"},{"id":"b2","name":"Board Two"}]`,
		"/1/boards/b1/lists":   `[{"id":"l1","name":"List One"}]`,
		"/1/boards/b1/cards":   `[{"id":"c1","name":"Card One","idList":"l1"}]`,
	})

	client := New("the-key", "the-token", server.URL)
	ctx := context.Background()

	boardID, err := client.BoardID(ctx, "Board One")
	if err != nil {
		t.Fatalf("BoardID returned error: %v", err)
	}
	if boardID != "b1" {
		t.Errorf("BoardID = %q, want %q", boardID, "b1")
	}

	if _, err := client.BoardID(ctx, "No Such Board"); err == nil {
		t.Error("BoardID returned no error for a board that does not exist")
	}

	lists, err := client.Lists(ctx, boardID)
	if err != nil {
		t.Fatalf("Lists returned error: %v", err)
	}
	if len(lists) != 1 || lists[0].Name != "List One" {
		t.Errorf("Lists = %v", lists)
	}

	cards, err := client.Cards(ctx, boardID)
	if err != nil {
		t.Fatalf("Cards returned error: %v", err)
	}
	if len(cards) != 1 || cards[0].Name != "Card One" || cards[0].IDList != "l1" {
		t.Errorf("Cards = %v", cards)
	}
}

func TestClientReportsFailures(t *testing.T) {
	server := newTestServer(t, nil)

	client := New("wrong", "wrong", server.URL)
	if _, err := client.Boards(context.Background()); err == nil {
		t.Error("Boards returned no error for rejected credentials")
	}
}

// Two boards of the same name would make the configuration ambiguous.
func TestClientRejectsDuplicateBoardNames(t *testing.T) {
	server := newTestServer(t, map[string]string{
		"/1/members/me/boards": `[{"id":"b1","name":"Same"},{"id":"b2","name":"Same"}]`,
	})

	client := New("the-key", "the-token", server.URL)
	if _, err := client.BoardID(context.Background(), "Same"); err == nil {
		t.Error("BoardID returned no error for duplicate board names")
	}
}

func TestCloseCard(t *testing.T) {
	var method string
	var query map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, query = r.Method, r.URL.Query()
		switch r.URL.Path {
		case "/1/cards/c1":
			w.Write([]byte(`{"id":"c1","closed":true}`))
		case "/1/cards/c2":
			w.Write([]byte(`{"id":"c2","closed":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := New("the-key", "the-token", server.URL)
	ctx := context.Background()

	if err := client.CloseCard(ctx, "c1"); err != nil {
		t.Fatalf("CloseCard returned error: %v", err)
	}
	if method != http.MethodPut {
		t.Errorf("CloseCard used %s, want PUT", method)
	}
	for key, want := range map[string]string{"dueComplete": "true", "closed": "true", "key": "the-key", "token": "the-token"} {
		if got := query[key]; len(got) != 1 || got[0] != want {
			t.Errorf("CloseCard sent %s=%v, want %q", key, got, want)
		}
	}

	if err := client.CloseCard(ctx, "c2"); err == nil {
		t.Error("CloseCard returned no error for a card Trello did not archive")
	}
	if err := client.CloseCard(ctx, "missing"); err == nil {
		t.Error("CloseCard returned no error for a card that does not exist")
	}
}
