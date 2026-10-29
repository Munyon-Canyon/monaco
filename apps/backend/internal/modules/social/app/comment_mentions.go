package app

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const maxCommentMentions = 5

func (h *CreateCommentHandler) resolveCommentMentions(
	ctx context.Context, tx db.Tx, at postedOn, author ids.UserID, body string,
) ([]uuid.UUID, error) {
	const op = "social.resolveCommentMentions"
	candidates, err := h.mentionCandidates(ctx, author, body)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}
	cards, err := h.d.Users.UsersByID(ctx, candidates)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	var out []uuid.UUID
	for _, id := range candidates {
		if len(out) == maxCommentMentions {
			break
		}
		card, found := cards[id]
		if !found || card.Deleted || domain.AccountStatus(card.AccountStatus) == domain.AccountBanned {
			continue
		}
		ok, err := h.mayBeMentioned(ctx, tx, at, author, id)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, id.UUID())
		}
	}
	return out, nil
}

func (h *CreateCommentHandler) mentionCandidates(
	ctx context.Context, author ids.UserID, body string,
) ([]ids.UserID, error) {
	handles := domain.ParseMentions(body)
	if len(handles) == 0 {
		return nil, nil
	}
	byHandle, err := h.d.Users.UserIDsByHandles(ctx, handles)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "social.mentionCandidates")
	}
	candidates := make([]ids.UserID, 0, len(handles))
	for _, handle := range handles {
		if id, ok := byHandle[handle]; ok && id != author && !slices.Contains(candidates, id) {
			candidates = append(candidates, id)
		}
	}
	return candidates, nil
}

func (h *CreateCommentHandler) mayBeMentioned(
	ctx context.Context, tx db.Tx, at postedOn, author, user ids.UserID,
) (bool, error) {
	ok, err := h.canReadThread(ctx, tx, at, user)
	if err != nil || !ok {
		return false, err
	}
	blocked, err := blockedEitherWay(ctx, tx, author, user)
	return !blocked && err == nil, err
}

func (h *CreateCommentHandler) canReadThread(
	ctx context.Context,
	tx db.Tx,
	at postedOn,
	user ids.UserID,
) (bool, error) {
	const op = "social.canReadThread"
	if at.rule == ReplyRuleMembers {
		member, err := h.d.Members.IsMember(ctx, ids.CabalIDFrom(at.thread.CabalID.Bytes), user)
		if err != nil {
			return false, errs.Wrap(err, errs.CodeOf(err), op)
		}
		return member, nil
	}
	view, err := GetFeedItem(ctx, tx.Queries(), user, at.thread.ID, FeedFilter{})
	return err == nil && view.Visible, err
}
