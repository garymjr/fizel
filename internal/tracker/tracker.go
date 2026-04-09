package tracker

import "github.com/gmurray/fizel/internal/model"

type Tracker interface {
	FetchCandidateItems() ([]model.Item, error)
	FetchItemsByStates(states []string) ([]model.Item, error)
	FetchItemStatesByIDs(ids []string) ([]model.Item, error)
	CreateComment(id, body string) error
	UpdateItemState(id, state string) error
}
