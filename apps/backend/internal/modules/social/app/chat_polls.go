package app

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func LoadChatPolls(
	ctx context.Context, reads sqlc.DBTX, viewer ids.UserID, messages []ChatMessage, now time.Time,
) error {
	const op = "social.LoadChatPolls"
	byID := map[uuid.UUID]int{}
	var pollIDs []uuid.UUID
	for i, m := range messages {
		if !m.Deleted && m.Poll == nil && m.Attachment != nil && m.Attachment.Poll != nil {
			byID[m.ID] = i
			pollIDs = append(pollIDs, m.ID)
		}
	}
	if len(pollIDs) == 0 {
		return nil
	}
	rows, err := sqlc.New(reads).ListChatPolls(ctx, sqlc.ListChatPollsParams{
		Viewer: viewer.UUID(), MessageIds: pollIDs,
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	for _, id := range pollIDs {
		closesAt := messages[byID[id]].Attachment.Poll.ClosesAt
		messages[byID[id]].Poll = &PollView{ClosesAt: closesAt, Closed: pollClosed(closesAt, now)}
	}
	for _, r := range rows {
		view := messages[byID[r.MessageID]].Poll
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
