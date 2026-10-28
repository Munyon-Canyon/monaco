package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Admins interface {
	Admins(ctx context.Context, id ids.CabalID) ([]ids.UserID, error)
}

type CabalAccessRequestedAdmins struct {
	CabalAccessRequested
	Admins Admins
}

func (k CabalAccessRequestedAdmins) Recipients(
	ctx context.Context, e events.CabalAccessRequested,
) ([]ids.UserID, error) {
	if e.Direction != accessDirectionRequest {
		return nil, nil
	}
	return k.Admins.Admins(ctx, ids.CabalIDFrom(e.CabalID))
}
