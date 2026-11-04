package adapters

import (
	"context"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (h HTTP) loadTallies(ctx context.Context, items []app.FeedItem) (map[ids.ProposalID]app.Tally, error) {
	const op = "social.wireFeedItems"
	var proposals []ids.ProposalID
	for _, item := range items {
		if item.Kind != feed.KindProposal {
			continue
		}
		id := ids.ProposalIDFrom(item.ProposalID)
		if !slices.Contains(proposals, id) {
			proposals = append(proposals, id)
		}
	}
	if len(proposals) == 0 || h.Tallies == nil {
		return map[ids.ProposalID]app.Tally{}, nil
	}
	tallies, err := h.Tallies.Tallies(ctx, proposals)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return tallies, nil
}

func wireTally(item app.FeedItem, tallies map[ids.ProposalID]app.Tally, out *api.FeedItem) {
	if item.Kind == feed.KindResult && item.Tally != nil {
		t := item.Tally
		out.Tally = &api.FeedTally{Yes: t.Yes, No: t.No, Voters: t.Voters, Needed: t.Needed}
		return
	}
	if item.Kind != feed.KindProposal {
		return
	}
	expires := item.ExpiresAt
	if t, ok := tallies[ids.ProposalIDFrom(item.ProposalID)]; ok {
		out.Tally = &api.FeedTally{Yes: t.Yes, No: t.No, Voters: t.Voters, Needed: t.Needed}
		if !t.ExpiresAt.IsZero() {
			expires = t.ExpiresAt
		}
	}
	if !expires.IsZero() {
		out.ExpiresAt = &expires
	}
}

func (h HTTP) loadBallots(
	ctx context.Context,
	me ids.UserID,
	items []app.FeedItem,
) (map[ids.ProposalID]app.ViewerBallot, error) {
	const op = "social.wireFeedItems"
	var proposals []ids.ProposalID
	for _, item := range items {
		if item.Kind != feed.KindProposal {
			continue
		}
		id := ids.ProposalIDFrom(item.ProposalID)
		if !slices.Contains(proposals, id) {
			proposals = append(proposals, id)
		}
	}
	if len(proposals) == 0 || h.Ballots == nil {
		return map[ids.ProposalID]app.ViewerBallot{}, nil
	}
	ballots, err := h.Ballots.MyBallots(ctx, me, proposals)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return ballots, nil
}

func wireBallot(item app.FeedItem, ballots map[ids.ProposalID]app.ViewerBallot, out *api.FeedItem) {
	if item.Kind != feed.KindProposal {
		return
	}
	b, ok := ballots[ids.ProposalIDFrom(item.ProposalID)]
	if !ok {
		return
	}
	out.CanVote = b.CanVote
	out.MyBallot = optionalWireEnum[api.FeedItemMyBallot](b.Choice)
}
