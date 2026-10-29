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

func (k CabalAccessRequestedAdmins) Render(
	ctx context.Context, e events.CabalAccessRequested, recipient ids.UserID,
) (Message, error) {
	msg, err := k.CabalAccessRequested.Render(ctx, e, recipient)
	if err != nil || e.ViaUserID == nil {
		return msg, err
	}
	via, err := personSender(ctx, k.Users, ids.UserIDFrom(*e.ViaUserID))
	if err != nil {
		return Message{}, err
	}
	msg.Body += " through " + via.Name + "'s link"
	return msg, nil
}
