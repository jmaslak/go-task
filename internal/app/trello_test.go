package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaslak/go-task/config"
	"github.com/jmaslak/go-task/task"
)

// trelloServer serves a board holding the given cards.
func trelloServer(t *testing.T, cards string) *httptest.Server {
	t.Helper()

	routes := map[string]string{
		"/1/members/me/boards": `[{"id":"b1","name":"Board One"}]`,
		"/1/boards/b1/lists":   `[{"id":"l1","name":"List One"}]`,
		"/1/boards/b1/cards":   cards,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// trelloConfig points the application at a fake Trello.
func trelloConfig(a *App, baseURL string) {
	a.Config.Trello = config.Trello{
		APIKey:  "the-key",
		Token:   "the-token",
		BaseURL: baseURL,
		Tasks:   map[string]map[string]string{"Board One": {"List One": "trello"}},
	}
}

func TestTrelloSync(t *testing.T) {
	a, _ := newTestApp(t, "")
	mustNewTask(t, a, "A local task")

	server := trelloServer(t, `[{"id":"c1","name":"Card One","idList":"l1"},{"id":"c2","name":"Card Two","idList":"l1"}]`)
	trelloConfig(a, server.URL)

	if err := a.TrelloSync(context.Background()); err != nil {
		t.Fatalf("TrelloSync returned error: %v", err)
	}

	tasks := mustTasks(t, a)
	if len(tasks) != 3 {
		t.Fatalf("len(tasks) = %d, want 3", len(tasks))
	}

	mirrored := map[string]*task.Task{}
	for _, task := range tasks {
		if task.TrelloID != "" {
			mirrored[task.TrelloID] = task
		}
	}
	if len(mirrored) != 2 {
		t.Fatalf("%d tasks mirror a card, want 2", len(mirrored))
	}
	if got := mirrored["c1"]; got == nil || got.Title != "Card One" || !got.HasTag("trello") {
		t.Errorf("the task for card c1 = %v", got)
	}

	// Syncing again must not create the cards a second time.
	if err := a.TrelloSync(context.Background()); err != nil {
		t.Fatalf("TrelloSync returned error: %v", err)
	}
	if len(mustTasks(t, a)) != 3 {
		t.Errorf("a second sync changed the task count to %d", len(mustTasks(t, a)))
	}

	// A card that is gone takes its task with it, and the tasks that are
	// left are renumbered.
	gone := trelloServer(t, `[{"id":"c1","name":"Card One","idList":"l1"}]`)
	trelloConfig(a, gone.URL)

	if err := a.TrelloSync(context.Background()); err != nil {
		t.Fatalf("TrelloSync returned error: %v", err)
	}

	tasks = mustTasks(t, a)
	if len(tasks) != 2 {
		t.Fatalf("len(tasks) = %d, want 2", len(tasks))
	}
	for i, task := range tasks {
		if task.Number != i+1 {
			t.Errorf("task %d is numbered %d", i, task.Number)
		}
		if task.Title == "Card Two" {
			t.Error("the task for the deleted card is still here")
		}
	}
}

func TestTrelloSyncNeedsCredentials(t *testing.T) {
	a, _ := newTestApp(t, "")

	if err := a.TrelloSync(context.Background()); err == nil {
		t.Error("TrelloSync returned no error without an API key")
	}

	a.Config.Trello.APIKey = "the-key"
	if err := a.TrelloSync(context.Background()); err == nil {
		t.Error("TrelloSync returned no error without a token")
	}
}
