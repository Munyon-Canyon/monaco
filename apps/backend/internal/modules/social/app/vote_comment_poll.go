package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type VoteCommentPoll struct {
	CommentID uuid.UUID
	Viewer    ids.UserID
	Position  int
}

type WithdrawCommentPollVote struct {
	CommentID uuid.UUID
	Viewer    ids.UserID
}

type PollDeps struct {
	UoW   *db.UnitOfWork
	Reads sqlc.DBTX
	Clock clock.Clock
	Hints Hints
}

type VoteCommentPollHandler struct{ d PollDeps }

func NewVoteCommentPollHandler(d PollDeps) *VoteCommentPollHandler {
	return &VoteCommentPollHandler{d: d}
}

type WithdrawCommentPollVoteHandler struct{ d PollDeps }

func NewWithdrawCommentPollVoteHandler(d PollDeps) *WithdrawCommentPollVoteHandler {
	return &WithdrawCommentPollVoteHandler{d: d}
}

func (h *VoteCommentPollHandler) Handle(ctx context.Context, cmd VoteCommentPoll) (PollView, error) {
	const op = "social.VoteCommentPoll"
	position, err := domain.ParsePollPosition(cmd.Position)
	if err != nil {
		return PollView{}, err
	}
	feedObjectID, attachment, err := requireOpenPoll(ctx, h.d, cmd.Viewer, cmd.CommentID)
	if err != nil {
		return PollView{}, err
	}
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := sqlc.New(tx.Queries()).UpsertCommentPollVote(ctx, sqlc.UpsertCommentPollVoteParams{
			CommentID: cmd.CommentID, UserID: cmd.Viewer.UUID(), Position: position,
			At: h.d.Clock.Now().UTC(),
		})
		switch {
		case err != nil:
			return errs.Wrap(err, errs.CodeInternal, op)
		case rows == 0:
			return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "poll_option"))
		}
		tx.AfterCommit(func(ctx context.Context) { h.d.Hints.PublishHint(ctx, FeedHint, nil) })
		return tx.Events.Append(ctx, events.CommentPollVoted{
			V: 1, CommentID: cmd.CommentID, FeedObjectID: feedObjectID, UserID: cmd.Viewer.UUID(),
			Position: cmd.Position,
		})
	})
	if err != nil {
		return PollView{}, err
	}
	return pollStateOf(ctx, h.d, cmd.Viewer, cmd.CommentID, attachment)
}

func (h *WithdrawCommentPollVoteHandler) Handle(ctx context.Context, cmd WithdrawCommentPollVote) (PollView, error) {
	const op = "social.WithdrawCommentPollVote"
	_, attachment, err := requireOpenPoll(ctx, h.d, cmd.Viewer, cmd.CommentID)
	if err != nil {
		return PollView{}, err
	}
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := sqlc.New(tx.Queries()).DeleteCommentPollVote(ctx, sqlc.DeleteCommentPollVoteParams{
			CommentID: cmd.CommentID, UserID: cmd.Viewer.UUID(),
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		if rows > 0 {
			tx.AfterCommit(func(ctx context.Context) { h.d.Hints.PublishHint(ctx, FeedHint, nil) })
		}
		return nil
	})
	if err != nil {
		return PollView{}, err
	}
	return pollStateOf(ctx, h.d, cmd.Viewer, cmd.CommentID, attachment)
}

func requireOpenPoll(
	ctx context.Context, d PollDeps, viewer ids.UserID, comment uuid.UUID,
) (uuid.UUID, *domain.Attachment, error) {
	const op = "social.requireOpenPoll"
	row, err := sqlc.New(d.Reads).GetCommentPoll(ctx, comment)
	switch {
	case errors.Is(err, sql.ErrNoRows) || err == nil && row.Deleted:
		return uuid.Nil, nil, errs.New(errs.CodeCommentNotFound, op, slog.String("comment_id", comment.String()))
	case err != nil:
		return uuid.Nil, nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	if _, err := CanReact(ctx, d.Reads, viewer, row.FeedObjectID); err != nil {
		return uuid.Nil, nil, err
	}
	attachment := domain.DecodeAttachment(string(domain.AttachmentPoll), row.Payload)
	if pollClosed(attachment.Poll.ClosesAt, d.Clock.Now()) {
		return uuid.Nil, nil, errs.New(errs.CodePollClosed, op, slog.String("comment_id", comment.String()))
	}
	return row.FeedObjectID, attachment, nil
}

func pollStateOf(
	ctx context.Context, d PollDeps, viewer ids.UserID, comment uuid.UUID, attachment *domain.Attachment,
) (PollView, error) {
	c := Comment{ID: comment, Attachment: attachment}
	if err := pollViews(ctx, d.Reads, viewer, []*Comment{&c}, d.Clock.Now()); err != nil {
		return PollView{}, err
	}
	return *c.Poll, nil
}
