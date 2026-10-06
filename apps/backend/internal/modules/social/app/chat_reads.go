package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	ChatPageDefault = 50
	ChatPageMax     = 100
)

type ChatChannelQuery struct {
	CabalID ids.CabalID
	Viewer  ids.UserID
	Before  *uuid.UUID
	After   *uuid.UUID
	Limit   int
}

type ChatThreadQuery struct {
	CabalID  ids.CabalID
	Viewer   ids.UserID
	ParentID uuid.UUID
	Before   *uuid.UUID
	Limit    int
}

type ChatThread struct {
	Parent  ChatMessage
	Replies []ChatMessage
}

func ListChatChannel(ctx context.Context, db sqlc.DBTX, members Members, q ChatChannelQuery) ([]ChatMessage, error) {
	const op = "social.ListChatChannel"
	if q.Before != nil && q.After != nil {
		return nil, errs.New(errs.CodeInvalidInput, op, slog.String("cursor", "before and after"))
	}
	limit, err := checkChatRead(ctx, members, q.CabalID, q.Viewer, q.Limit)
	if err != nil {
		return nil, err
	}
	reads := sqlc.New(db)
	var rows []sqlc.CabalMessage
	if q.After != nil {
		var at domain.Keyset
		if at, err = chatCursor(ctx, reads, q.CabalID, *q.After); err != nil {
			return nil, err
		}
		rows, err = reads.ListChatChannelAfter(ctx, sqlc.ListChatChannelAfterParams{
			CabalID: q.CabalID.UUID(), AfterAt: at.At, AfterID: at.ID, RowLimit: limit,
		})
	} else {
		params := sqlc.ListChatChannelParams{CabalID: q.CabalID.UUID(), RowLimit: limit}
		if params.HasBefore, params.BeforeAt, params.BeforeID, err = optionalCursor(
			ctx, reads, q.CabalID, q.Before,
		); err != nil {
			return nil, err
		}
		rows, err = reads.ListChatChannel(ctx, params)
	}
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	page := storedMessages(rows)
	if q.Before != nil {
		return page, nil
	}
	return page, withSeenCount(ctx, reads, q.CabalID, page)
}

func GetChatThread(ctx context.Context, db sqlc.DBTX, members Members, q ChatThreadQuery) (ChatThread, error) {
	const op = "social.GetChatThread"
	limit, err := checkChatRead(ctx, members, q.CabalID, q.Viewer, q.Limit)
	if err != nil {
		return ChatThread{}, err
	}
	reads := sqlc.New(db)
	row, err := reads.GetChatMessage(ctx, sqlc.GetChatMessageParams{ID: q.ParentID, CabalID: q.CabalID.UUID()})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ChatThread{}, errs.New(errs.CodeChatMessageNotFound, op)
	case err != nil:
		return ChatThread{}, errs.Wrap(err, errs.CodeInternal, op)
	case row.ParentID.Valid:
		return ChatThread{}, errs.New(errs.CodeChatParentIsReply, op)
	case row.DeletedAt.Valid && row.ReplyCount == 0:
		return ChatThread{}, errs.New(errs.CodeChatMessageNotFound, op, slog.Bool("deleted", true))
	}
	params := sqlc.ListChatRepliesParams{ParentID: q.ParentID, RowLimit: limit}
	if params.HasBefore, params.BeforeAt, params.BeforeID, err = optionalCursor(
		ctx, reads, q.CabalID, q.Before,
	); err != nil {
		return ChatThread{}, err
	}
	replies, err := reads.ListChatReplies(ctx, params)
	if err != nil {
		return ChatThread{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	slices.Reverse(replies)
	return ChatThread{Parent: storedMessage(row), Replies: storedMessages(replies)}, nil
}

func checkChatRead(
	ctx context.Context, members Members, cabal ids.CabalID, viewer ids.UserID, limit int,
) (int32, error) {
	if limit < 1 || limit > ChatPageMax {
		return 0, errs.New(errs.CodeInvalidInput, "social.checkChatRead", slog.Int("limit", limit))
	}
	return int32(limit), requireMember(ctx, members, cabal, viewer)
}

func optionalCursor(
	ctx context.Context, reads *sqlc.Queries, cabal ids.CabalID, id *uuid.UUID,
) (bool, time.Time, uuid.UUID, error) {
	if id == nil {
		return false, time.Time{}, uuid.Nil, nil
	}
	at, err := chatCursor(ctx, reads, cabal, *id)
	return err == nil, at.At, at.ID, err
}

func chatCursor(ctx context.Context, reads *sqlc.Queries, cabal ids.CabalID, id uuid.UUID) (domain.Keyset, error) {
	const op = "social.chatCursor"
	row, err := reads.GetChatCursor(ctx, sqlc.GetChatCursorParams{ID: id, CabalID: cabal.UUID()})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Keyset{}, errs.New(errs.CodeChatMessageNotFound, op, slog.String("cursor", id.String()))
	}
	if err != nil {
		return domain.Keyset{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return domain.Keyset{At: row.CreatedAt, ID: row.ID}, nil
}

func storedMessages(rows []sqlc.CabalMessage) []ChatMessage {
	out := make([]ChatMessage, len(rows))
	for i, row := range rows {
		out[i] = storedMessage(row)
	}
	return out
}

func storedMessage(row sqlc.CabalMessage) ChatMessage {
	m := ChatMessage{
		ID: row.ID, CabalID: ids.CabalIDFrom(row.CabalID), AuthorID: ids.UserIDFrom(row.AuthorID), Body: row.Body,
		CreatedAt: row.CreatedAt.UTC(), ParentID: row.ParentID.Bytes, AlsoInChannel: row.AlsoInChannel,
		ReplyCount: row.ReplyCount, ProposalID: row.ProposalID.Bytes, Deleted: row.DeletedAt.Valid,
	}
	if row.LastReplyAt.Valid {
		at := row.LastReplyAt.Time.UTC()
		m.LastReplyAt = &at
	}
	if m.Deleted {
		m.Body = ""
	}
	return m
}
