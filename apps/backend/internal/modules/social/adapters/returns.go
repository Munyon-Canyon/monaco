package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type people struct {
	cards   map[ids.UserID]app.UserCard
	returns map[ids.UserID]app.PersonReturn
}

func loadPeople(
	ctx context.Context, users app.Users, returns app.Returns, who []ids.UserID, op string,
) (people, error) {
	cards, err := users.UsersByID(ctx, who)
	if err != nil {
		return people{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	found, err := returns.PeopleReturns(ctx, app.OneMonth, who)
	if err != nil {
		return people{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return people{cards: cards, returns: found}, nil
}

func (p people) authorReturn(id ids.UserID) (bps *int64, private bool) {
	card, ok := p.cards[id]
	switch {
	case !ok || card.Deleted:
		return nil, false
	case !card.SharePerformance:
		return nil, true
	}
	return p.returns[id].ReturnBps, false
}
