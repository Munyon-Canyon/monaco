package app

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const opCatchUpClears = "notify.CatchUpClears"

const (
	directionInvite  = "invite"
	decisionAccepted = "accepted"
)

type CatchUpClear struct {
	User     ids.UserID
	Key      string
	Reason   string
	Snapshot []byte
	At       time.Time
}

type CatchUpClears struct {
	reads  *ReadCatchUp
	cabals Cabals
	hints  Hints
}

func NewCatchUpClears(reads *ReadCatchUp, cabals Cabals, hints Hints) *CatchUpClears {
	return &CatchUpClears{reads: reads, cabals: cabals, hints: hints}
}

func (c *CatchUpClears) OnVoteCast(ctx context.Context, e events.VoteCast) ([]CatchUpClear, error) {
	voter, proposal := ids.UserIDFrom(e.VoterID), ids.ProposalIDFrom(e.ProposalID)
	cards, err := c.reads.ports.Governance.ProposalCards(ctx, voter, []ids.ProposalID{proposal})
	if err != nil {
		return nil, err
	}
	card, ok := cards[proposal]
	if !ok {
		return nil, nil
	}
	card.YourChoice = e.Choice
	replies, err := c.reads.ports.Replies.ReplyCounts(ctx, []ids.ProposalID{proposal})
	if err != nil {
		return nil, err
	}
	deco := decoration{replies: replies}
	item := domain.CatchUpItem{
		Key: domain.ItemKeyProposalPrefix + proposal.String(), Kind: domain.CatchUpWaitingVote, At: card.CreatedAt,
		Proposal: deco.proposal(card), Cabal: deco.cabalRef(card.CabalID.String(), card.CabalName),
	}
	return snapshotClears(voter, domain.ClearedVoted, e.CastAt, item), nil
}

func (c *CatchUpClears) OnCommentCreated(ctx context.Context, e events.CommentCreated) ([]CatchUpClear, error) {
	actor := ids.UserIDFrom(e.AuthorID)
	keys, err := sqlc.New(c.reads.ports.Reads).ThreadCommentKeys(ctx, sqlc.ThreadCommentKeysParams{
		UserID: actor.UUID(), FeedItemID: e.FeedObjectID.String(),
		Since: c.reads.ports.Clock.Now().Add(-domain.CatchUpWindow),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, opCatchUpClears)
	}
	if len(keys) == 0 {
		return nil, nil
	}
	items, err := c.reads.Render(ctx, actor, keys)
	if err != nil {
		return nil, err
	}
	var out []CatchUpClear
	for _, key := range keys {
		item, ok := items[key]
		if !ok {
			continue
		}
		out = append(out, snapshotClears(actor, domain.ClearedReplied, time.Time{}, item)...)
	}
	return out, nil
}

func (c *CatchUpClears) OnInviteAccepted(ctx context.Context, e events.CabalAccessDecided) ([]CatchUpClear, error) {
	if e.Direction != directionInvite || e.Decision != decisionAccepted {
		return nil, nil
	}
	found, err := c.reads.ports.InviteByID.InvitesByID(ctx, []uuid.UUID{e.RequestID})
	if err != nil {
		return nil, err
	}
	invite, ok := found[e.RequestID]
	if !ok {
		return nil, nil
	}
	user := ids.UserIDFrom(e.UserID)
	cand := candidate{
		key: domain.ItemKeyInvitePrefix + invite.ID.String(), kind: domain.CatchUpInvite, at: invite.CreatedAt,
		invite: &invite,
	}
	deco := decoration{set: catchUpSet{actives: []candidate{cand}}}
	if err := c.reads.decorateHoldings(ctx, &deco); err != nil {
		return nil, err
	}
	return snapshotClears(user, domain.ClearedJoined, time.Time{}, deco.item(cand, user)), nil
}

func snapshotClears(user ids.UserID, reason string, at time.Time, item domain.CatchUpItem) []CatchUpClear {
	raw, _ := json.Marshal(item)
	return []CatchUpClear{{User: user, Key: item.Key, Reason: reason, Snapshot: raw, At: at}}
}

func (c *CatchUpClears) Apply(ctx context.Context, tx db.Tx, clears []CatchUpClear, at time.Time) error {
	q := sqlc.New(tx.Queries())
	var users []ids.UserID
	for _, cl := range clears {
		when := cl.At
		if when.IsZero() {
			when = at
		}
		err := q.UpsertClear(ctx, sqlc.UpsertClearParams{
			UserID: cl.User.UUID(), ItemKey: cl.Key, Reason: cl.Reason, Snapshot: cl.Snapshot, At: when,
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeDBUnavailable, opCatchUpClears)
		}
		if !slices.Contains(users, cl.User) {
			users = append(users, cl.User)
		}
	}
	hintCatchUp(tx, c.hints, users...)
	return nil
}

func (c *CatchUpClears) Reopen(ctx context.Context, tx db.Tx, e events.VoteRetracted) error {
	voter := ids.UserIDFrom(e.VoterID)
	err := sqlc.New(tx.Queries()).ReopenVoted(ctx, sqlc.ReopenVotedParams{
		UserID: voter.UUID(), ItemKey: domain.ItemKeyProposalPrefix + e.ProposalID.String(), At: e.RetractedAt,
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeDBUnavailable, opCatchUpClears)
	}
	hintCatchUp(tx, c.hints, voter)
	return nil
}

func (c *CatchUpClears) ProposalMembers(ctx context.Context, cabalID uuid.UUID) ([]ids.UserID, error) {
	return memberIDs(ctx, c.cabals, &cabalID)
}

func (c *CatchUpClears) InviteeOf(e events.CabalAccessRequested) []ids.UserID {
	if e.Direction != directionInvite {
		return nil
	}
	return []ids.UserID{ids.UserIDFrom(e.UserID)}
}

func (c *CatchUpClears) Hint(tx db.Tx, users []ids.UserID) {
	hintCatchUp(tx, c.hints, users...)
}
