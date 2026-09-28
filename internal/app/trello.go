package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/jmaslak/go-task/internal/task"
	"github.com/jmaslak/go-task/internal/trello"
)

// TrelloSync mirrors the configured Trello lists into the task list. The sync
// is one way: cards become tasks, and tasks whose card is gone are deleted.
func (a *App) TrelloSync(ctx context.Context) error {
	cfg := a.Config.Trello
	if cfg.APIKey == "" {
		return errors.New("must provide Trello API key in config file")
	}
	if cfg.Token == "" {
		return errors.New("must provide Trello token in config file")
	}

	client := trello.New(cfg.APIKey, cfg.Token, cfg.BaseURL)

	// Trello is asked for the cards with the task directory unlocked: holding
	// the lock across the network would keep every other task process out of
	// the directory for as long as Trello took to answer, or for as long as
	// it took to give up on Trello answering at all.
	var fetched []listCards
	for boardName, lists := range cfg.Tasks {
		cards, err := fetchBoard(ctx, client, boardName, lists)
		if err != nil {
			return err
		}
		fetched = append(fetched, cards...)
	}

	return a.Store.WithLock(func() error {
		for _, list := range fetched {
			if err := a.syncList(list.cards, list.tag); err != nil {
				return err
			}
		}

		return a.Store.Coalesce()
	})
}

// listCards are the cards of one configured Trello list, keyed by card ID,
// along with the tag the list is mirrored under.
type listCards struct {
	tag   string
	cards map[string]trello.Card
}

// fetchBoard collects the cards of the configured lists of one board.
func fetchBoard(ctx context.Context, client *trello.Client, boardName string, lists map[string]string) ([]listCards, error) {
	boardID, err := client.BoardID(ctx, boardName)
	if err != nil {
		return nil, err
	}

	boardLists, err := client.Lists(ctx, boardID)
	if err != nil {
		return nil, err
	}
	boardCards, err := client.Cards(ctx, boardID)
	if err != nil {
		return nil, err
	}

	var fetched []listCards
	for listName, tag := range lists {
		listID := ""
		for _, list := range boardLists {
			if list.Name == listName {
				listID = list.ID
			}
		}
		if listID == "" {
			return nil, fmt.Errorf("list %q does not exist on Trello board %q", listName, boardName)
		}

		cards := map[string]trello.Card{}
		for _, card := range boardCards {
			if card.IDList == listID {
				cards[card.ID] = card
			}
		}

		fetched = append(fetched, listCards{tag: tag, cards: cards})
	}

	return fetched, nil
}

// syncList reconciles the cards of one Trello list against the tasks carrying
// that list's tag.
func (a *App) syncList(cards map[string]trello.Card, tag string) error {
	tasks, err := a.Store.Tasks()
	if err != nil {
		return err
	}

	mirrored := map[string]*task.Task{}
	for _, t := range tasks {
		if t.TrelloID != "" && t.HasTag(tag) {
			mirrored[t.TrelloID] = t
		}
	}

	// Cards with no task yet.
	for id, card := range cards {
		if _, ok := mirrored[id]; ok {
			continue
		}

		number, err := a.Store.NextNumber()
		if err != nil {
			return err
		}

		t := task.New(number, card.Name)
		t.Tags = []string{tag}
		t.TrelloID = id
		if err := a.Store.Save(t); err != nil {
			return err
		}
	}

	// Tasks whose card is gone.
	for id, t := range mirrored {
		if _, ok := cards[id]; ok {
			continue
		}
		if err := a.Store.Delete(t.Number); err != nil {
			return err
		}
	}

	return nil
}
