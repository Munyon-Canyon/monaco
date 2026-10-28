package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"slices"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ReplyRule string

const (
	ReplyRuleAnyone  ReplyRule = "anyone"
	ReplyRuleMembers ReplyRule = "members"
)

func ThreadOf(ctx context.Context, q *sqlc.Queries, item sqlc.GetCommentItemRow) (sqlc.GetCommentItemRow, error) {
	const op = "social.ThreadOf"
	proposal, shared, err := threadProposal(item)
	if err != nil || !shared {
		return item, err
	}
	thread, err := proposalThreadItem(ctx, q, proposal)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return sqlc.GetCommentItemRow{}, errs.New(
			errs.CodeFeedItemPending, op, slog.String("proposal_id", proposal.String()),
		)
	case err != nil:
		return sqlc.GetCommentItemRow{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return thread, nil
}

func proposalThreadItem(ctx context.Context, q *sqlc.Queries, proposal uuid.UUID) (sqlc.GetCommentItemRow, error) {
	id, err := q.GetProposalFeedItem(ctx, proposal)
	if err != nil {
		return sqlc.GetCommentItemRow{}, err
	}
	return q.GetCommentItem(ctx, id)
}

func ResolveThread(
	ctx context.Context, q *sqlc.Queries, id uuid.UUID,
) (item, thread sqlc.GetCommentItemRow, err error) {
	const op = "social.ResolveThread"
	item, err = q.GetCommentItem(ctx, id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return item, thread, errs.New(errs.CodeFeedItemNotFound, op, slog.String("feed_object_id", id.String()))
	case err != nil:
		return item, thread, errs.Wrap(err, errs.CodeInternal, op)
	}
	thread, err = ThreadOf(ctx, q, item)
	return item, thread, err
}

func threadProposal(item sqlc.GetCommentItemRow) (proposal uuid.UUID, shared bool, err error) {
	if !slices.Contains([]feed.Kind{feed.KindThreshold, feed.KindReply, feed.KindResult}, feed.Kind(item.Kind)) {
		return uuid.Nil, false, nil
	}
	payload, err := feed.ParsePayload(item.Payload)
	if err != nil {
		return uuid.Nil, false, err
	}
	if payload.Shape == feed.ShapeMember {
		return uuid.Nil, false, nil
	}
	proposal = payload.ProposalID
	if proposal == uuid.Nil && item.RefType == string(feed.RefProposals) {
		proposal = item.RefID
	}
	return proposal, true, nil
}

func CanReply(
	ctx context.Context, members Members, thread feed.Kind, cabal ids.CabalID, user ids.UserID,
) (bool, ReplyRule, error) {
	if thread != feed.KindProposal {
		return true, ReplyRuleAnyone, nil
	}
	member, err := members.IsMember(ctx, cabal, user)
	if err != nil {
		return false, ReplyRuleMembers, errs.Wrap(err, errs.CodeOf(err), "social.CanReply")
	}
	return member, ReplyRuleMembers, nil
}

func CanReplyToItem(
	ctx context.Context, reads sqlc.DBTX, members Members, id uuid.UUID, user ids.UserID,
) (bool, error) {
	_, thread, err := ResolveThread(ctx, sqlc.New(reads), id)
	if err != nil {
		return false, err
	}
	ok, _, err := CanReply(ctx, members, feed.Kind(thread.Kind), ids.CabalIDFrom(thread.CabalID.Bytes), user)
	return ok, err
}
