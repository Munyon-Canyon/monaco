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
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type VoteChatPoll struct {
	CabalID   ids.CabalID
	MessageID uuid.UUID
	Viewer    ids.UserID
	Position  int
}

type UnvoteChatPoll struct {
	CabalID   ids.CabalID
	MessageID uuid.UUID
	Viewer    ids.UserID
}

type VoteChatPollHandler struct{ d ChatDeps }

func NewVoteChatPollHandler(d ChatDeps) *VoteChatPollHandler { return &VoteChatPollHandler{d: d} }

type UnvoteChatPollHandler struct{ d ChatDeps }

func NewUnvoteChatPollHandler(d ChatDeps) *UnvoteChatPollHandler { return &UnvoteChatPollHandler{d: d} }

func (h *VoteChatPollHandler) Handle(ctx context.Context, cmd VoteChatPoll) (PollView, error) {
	const op = "social.VoteChatPoll"
	if err := requireMember(ctx, h.d.Members, cmd.CabalID, cmd.Viewer); err != nil {
		return PollView{}, err
	}
	position, err := domain.ParsePollPosition(cmd.Position)
	if err != nil {
		return PollView{}, err
	}
	attachment, author, err := h.d.requireOpenChatPoll(ctx, cmd.CabalID, cmd.MessageID)
	if err != nil {
		return PollView{}, err
	}
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		if err := refuseBlockedAuthor(ctx, tx, cmd.Viewer, author, op); err != nil {
			return err
		}
		rows, err := sqlc.New(tx.Queries()).UpsertChatPollVote(ctx, sqlc.UpsertChatPollVoteParams{
			MessageID: cmd.MessageID, UserID: cmd.Viewer.UUID(), Position: position, At: h.d.Clock.Now().UTC(),
		})
		switch {
		case err != nil:
			return errs.Wrap(err, errs.CodeInternal, op)
		case rows == 0:
			return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "poll_option"))
		}
		return tx.Events.Append(ctx, events.ChatPollVoted{
			V: 1, MessageID: cmd.MessageID, CabalID: cmd.CabalID.UUID(), UserID: cmd.Viewer.UUID(),
			Position: cmd.Position,
		})
	})
	if err != nil {
		return PollView{}, err
	}
	return h.d.settleChatPoll(ctx, cmd.CabalID, cmd.MessageID, cmd.Viewer, attachment, true)
}

func (h *UnvoteChatPollHandler) Handle(ctx context.Context, cmd UnvoteChatPoll) (PollView, error) {
	const op = "social.UnvoteChatPoll"
	if err := requireMember(ctx, h.d.Members, cmd.CabalID, cmd.Viewer); err != nil {
		return PollView{}, err
	}
	attachment, _, err := h.d.requireOpenChatPoll(ctx, cmd.CabalID, cmd.MessageID)
	if err != nil {
		return PollView{}, err
	}
	changed := false
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := sqlc.New(tx.Queries()).DeleteChatPollVote(ctx, sqlc.DeleteChatPollVoteParams{
			MessageID: cmd.MessageID, UserID: cmd.Viewer.UUID(),
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		changed = rows > 0
		return nil
	})
	if err != nil {
		return PollView{}, err
	}
	return h.d.settleChatPoll(ctx, cmd.CabalID, cmd.MessageID, cmd.Viewer, attachment, changed)
}

func (d ChatDeps) requireOpenChatPoll(
	ctx context.Context, cabal ids.CabalID, message uuid.UUID,
) (*domain.Attachment, uuid.UUID, error) {
	const op = "social.requireOpenChatPoll"
	row, err := sqlc.New(d.Reads).GetChatPoll(ctx, sqlc.GetChatPollParams{
		MessageID: message, CabalID: cabal.UUID(),
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, uuid.Nil, errs.New(errs.CodeChatMessageNotFound, op, slog.String("message_id", message.String()))
	case err != nil:
		return nil, uuid.Nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	attachment := domain.DecodeAttachment(string(domain.AttachmentPoll), row.Payload)
	if pollClosed(attachment.Poll.ClosesAt, d.Clock.Now()) {
		return nil, uuid.Nil, errs.New(errs.CodePollClosed, op, slog.String("message_id", message.String()))
	}
	return attachment, row.AuthorID.Bytes, nil
}

func (d ChatDeps) settleChatPoll(
	ctx context.Context, cabal ids.CabalID, message uuid.UUID, viewer ids.UserID, attachment *domain.Attachment,
	changed bool,
) (PollView, error) {
	messages := []ChatMessage{{ID: message, Attachment: attachment}}
	if err := LoadChatPolls(ctx, d.Reads, viewer, messages, d.Clock.Now()); err != nil {
		return PollView{}, err
	}
	if changed {
		d.Publish.pollUpdated(ctx, cabal, message, *messages[0].Poll)
	}
	return *messages[0].Poll, nil
}
