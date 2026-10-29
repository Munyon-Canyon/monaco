package adapters

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const CatchUpDurable = "notify_catch_up"

func CatchUpHandlers(c *app.CatchUpClears) []bus.HandlerSpec {
	return []bus.HandlerSpec{
		clearing("notify.catch_up_vote_cast", c, c.OnVoteCast),
		clearing("notify.catch_up_comment_created", c, c.OnCommentCreated),
		clearing("notify.catch_up_access_decided", c, c.OnInviteAccepted),
		bus.Handle("notify.catch_up_vote_retracted",
			func(ctx context.Context, tx db.Tx, e events.VoteRetracted, _ time.Time) error {
				return c.Reopen(ctx, tx, e)
			}),
		hinting("notify.catch_up_proposal_passed", c, closedMembers[events.ProposalPassed](c, passedCabal)),
		hinting("notify.catch_up_proposal_failed", c, closedMembers[events.ProposalFailed](c, failedCabal)),
		hinting("notify.catch_up_proposal_expired", c, closedMembers[events.ProposalExpired](c, expiredCabal)),
		hinting("notify.catch_up_proposal_withdrawn", c, closedMembers[events.ProposalWithdrawn](c, withdrawnCabal)),
		hinting("notify.catch_up_proposal_voided", c, closedMembers[events.ProposalVoided](c, voidedCabal)),
		hinting("notify.catch_up_access_requested", c, func(_ context.Context, e events.CabalAccessRequested) (
			[]ids.UserID, error,
		) {
			return c.InviteeOf(e), nil
		}),
	}
}

func clearing[E events.Event](
	name string,
	c *app.CatchUpClears,
	fetch func(ctx context.Context, e E) ([]app.CatchUpClear, error),
) bus.HandlerSpec {
	return bus.HandleFetched(name, fetch,
		func(ctx context.Context, tx db.Tx, _ E, clears []app.CatchUpClear, at time.Time) error {
			return c.Apply(ctx, tx, clears, at)
		})
}

func hinting[E events.Event](
	name string,
	c *app.CatchUpClears,
	fetch func(ctx context.Context, e E) ([]ids.UserID, error),
) bus.HandlerSpec {
	return bus.HandleFetched(name, fetch,
		func(_ context.Context, tx db.Tx, _ E, users []ids.UserID, _ time.Time) error {
			c.Hint(tx, users)
			return nil
		})
}

func closedMembers[E events.Event](
	c *app.CatchUpClears, cabalOf func(E) uuid.UUID,
) func(ctx context.Context, e E) ([]ids.UserID, error) {
	return func(ctx context.Context, e E) ([]ids.UserID, error) {
		return c.ProposalMembers(ctx, cabalOf(e))
	}
}

func passedCabal(e events.ProposalPassed) uuid.UUID       { return e.CabalID }
func failedCabal(e events.ProposalFailed) uuid.UUID       { return e.CabalID }
func expiredCabal(e events.ProposalExpired) uuid.UUID     { return e.CabalID }
func withdrawnCabal(e events.ProposalWithdrawn) uuid.UUID { return e.CabalID }
func voidedCabal(e events.ProposalVoided) uuid.UUID       { return e.CabalID }
