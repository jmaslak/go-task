// Package trello is a small client for the Trello API, covering what the task
// application needs to mirror cards as tasks and to close a card whose task
// is done.
package trello

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is where the Trello API lives.
const DefaultBaseURL = "https://trello.com/"

// Client talks to the Trello API on behalf of one set of credentials.
type Client struct {
	APIKey  string
	Token   string
	BaseURL string
	HTTP    *http.Client

	// boards caches the board name to identifier mapping, which costs a
	// request to look up.
	boards map[string]string
}

// Board is a Trello board.
type Board struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// List is a list of cards within a board.
type List struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Card is a single Trello card.
type Card struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	IDList string `json:"idList"`
}

// New returns a client for the given credentials. An empty baseURL uses the
// public API.
func New(apiKey, token, baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	return &Client{
		APIKey:  apiKey,
		Token:   token,
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Boards returns the boards the credentials can see.
func (c *Client) Boards(ctx context.Context) ([]Board, error) {
	var boards []Board
	err := c.get(ctx, "1/members/me/boards", []string{"name"}, &boards)
	return boards, err
}

// BoardID returns the identifier of the board with the given name.
func (c *Client) BoardID(ctx context.Context, name string) (string, error) {
	if c.boards == nil {
		boards, err := c.Boards(ctx)
		if err != nil {
			return "", err
		}

		c.boards = make(map[string]string, len(boards))
		for _, board := range boards {
			if _, duplicate := c.boards[board.Name]; duplicate {
				return "", fmt.Errorf("cannot handle duplicate board names in Trello: %s", board.Name)
			}
			c.boards[board.Name] = board.ID
		}
	}

	id, ok := c.boards[name]
	if !ok {
		return "", fmt.Errorf("board %q does not exist on Trello", name)
	}
	return id, nil
}

// Lists returns the lists on a board.
func (c *Client) Lists(ctx context.Context, boardID string) ([]List, error) {
	var lists []List
	err := c.get(ctx, "1/boards/"+url.PathEscape(boardID)+"/lists", []string{"name"}, &lists)
	return lists, err
}

// Cards returns the cards on a board, along with the list each one is in.
func (c *Client) Cards(ctx context.Context, boardID string) ([]Card, error) {
	var cards []Card
	err := c.get(ctx, "1/boards/"+url.PathEscape(boardID)+"/cards", []string{"name", "idList"}, &cards)
	return cards, err
}

// CloseCard marks a card's due date complete and archives the card, as is
// done to a card whose task is finished. An archived card is no longer among
// a board's cards, so the task mirroring it is not recreated by a sync.
func (c *Client) CloseCard(ctx context.Context, cardID string) error {
	query := url.Values{}
	query.Set("dueComplete", "true")
	query.Set("closed", "true")
	query.Set("fields", "closed")
	var card struct {
		Closed bool `json:"closed"`
	}
	if err := c.do(ctx, http.MethodPut, "1/cards/"+url.PathEscape(cardID), query, &card); err != nil {
		return err
	}
	if !card.Closed {
		return fmt.Errorf("trello: card %s was not archived", cardID)
	}
	return nil
}

// get fetches one API endpoint and decodes the JSON response into out.
func (c *Client) get(ctx context.Context, path string, fields []string, out any) error {
	query := url.Values{}
	query.Set("fields", strings.Join(fields, ","))
	return c.do(ctx, http.MethodGet, path, query, out)
}

// do makes one API request, authenticated with the client's credentials, and
// decodes the JSON response into out.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, out any) error {
	endpoint, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("invalid Trello base URL: %w", err)
	}
	endpoint = endpoint.JoinPath(path)

	query.Set("key", c.APIKey)
	query.Set("token", c.Token)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("trello: %s: %s", path, resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("trello: %s: %w", path, err)
	}
	return nil
}
