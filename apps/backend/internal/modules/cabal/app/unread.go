package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type UnreadCounter = interface {
	UnreadCounts(ctx context.Context, user ids.UserID, cabals []ids.CabalID) (map[ids.CabalID]int, error)
}

type UnwiredUnread struct{}

func (UnwiredUnread) UnreadCounts(context.Context, ids.UserID, []ids.CabalID) (map[ids.CabalID]int, error) {
	return nil, errs.New(errs.CodeInternal, "cabal.UnwiredUnread")
}
