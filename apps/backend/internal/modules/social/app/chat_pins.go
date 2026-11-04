package app

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const MaxChatPins = 3

type PinChatMessage struct {
	CabalID   ids.CabalID
	MessageID uuid.UUID
	Caller    ids.UserID
}

type UnpinChatMessage struct {
	CabalID   ids.CabalID
	MessageID uuid.UUID
	Caller    ids.UserID
}

type PinChatMessageHandler struct{ d ChatDeps }

func NewPinChatMessageHandler(d ChatDeps) *PinChatMessageHandler { return &PinChatMessageHandler{d: d} }

type UnpinChatMessageHandler struct{ d ChatDeps }

func NewUnpinChatMessageHandler(d ChatDeps) *UnpinChatMessageHandler {
	return &UnpinChatMessageHandler{d: d}
}

func (h *PinChatMessageHandler) Handle(ctx context.Context, cmd PinChatMessage) (ChatMessage, error) {
	const op = "social.PinChatMessage"
	admin, err := chatAdmin(ctx, h.d.Members, cmd.CabalID, cmd.Caller)
	if err != nil {
		return ChatMessage{}, err
	}
	var message ChatMessage
	changed := false
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		if err := q.LockChatPins(ctx, cmd.CabalID.UUID()); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		row, err := lockPinnable(ctx, q, cmd.CabalID, cmd.MessageID, op)
		switch {
		case err != nil:
			return err
		case row.ParentID.Valid:
			return errs.New(errs.CodeInvalidInput, op, slog.String("reason", domain.ReasonReply))
		case !admin && row.AuthorID.Bytes != cmd.Caller.UUID():
			return errs.New(errs.CodeChatPinForbidden, op)
		}
		if !row.PinnedAt.Valid {
			if err := pinMessage(ctx, q, tx, h.d, cmd); err != nil {
				return err
			}
			changed = true
		}
		message, err = storedChatMessage(ctx, q, cmd.CabalID, cmd.MessageID, op)
		return err
	})
	if err != nil {
		return ChatMessage{}, err
	}
	if changed {
		h.d.Publish.pinUpdated(ctx, cmd.CabalID, cmd.MessageID, true)
	}
	return message, nil
}

func pinMessage(ctx context.Context, q *sqlc.Queries, tx db.Tx, d ChatDeps, cmd PinChatMessage) error {
	const op = "social.PinChatMessage"
	pins, err := q.CountChatPins(ctx, cmd.CabalID.UUID())
	switch {
	case err != nil:
		return errs.Wrap(err, errs.CodeInternal, op)
	case pins >= MaxChatPins:
		return errs.New(errs.CodeChatPinLimit, op, slog.Int("pins", int(pins)))
	}
	if _, err := q.PinChatMessage(ctx, sqlc.PinChatMessageParams{
		At: d.Clock.Now().UTC(), Actor: cmd.Caller.UUID(), ID: cmd.MessageID,
	}); err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	return tx.Events.Append(ctx, events.ChatMessagePinned{
		V: 1, MessageID: cmd.MessageID, CabalID: cmd.CabalID.UUID(), ActorID: cmd.Caller.UUID(),
	})
}

func (h *UnpinChatMessageHandler) Handle(ctx context.Context, cmd UnpinChatMessage) (ChatMessage, error) {
	const op = "social.UnpinChatMessage"
	admin, err := chatAdmin(ctx, h.d.Members, cmd.CabalID, cmd.Caller)
	if err != nil {
		return ChatMessage{}, err
	}
	var message ChatMessage
	changed := false
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		row, err := lockPinnable(ctx, q, cmd.CabalID, cmd.MessageID, op)
		switch {
		case err != nil:
			return err
		case row.PinnedAt.Valid && !admin && row.PinnedBy.Bytes != cmd.Caller.UUID():
			return errs.New(errs.CodeChatPinForbidden, op)
		}
		if row.PinnedAt.Valid {
			if _, err := q.UnpinChatMessage(ctx, cmd.MessageID); err != nil {
				return errs.Wrap(err, errs.CodeInternal, op)
			}
			if err := tx.Events.Append(ctx, events.ChatMessageUnpinned{
				V: 1, MessageID: cmd.MessageID, CabalID: cmd.CabalID.UUID(), ActorID: cmd.Caller.UUID(),
			}); err != nil {
				return err
			}
			changed = true
		}
		message, err = storedChatMessage(ctx, q, cmd.CabalID, cmd.MessageID, op)
		return err
	})
	if err != nil {
		return ChatMessage{}, err
	}
	if changed {
		h.d.Publish.pinUpdated(ctx, cmd.CabalID, cmd.MessageID, false)
	}
	return message, nil
}

func ListChatPins(
	ctx context.Context, reads sqlc.DBTX, members Members, cabal ids.CabalID, viewer ids.UserID,
) ([]ChatMessage, error) {
	const op = "social.ListChatPins"
	if err := requireMember(ctx, members, cabal, viewer); err != nil {
		return nil, err
	}
	rows, err := sqlc.New(reads).ListChatPins(ctx, cabal.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	return storedMessages(rows), nil
}

func chatAdmin(ctx context.Context, members ChatMembers, cabal ids.CabalID, user ids.UserID) (bool, error) {
	const op = "social.chatAdmin"
	list, err := members.Members(ctx, cabal)
	if err != nil {
		return false, errs.Wrap(err, errs.CodeOf(err), op)
	}
	for _, m := range list {
		if m.UserID == user {
			return m.Role.IsAdmin(), nil
		}
	}
	return false, errs.New(errs.CodeNotCabalMember, op)
}

func lockPinnable(
	ctx context.Context, q *sqlc.Queries, cabal ids.CabalID, id uuid.UUID, op string,
) (sqlc.LockChatMessageRow, error) {
	row, found, err := lockChatMessage(ctx, q, id)
	switch {
	case err != nil:
		return row, err
	case !found || row.CabalID != cabal.UUID() || row.Deleted:
		return row, errs.New(errs.CodeChatMessageNotFound, op, slog.String("message_id", id.String()))
	}
	return row, nil
}

func storedChatMessage(
	ctx context.Context, q *sqlc.Queries, cabal ids.CabalID, id uuid.UUID, op string,
) (ChatMessage, error) {
	row, err := q.GetChatMessage(ctx, sqlc.GetChatMessageParams{ID: id, CabalID: cabal.UUID()})
	if err != nil {
		return ChatMessage{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return storedMessage(row), nil
}
