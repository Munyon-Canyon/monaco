package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type CallStore interface {
	UserRecords(
		ctx context.Context,
		users []ids.UserID,
		cabal *ids.CabalID,
		p port.Period,
	) (map[ids.UserID]port.CallRecord, error)
	PendingSymbols(
		ctx context.Context,
		users []ids.UserID,
		cabal *ids.CabalID,
		p port.Period,
	) (map[ids.UserID][]string, error)
	CabalRecords(ctx context.Context, cabals []ids.CabalID, p port.Period) (map[ids.CabalID]port.CallRecord, error)
}

type CallRecords struct{ store CallStore }

var _ port.Calls = CallRecords{}

func NewCallRecords(store CallStore) CallRecords { return CallRecords{store: store} }

func (c CallRecords) Records(
	ctx context.Context, userIDs []ids.UserID, cabalID *ids.CabalID, p port.Period,
) (map[ids.UserID]port.CallRecord, error) {
	if len(userIDs) == 0 {
		return map[ids.UserID]port.CallRecord{}, nil
	}
	out, err := c.store.UserRecords(ctx, userIDs, cabalID, p)
	if err != nil {
		return nil, err
	}
	pending, err := c.store.PendingSymbols(ctx, userIDs, cabalID, p)
	if err != nil {
		return nil, err
	}
	for id, symbols := range pending {
		rec := out[id]
		rec.PendingSymbols = symbols
		out[id] = rec
	}
	return out, nil
}

func (c CallRecords) CabalRecords(
	ctx context.Context, cabalIDs []ids.CabalID, p port.Period,
) (map[ids.CabalID]port.CallRecord, error) {
	if len(cabalIDs) == 0 {
		return map[ids.CabalID]port.CallRecord{}, nil
	}
	return c.store.CabalRecords(ctx, cabalIDs, p)
}
