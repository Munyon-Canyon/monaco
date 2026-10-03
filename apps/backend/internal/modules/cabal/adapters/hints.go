package adapters

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
)

const inviteDirection = string(domain.DirectionInvite)

type HintPublisher interface {
	PublishHint(ctx context.Context, key string, payload []byte)
}

type Hints struct {
	Publish HintPublisher
}

func (h Hints) Handle(_ context.Context, tx db.Tx, e events.CabalCreated, _ time.Time) error {
	h.after(tx, "user."+e.CreatorID.String()+".cabals")
	return nil
}

func (h Hints) MemberJoined(_ context.Context, tx db.Tx, e events.CabalMemberJoined, _ time.Time) error {
	h.after(tx, cabalHint(e.CabalID, "members"), accessHint(e.UserID))
	return nil
}

func (h Hints) MemberLeft(_ context.Context, tx db.Tx, e events.CabalMemberLeft, _ time.Time) error {
	h.after(tx, cabalHint(e.CabalID, "members"), "user."+e.UserID.String()+".cabals", accessHint(e.UserID))
	return nil
}

func (h Hints) AccessDecided(_ context.Context, tx db.Tx, e events.CabalAccessDecided, _ time.Time) error {
	if e.Direction == inviteDirection {
		h.after(tx, cabalHint(e.CabalID, "members"), inviteHint(e.UserID), accessHint(e.UserID))
		return nil
	}
	h.after(tx, cabalHint(e.CabalID, "members"), cabalHint(e.CabalID, "access_requests"), accessHint(e.UserID))
	return nil
}

func (h Hints) AccessRequested(_ context.Context, tx db.Tx, e events.CabalAccessRequested, _ time.Time) error {
	if e.Direction == inviteDirection {
		h.after(tx, inviteHint(e.UserID))
		return nil
	}
	h.after(tx, cabalHint(e.CabalID, "access_requests"))
	return nil
}

func (h Hints) Updated(_ context.Context, tx db.Tx, e events.CabalUpdated, _ time.Time) error {
	h.after(tx, cabalHint(e.CabalID, "updated"))
	return nil
}

func (h Hints) after(tx db.Tx, keys ...string) {
	tx.AfterCommit(func(ctx context.Context) {
		for _, key := range keys {
			h.Publish.PublishHint(ctx, key, nil)
		}
	})
}

func cabalHint(cabal uuid.UUID, what string) string { return "cabal." + cabal.String() + "." + what }

func inviteHint(user uuid.UUID) string { return "user." + user.String() + ".cabal_invites" }

func accessHint(user uuid.UUID) string { return "user." + user.String() + "." + sse.MembershipChanged }
