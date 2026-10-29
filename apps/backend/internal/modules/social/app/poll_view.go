package app

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type PollOptionView struct {
	Position int
	Label    string
	Count    int
}

type PollView struct {
	ClosesAt   time.Time
	Closed     bool
	Options    []PollOptionView
	Total      int
	MyPosition *int
}

func pollClosed(closesAt, now time.Time) bool { return !now.Before(closesAt) }

func storePoll(
	ctx context.Context, q *sqlc.Queries, row sqlc.FeedComment, att *domain.Attachment,
) (*domain.Attachment, *PollView, error) {
	const op = "social.storePoll"
	if att.Poll == nil {
		return att, nil, nil
	}
	positions := make([]int32, len(att.Poll.Options))
	view := &PollView{
		ClosesAt: row.CreatedAt.Add(att.Poll.ClosesIn).UTC(),
		Options:  make([]PollOptionView, len(positions)),
	}
	for i, label := range att.Poll.Options {
		positions[i] = int32(i)
		view.Options[i] = PollOptionView{Position: i, Label: label}
	}
	err := q.InsertCommentPollOptions(ctx, sqlc.InsertCommentPollOptionsParams{
		CommentID: row.ID, Positions: positions, Labels: att.Poll.Options,
	})
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	stored := *att
	poll := *att.Poll
	poll.ClosesAt = view.ClosesAt
	stored.Poll = &poll
	return &stored, view, nil
}

func pollViews(
	ctx context.Context, reads sqlc.DBTX, viewer ids.UserID, comments []*Comment, now time.Time,
) error {
	const op = "social.pollViews"
	byID := map[uuid.UUID]*Comment{}
	var pollIDs []uuid.UUID
	for _, c := range comments {
		if c.Attachment != nil && c.Attachment.Poll != nil {
			byID[c.ID] = c
			pollIDs = append(pollIDs, c.ID)
		}
	}
	if len(pollIDs) == 0 {
		return nil
	}
	rows, err := sqlc.New(reads).ListCommentPolls(ctx, sqlc.ListCommentPollsParams{
		Viewer: viewer.UUID(), CommentIds: pollIDs,
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	for _, id := range pollIDs {
		closesAt := byID[id].Attachment.Poll.ClosesAt
		byID[id].Poll = &PollView{ClosesAt: closesAt, Closed: pollClosed(closesAt, now)}
	}
	for _, r := range rows {
		view := byID[r.CommentID].Poll
		view.Options = append(
			view.Options,
			PollOptionView{Position: int(r.Position), Label: r.Label, Count: int(r.Votes)},
		)
		view.Total += int(r.Votes)
		if r.Mine {
			position := int(r.Position)
			view.MyPosition = &position
		}
	}
	return nil
}
